package cluster

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"postgresql-cluster-console/internal/storage"
)

const backupPlaybook = "backup_pgbackrest.yml"

type backupDesired struct {
	Type           string `json:"type"`
	SchedulerOwner string `json:"scheduler_owner"`
}

func (h *guardedOperationsHandler) backupPreflightState(ctx context.Context, clusterInfo *storage.Cluster, operationType string) (*guardedPreflight, error) {
	evidence, err := h.db.GetBackupEvidence(ctx, clusterInfo.ID)
	if err != nil {
		return nil, err
	}
	active, err := h.db.HasActiveOperation(ctx, clusterInfo.ID)
	if err != nil {
		return nil, err
	}
	enabled := backupEnabled(clusterInfo.ExtraVars)
	owners, locks := []string{}, []string{}
	if evidence != nil {
		_ = json.Unmarshal(evidence.SchedulerOwners, &owners)
		_ = json.Unmarshal(evidence.Locks, &locks)
	}
	freshFor := 10 * time.Minute
	if h.cfg != nil && h.cfg.Backup.RunEvery > 0 {
		freshFor = 2 * h.cfg.Backup.RunEvery
	}
	evidenceFresh := evidence != nil && time.Since(evidence.ObservedAt) >= 0 && time.Since(evidence.ObservedAt) <= freshFor
	owner := ""
	if len(owners) == 1 {
		owner = owners[0]
	}
	checks := []preflightCheck{
		{Name: "pgBackRest configured", OK: enabled},
		{Name: "backup evidence fresh", OK: evidenceFresh},
		{Name: "repository reachable", OK: evidence != nil && evidence.RepositoryReachable},
		{Name: "exactly one scheduler owner", OK: len(owners) == 1},
		{Name: "repository has no active lock", OK: len(locks) == 0},
		{Name: "no active cluster mutation", OK: !active},
	}
	if operationType == storage.OperationTypeBackupDiff {
		checks = append(checks, preflightCheck{Name: "full backup exists", OK: evidence != nil && evidence.LatestFull != nil})
	}
	blockers := make([]string, 0)
	for _, check := range checks {
		if !check.OK {
			blockers = append(blockers, check.Name)
		}
	}
	observed := map[string]any{
		"observed_at": nil, "repository_reachable": false,
		"locks": locks, "scheduler_owners": owners,
	}
	if evidence != nil {
		observed["observed_at"] = evidence.ObservedAt
		observed["repository_reachable"] = evidence.RepositoryReachable
		observed["latest_full"] = evidence.LatestFull
		observed["latest_differential"] = evidence.LatestDifferential
	}
	hash := sha256.Sum256(mustJSON(observed))
	backupType := strings.TrimPrefix(operationType, "backup_")
	return &guardedPreflight{
		observed: observed,
		desired:  backupDesired{Type: backupType, SchedulerOwner: owner},
		checks:   checks, blockers: blockers,
		plan: []string{
			"recheck one scheduler owner and no pgBackRest lock",
			"run " + backupType + " backup on " + owner,
			"verify pgBackRest repository, WAL, and completed backup inventory",
		},
		affectedNodes: []string{owner},
		confirmation:  "BACKUP " + strings.ToUpper(backupType) + " " + clusterInfo.Name,
		topologyHash:  hex.EncodeToString(hash[:]),
	}, nil
}

func (h *guardedOperationsHandler) backupOperationInputs(ctx context.Context, clusterInfo *storage.Cluster, operationType string, desired []byte) ([]string, []byte, error) {
	var state backupDesired
	expected := strings.TrimPrefix(operationType, "backup_")
	if err := json.Unmarshal(desired, &state); err != nil || state.Type != expected || state.SchedulerOwner == "" {
		return nil, nil, errors.New("backup desired state is invalid")
	}
	envs, extraVars, err := h.baseOperationInputs(ctx, clusterInfo)
	if err != nil {
		return nil, nil, err
	}
	extraVars["patroni_cluster_name"] = clusterInfo.Name
	extraVars["pgbackrest_scheduler_host"] = state.SchedulerOwner
	extraVars["backup_operation_type"] = state.Type
	payload, err := json.Marshal(extraVars)
	return envs, payload, err
}
