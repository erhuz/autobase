package storage

import (
	"context"
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
	defer pool.Exec(context.Background(), "delete from clusters where cluster_id=$1", cluster.ID) //nolint:errcheck

	preflight, err := store.CreateOperationPreflight(ctx, &CreateOperationPreflightReq{
		ClusterID: cluster.ID, Type: OperationTypeQueryAnalyticsDisable, Observed: []byte(`{}`), Desired: []byte(`{}`),
		Checks: []byte(`[]`), Blockers: []byte(`[]`), Plan: []byte(`[]`), AffectedNodes: []byte(`[]`),
		Confirmation: "DISABLE QUERY ANALYTICS", TopologyHash: "hash", ExpiresAt: time.Now().Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if consumed, err := store.ConsumeOperationPreflight(ctx, preflight.ID); err != nil || !consumed {
		t.Fatalf("consume = %v, %v", consumed, err)
	}
	if consumed, err := store.ConsumeOperationPreflight(ctx, preflight.ID); err != nil || consumed {
		t.Fatalf("second consume = %v, %v", consumed, err)
	}

	req := &CreateOperationReq{ProjectID: projectID, ClusterID: cluster.ID, Type: OperationTypeQueryAnalyticsDisable, Cid: uuid.NewString()}
	first, err := store.ReserveOperation(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.ReserveOperation(ctx, req); err == nil {
		t.Fatal("second cluster mutation acquired the lock")
	}
	running := OperationStatusRunning
	code := "container"
	if _, err = store.UpdateOperation(ctx, &UpdateOperationReq{ID: first.ID, Status: &running, DockerCode: &code}); err != nil {
		t.Fatal(err)
	}
	succeeded := OperationStatusSucceeded
	if _, err = store.UpdateOperation(ctx, &UpdateOperationReq{ID: first.ID, Status: &succeeded}); err != nil {
		t.Fatal(err)
	}
	if active, err := store.HasActiveOperation(ctx, cluster.ID); err != nil || active {
		t.Fatalf("active after terminal = %v, %v", active, err)
	}
	correction := "audit correction"
	if _, err = store.UpdateOperation(ctx, &UpdateOperationReq{ID: first.ID, Logs: &correction}); err != nil {
		t.Fatalf("append-only terminal audit correction failed: %v", err)
	}
	failed := OperationStatusFailed
	if _, err = store.UpdateOperation(ctx, &UpdateOperationReq{ID: first.ID, Status: &failed}); err == nil {
		t.Fatal("terminal operation was mutable")
	}
	second, err := store.ReserveOperation(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	cancelled := OperationStatusCancelled
	if _, err = store.UpdateOperation(ctx, &UpdateOperationReq{ID: second.ID, Status: &cancelled}); err != nil {
		t.Fatal(err)
	}
}
