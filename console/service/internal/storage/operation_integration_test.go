package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestOperationPreflightLockAndTerminalImmutability(t *testing.T) {
	dsn := os.Getenv("PG_CONSOLE_TEST_DSN")
	if dsn == "" {
		t.Skip("PG_CONSOLE_TEST_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var projectID, environmentID int64
	if err = pool.QueryRow(ctx, "select project_id from projects where project_name='default'").Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, "select environment_id from environments where environment_name='test' limit 1").Scan(&environmentID); err != nil {
		t.Fatal(err)
	}
	store := &dbStorage{db: pool}
	cluster, err := store.CreateCluster(ctx, &CreateClusterReq{ProjectID: projectID, EnvironmentID: environmentID, Name: fmt.Sprintf("operation-test-%d", time.Now().UnixNano()), PostgreSqlVersion: 16})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.DeleteCluster(context.Background(), cluster.ID) }()

	walContinuous := true
	observedAt := time.Now().UTC().Truncate(time.Second)
	if err = store.UpsertBackupEvidence(ctx, &BackupEvidence{
		ClusterID: cluster.ID, ObservedAt: observedAt, RepositoryReachable: true,
		LatestFull: &observedAt, Retention: []byte(`{"full":7}`), WalContinuous: &walContinuous,
		Locks: []byte(`[]`), SchedulerOwners: []byte(`["postgresql-1"]`), FreshnessSeconds: 86400,
	}); err != nil {
		t.Fatal(err)
	}
	backupEvidence, err := store.GetBackupEvidence(ctx, cluster.ID)
	if err != nil || backupEvidence == nil || !backupEvidence.RepositoryReachable ||
		backupEvidence.LatestFull == nil || string(backupEvidence.SchedulerOwners) != `["postgresql-1"]` {
		t.Fatalf("backup evidence = %+v, %v", backupEvidence, err)
	}

	preflightReq := &CreateOperationPreflightReq{
		ClusterID: cluster.ID, Type: OperationTypeSwitchover, Observed: []byte(`{"token":"cleartext"}`), Desired: []byte(`{}`),
		Checks: []byte(`[]`), Blockers: []byte(`[]`), Plan: []byte(`[]`), AffectedNodes: []byte(`[]`),
		Confirmation: "SWITCHOVER TO postgresql-2", TopologyHash: "hash", ExpiresAt: time.Now().Add(time.Minute),
	}
	preflight, err := store.CreateOperationPreflight(ctx, preflightReq)
	if err != nil {
		t.Fatal(err)
	}
	if preflight.Type != OperationTypeSwitchover {
		t.Fatalf("preflight type = %q", preflight.Type)
	}
	var observed map[string]any
	if err = json.Unmarshal(preflight.Observed, &observed); err != nil || observed["token"] != "[REDACTED]" {
		t.Fatalf("preflight secret persisted: %s", preflight.Observed)
	}
	if consumed, err := store.ConsumeOperationPreflight(ctx, preflight.ID); err != nil || !consumed {
		t.Fatalf("consume = %v, %v", consumed, err)
	}
	if consumed, err := store.ConsumeOperationPreflight(ctx, preflight.ID); err != nil || consumed {
		t.Fatalf("second consume = %v, %v", consumed, err)
	}
	unsupported := *preflightReq
	unsupported.Type = "unsupported"
	if _, err = store.CreateOperationPreflight(ctx, &unsupported); err == nil {
		t.Fatal("unsupported preflight operation type was accepted")
	}

	req := CreateOperationReq{
		ProjectID: projectID, ClusterID: cluster.ID, Type: OperationTypeQueryAnalyticsDisable,
		Actor: "fixture-operator", SanitizedParams: []byte(`{"state":"disabled","password":"cleartext"}`),
		PreflightSnapshot: []byte(`{"id":"fixture","checks":[],"token":"cleartext"}`),
		Plan:              []byte(`["serial rollout"]`), AffectedNodes: []byte(`["postgresql-1"]`),
	}

	type reservation struct {
		operation *Operation
		err       error
	}
	start := make(chan struct{})
	ready := make(chan struct{}, 2)
	results := make(chan reservation, 2)
	for range 2 {
		attempt := req
		attempt.Cid = uuid.NewString()
		go func(attempt CreateOperationReq) {
			ready <- struct{}{}
			<-start
			operation, err := store.ReserveOperation(ctx, &attempt)
			results <- reservation{operation, err}
		}(attempt)
	}
	for range 2 {
		<-ready
	}
	close(start)

	var first *Operation
	failures := 0
	for range 2 {
		result := <-results
		if result.err != nil {
			failures++
		} else {
			first = result.operation
		}
	}
	if first == nil || failures != 1 {
		t.Fatalf("concurrent reservations: winner=%v failures=%d", first != nil, failures)
	}

	running := OperationStatusRunning
	code := "container"
	if _, err = store.UpdateOperation(ctx, &UpdateOperationReq{ID: first.ID, Status: &running, DockerCode: &code}); err != nil {
		t.Fatal(err)
	}
	succeeded := OperationStatusSucceeded
	verification := []byte(`{"verified":true,"stderr":"cleartext"}`)
	next := "No action required."
	if _, err = store.UpdateOperation(ctx, &UpdateOperationReq{
		ID: first.ID, Status: &succeeded, FinalVerification: verification, SafeNextAction: &next,
	}); err != nil {
		t.Fatal(err)
	}
	if active, err := store.HasActiveOperation(ctx, cluster.ID); err != nil || active {
		t.Fatalf("active after terminal = %v, %v", active, err)
	}
	correction := "audit correction PASSWORD=cleartext"
	if _, err = store.UpdateOperation(ctx, &UpdateOperationReq{ID: first.ID, Logs: &correction}); err != nil {
		t.Fatalf("append-only terminal audit correction failed: %v", err)
	}
	var auditComplete bool
	if err = pool.QueryRow(ctx, `select
		actor = 'fixture-operator'
		and sanitized_params = '{"state":"disabled","password":"[REDACTED]"}'
		and preflight_snapshot = '{"id":"fixture","checks":[],"token":"[REDACTED]"}'
		and plan = '["serial rollout"]'
		and affected_nodes = '["postgresql-1"]'
		and final_verification = '{"verified":true,"stderr":"[REDACTED]"}'
		and safe_next_action = 'No action required.'
		and operation_log like '%audit correction%'
		and operation_log not like '%cleartext%'
		and created_at is not null
		and updated_at is not null
		from operations where id = $1`, first.ID).Scan(&auditComplete); err != nil {
		t.Fatal(err)
	}
	if !auditComplete {
		t.Fatal("durable operation audit is incomplete")
	}
	failed := OperationStatusFailed
	if _, err = store.UpdateOperation(ctx, &UpdateOperationReq{ID: first.ID, Status: &failed}); err == nil {
		t.Fatal("terminal operation was mutable")
	}
	req.Cid = uuid.NewString()
	second, err := store.ReserveOperation(ctx, &req)
	if err != nil {
		t.Fatal(err)
	}
	operations, _, err := store.GetOperations(ctx, &GetOperationsReq{
		ProjectID: projectID, StartedFrom: time.Time{}, EndedTill: time.Now().Add(time.Hour), ClusterName: &cluster.Name,
	})
	if err != nil {
		t.Fatal(err)
	}
	var firstFinished, secondFinished *time.Time
	var firstFound, secondFound bool
	for _, operation := range operations {
		if operation.ID == first.ID {
			firstFound = true
			firstFinished = operation.Finished
		}
		if operation.ID == second.ID {
			secondFound = true
			secondFinished = operation.Finished
		}
	}
	if !firstFound || !secondFound || firstFinished == nil || secondFinished != nil {
		t.Fatalf("operations found: succeeded=%t queued=%t; finished timestamps: succeeded=%v queued=%v",
			firstFound, secondFound, firstFinished, secondFinished)
	}
	healthOperations, err := store.GetClusterHealthOperations(ctx, cluster.ID)
	if err != nil {
		t.Fatal(err)
	}
	var healthActive, healthLatest bool
	for _, operation := range healthOperations {
		healthActive = healthActive || operation.ID == second.ID
		healthLatest = healthLatest || operation.ID == first.ID
	}
	if !healthActive || !healthLatest {
		t.Fatalf("health operations = %+v", healthOperations)
	}
	cancelled := OperationStatusCancelled
	if _, err = store.UpdateOperation(ctx, &UpdateOperationReq{ID: second.ID, Status: &cancelled}); err != nil {
		t.Fatal(err)
	}
	operations, _, err = store.GetOperations(ctx, &GetOperationsReq{
		ProjectID: projectID, StartedFrom: time.Time{}, EndedTill: time.Now().Add(time.Hour), ClusterName: &cluster.Name,
	})
	if err != nil {
		t.Fatal(err)
	}
	secondFound = false
	for _, operation := range operations {
		if operation.ID == second.ID {
			secondFound = operation.Finished != nil
		}
	}
	if !secondFound {
		t.Fatal("cancelled operation has no finished timestamp")
	}
}
