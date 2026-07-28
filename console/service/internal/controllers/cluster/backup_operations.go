package cluster

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"sort"
	"strings"
	"time"

	"postgresql-cluster-console/internal/storage"
)

const backupPlaybook = "backup_pgbackrest.yml"

type backupDesired struct {
	Type           string `json:"type"`
	SchedulerOwner string `json:"scheduler_owner"`
}

func (h *guardedOperationsHandler) backupPreflightState(
	ctx context.Context,
	clusterInfo *storage.Cluster,
	operationType, target string,
	params []byte,
) (*guardedPreflight, error) {
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
	affectedNodes := []string{owner}
	reconcile := operationType == storage.OperationTypeBackupSchedulerReconcile
	configuredOwner, schedulerNodes, configuredOwnerOK := configuredBackupScheduler(clusterInfo.ExtraVars, clusterInfo.Inventory)
	if reconcile {
		owner = configuredOwner
		affectedNodes = schedulerNodes
	}
	checks := []preflightCheck{
		{Name: "pgBackRest configured", OK: enabled},
		{Name: "backup evidence fresh", OK: evidenceFresh},
		{Name: "repository reachable", OK: evidence != nil && evidence.RepositoryReachable},
		{Name: "repository has no active lock", OK: len(locks) == 0},
		{Name: "no active cluster mutation", OK: !active},
		{Name: "target omitted", OK: target == ""},
		{Name: "operation parameters omitted", OK: len(params) == 0},
	}
	if reconcile {
		checks = append(checks,
			preflightCheck{Name: "configured scheduler owner resolved", OK: configuredOwnerOK},
			preflightCheck{Name: "scheduler ownership drift observed", OK: configuredOwnerOK && (len(owners) != 1 || owners[0] != configuredOwner)},
		)
	} else {
		checks = append(checks, preflightCheck{Name: "exactly one scheduler owner", OK: len(owners) == 1})
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
		"locks": locks, "scheduler_owners": owners, "configured_scheduler_owner": configuredOwner,
	}
	if evidence != nil {
		observed["observed_at"] = evidence.ObservedAt
		observed["repository_reachable"] = evidence.RepositoryReachable
		observed["latest_full"] = evidence.LatestFull
		observed["latest_differential"] = evidence.LatestDifferential
	}
	hash := sha256.Sum256(mustJSON(observed))
	backupType := backupOperationType(operationType)
	plan := []string{
		"recheck one scheduler owner and no pgBackRest lock",
		"run " + backupType + " backup on " + owner,
		"verify pgBackRest repository, WAL, and completed backup inventory",
	}
	confirmation := "BACKUP " + strings.ToUpper(backupType) + " " + clusterInfo.Name
	if reconcile {
		plan = []string{
			"recheck scheduler ownership drift and no pgBackRest lock",
			"remove pgBackRest cron from non-owner nodes",
			"apply pgBackRest cron on " + owner + " and verify sole ownership",
		}
		confirmation = "RECONCILE BACKUP SCHEDULER " + clusterInfo.Name
	}
	return &guardedPreflight{
		observed: observed,
		desired:  backupDesired{Type: backupType, SchedulerOwner: owner},
		checks:   checks, blockers: blockers,
		plan: plan, affectedNodes: affectedNodes, confirmation: confirmation,
		topologyHash: hex.EncodeToString(hash[:]),
	}, nil
}

func (h *guardedOperationsHandler) backupOperationInputs(ctx context.Context, clusterInfo *storage.Cluster, operationType string, desired []byte) ([]string, []byte, error) {
	var state backupDesired
	expected := backupOperationType(operationType)
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

func backupOperationType(operationType string) string {
	if operationType == storage.OperationTypeBackupSchedulerReconcile {
		return "reconcile"
	}
	return strings.TrimPrefix(operationType, "backup_")
}

func configuredBackupScheduler(extraVars, rawInventory []byte) (string, []string, bool) {
	inventory, inventoryOK := lifecycleInventory(rawInventory)
	if !inventoryOK {
		return "", nil, false
	}
	nodes := make([]string, 0, len(inventory))
	for _, host := range inventory {
		if slices.Contains(host.Groups, "master") || slices.Contains(host.Groups, "replica") ||
			slices.Contains(host.Groups, "postgres_cluster") || slices.Contains(host.Groups, "pgbackrest") {
			nodes = append(nodes, host.Name)
		}
	}
	sort.Strings(nodes)

	var values map[string]any
	if json.Unmarshal(extraVars, &values) != nil {
		return "", nodes, false
	}
	if configured, _ := values["pgbackrest_scheduler_host"].(string); configured != "" {
		host, ok := lifecycleInventoryTarget(inventory, configured)
		return host.Name, nodes, ok && host.Name != ""
	}
	group := "master"
	if repoHost, _ := values["pgbackrest_repo_host"].(string); repoHost != "" {
		group = "pgbackrest"
	}
	candidates := make([]string, 0)
	for _, host := range inventory {
		if slices.Contains(host.Groups, group) {
			candidates = append(candidates, host.Name)
		}
	}
	sort.Strings(candidates)
	if len(candidates) == 0 {
		return "", nodes, false
	}
	return candidates[0], nodes, true
}
