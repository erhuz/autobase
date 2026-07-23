package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"time"

	"postgresql-cluster-console/internal/storage"
)

const phase2Playbook = "phase2_pgcluster.yml"

type phase2Params struct {
	UpdateTarget      string `json:"update_target,omitempty"`
	PostgreSQLVersion int32  `json:"postgresql_version,omitempty"`
}

type phase2Desired struct {
	Action               string                   `json:"action"`
	Target               string                   `json:"target,omitempty"`
	InventoryTarget      string                   `json:"inventory_target,omitempty"`
	UpdateTarget         string                   `json:"update_target,omitempty"`
	CurrentVersion       int32                    `json:"current_version,omitempty"`
	PostgreSQLVersion    int32                    `json:"postgresql_version,omitempty"`
	Routing              []operationRoutingTarget `json:"routing"`
	BackupEnabled        bool                     `json:"backup_enabled"`
	BackupScheduler      string                   `json:"backup_scheduler,omitempty"`
	VerificationNodes    []string                 `json:"verification_nodes"`
	MaxCandidateLagBytes int64                    `json:"max_candidate_lag_bytes,omitempty"`
}

type operationBackupSnapshot struct {
	Observed   map[string]any
	Configured bool
	Ready      bool
	Scheduler  string
	LatestFull bool
}

func (h *guardedOperationsHandler) phase2PreflightState(
	ctx context.Context,
	clusterInfo *storage.Cluster,
	operationType, target string,
	params []byte,
) (*guardedPreflight, error) {
	refreshStarted := time.Now().UTC().Add(-2 * time.Second)
	h.clusterWatcher.HandleCluster(ctx, clusterInfo)

	clusterInfo, err := h.db.GetCluster(ctx, clusterInfo.ID)
	if err != nil {
		return nil, err
	}
	servers, err := h.db.GetClusterServers(ctx, clusterInfo.ID)
	if err != nil {
		return nil, err
	}
	active, err := h.db.HasActiveOperation(ctx, clusterInfo.ID)
	if err != nil {
		return nil, err
	}
	backup, err := h.operationBackupState(ctx, clusterInfo)
	if err != nil {
		return nil, err
	}

	var request phase2Params
	paramsOK := len(params) > 0 && json.Unmarshal(params, &request) == nil
	switch operationType {
	case storage.OperationTypeRollingUpdate:
		paramsOK = paramsOK && validUpdateTarget(request.UpdateTarget) && request.PostgreSQLVersion == 0
	case storage.OperationTypePostgreSQLUpgrade:
		paramsOK = paramsOK && request.UpdateTarget == "" &&
			clusterInfo.PostgreVersion >= 14 && clusterInfo.PostgreVersion <= 18 &&
			request.PostgreSQLVersion > clusterInfo.PostgreVersion && request.PostgreSQLVersion <= 18
	case storage.OperationTypeEmergencyFailover:
		paramsOK = len(params) == 0
	default:
		return nil, errors.New("unsupported phase 2 operation")
	}

	inventory, inventoryOK := lifecycleInventory(clusterInfo.Inventory)
	selected, selectedOK := lifecycleInventoryHost{}, false
	if target != "" {
		selected, selectedOK = lifecycleInventoryTarget(inventory, target)
	}

	nodes, leaderCount, healthyCount := operationTopology(servers)
	healthyLeaders, healthyReplicas, allNamed := 0, 0, true
	selectedNode, selectedNodeOK := topologyNode{}, false
	var candidateLag *int64
	verificationNodes, verificationNodesOK := make([]string, 0, len(nodes)), true
	for _, node := range nodes {
		allNamed = allNamed && node.Name != ""
		if healthyStatus(node.Status) {
			if leaderRole(node.Role) {
				healthyLeaders++
			} else if node.Role == "replica" {
				healthyReplicas++
			}
			host, ok := lifecycleInventoryTarget(inventory, node.Name)
			if !ok {
				verificationNodesOK = false
			} else {
				verificationNodes = append(verificationNodes, host.Name)
			}
		}
		if selectedOK && lifecycleHostMatches(selected, node.Name) {
			selectedNode, selectedNodeOK = node, true
		}
	}
	if selectedNodeOK {
		for _, server := range servers {
			if server.Name == selectedNode.Name {
				candidateLag = server.Lag
				break
			}
		}
	}
	sort.Strings(verificationNodes)
	backupScheduler, backupSchedulerOK := backup.Scheduler, !backup.Configured
	if backup.Configured {
		if host, ok := lifecycleInventoryTarget(inventory, backup.Scheduler); ok {
			backupScheduler, backupSchedulerOK = host.Name, true
		}
	}

	dcs := healthDCS(clusterInfo.ExtraVars, clusterInfo.Inventory)
	patroniReachable := storage.GetPatroniConnectStatus(clusterInfo.Flags) == 1
	dcsConfigured := len(dcs.Members) > 0
	routing := primaryRoutingTargets(clusterInfo.ConnectionInfo)
	checks := []preflightCheck{
		{Name: "stored inventory valid", OK: inventoryOK},
		{Name: "topology names resolved", OK: allNamed},
		{Name: "primary routing configured", OK: len(routing) > 0},
		{Name: "backup verification ready", OK: backup.Ready},
		{Name: "backup scheduler inventory resolved", OK: backupSchedulerOK},
		{Name: "topology refreshed now", OK: topologyFresh(servers, refreshStarted)},
		{Name: "no active cluster mutation", OK: !active},
	}

	maxLag, lagPolicyOK := switchoverLagPolicy(clusterInfo.ExtraVars)
	switch operationType {
	case storage.OperationTypeRollingUpdate:
		checks = append(checks,
			preflightCheck{Name: "target omitted for cluster update", OK: target == ""},
			preflightCheck{Name: "supported fixed update target", OK: paramsOK},
			preflightCheck{Name: "healthy leader", OK: leaderCount == 1 && healthyLeaders == 1},
			preflightCheck{Name: "all current members healthy", OK: len(nodes) >= 3 && healthyCount == len(nodes)},
			preflightCheck{Name: "two healthy failover replicas", OK: healthyReplicas >= 2},
			preflightCheck{Name: "DCS configured and Patroni reachable", OK: dcsConfigured && patroniReachable && clusterInfo.Status == storage.ClusterStatusHealthy},
			preflightCheck{Name: "healthy inventory nodes resolved", OK: verificationNodesOK && len(verificationNodes) == len(nodes)},
		)
	case storage.OperationTypePostgreSQLUpgrade:
		checks = append(checks,
			preflightCheck{Name: "target omitted for PostgreSQL upgrade", OK: target == ""},
			preflightCheck{Name: "supported forward PostgreSQL version", OK: paramsOK},
			preflightCheck{Name: "healthy leader", OK: leaderCount == 1 && healthyLeaders == 1},
			preflightCheck{Name: "all current members healthy", OK: len(nodes) >= 3 && healthyCount == len(nodes)},
			preflightCheck{Name: "two healthy failover replicas", OK: healthyReplicas >= 2},
			preflightCheck{Name: "DCS configured and Patroni reachable", OK: dcsConfigured && patroniReachable && clusterInfo.Status == storage.ClusterStatusHealthy},
			preflightCheck{Name: "current full backup available", OK: backup.Configured && backup.Ready && backup.LatestFull},
			preflightCheck{Name: "healthy inventory nodes resolved", OK: verificationNodesOK && len(verificationNodes) == len(nodes)},
		)
	case storage.OperationTypeEmergencyFailover:
		checks = append(checks,
			preflightCheck{Name: "operation parameters omitted", OK: paramsOK},
			preflightCheck{Name: "DCS configured and Patroni reachable", OK: dcsConfigured && patroniReachable},
			preflightCheck{Name: "no healthy leader", OK: healthyLeaders == 0},
			preflightCheck{Name: "selected healthy replica", OK: selectedOK && selectedNodeOK && selectedNode.Role == "replica" && healthyStatus(selectedNode.Status)},
			preflightCheck{Name: "target is replica only", OK: selectedOK && lifecycleReplicaOnly(selected)},
			preflightCheck{Name: "candidate lag policy valid", OK: lagPolicyOK},
			preflightCheck{Name: "candidate lag within policy", OK: candidateLag != nil && *candidateLag >= 0 && *candidateLag <= maxLag},
			preflightCheck{Name: "healthy failover capacity retained", OK: healthyReplicas >= 2},
			preflightCheck{Name: "healthy inventory nodes resolved", OK: verificationNodesOK && len(verificationNodes) == healthyCount && healthyCount >= 2},
		)
	}

	blockers := make([]string, 0)
	for _, check := range checks {
		if !check.OK {
			blockers = append(blockers, check.Name)
		}
	}

	desired := phase2Desired{
		Action: operationType, UpdateTarget: request.UpdateTarget,
		CurrentVersion: clusterInfo.PostgreVersion, PostgreSQLVersion: request.PostgreSQLVersion,
		Routing: routing, BackupEnabled: backup.Configured, BackupScheduler: backupScheduler,
		VerificationNodes: verificationNodes, MaxCandidateLagBytes: maxLag,
	}
	if selectedOK && selectedNodeOK {
		desired.Target = selectedNode.Name
		desired.InventoryTarget = selected.Name
	}
	plan, confirmation := phase2Plan(desired)
	affected := make([]string, 0, len(nodes))
	if operationType == storage.OperationTypeEmergencyFailover {
		if desired.Target != "" {
			affected = append(affected, desired.Target)
		}
	} else {
		for _, node := range nodes {
			affected = append(affected, node.Name)
		}
	}
	hash, err := topologyHash(nodes)
	if err != nil {
		return nil, err
	}
	return &guardedPreflight{
		observed: map[string]any{
			"topology": nodes, "healthy_nodes": healthyCount, "healthy_replicas": healthyReplicas,
			"dcs": map[string]any{
				"type": dcs.Type, "members": dcs.Members,
				"configured": dcsConfigured, "patroni_reachable": patroniReachable,
			},
			"routing": routing, "backup": backup.Observed,
			"current_postgresql_version": clusterInfo.PostgreVersion,
		},
		desired: desired, checks: checks, blockers: blockers, plan: plan, affectedNodes: affected,
		confirmation: confirmation, topologyHash: hash,
	}, nil
}

func (h *guardedOperationsHandler) operationBackupState(ctx context.Context, clusterInfo *storage.Cluster) (operationBackupSnapshot, error) {
	configured := backupEnabled(clusterInfo.ExtraVars)
	snapshot := operationBackupSnapshot{
		Observed:   map[string]any{"configured": configured},
		Configured: configured, Ready: !configured,
	}
	if !configured {
		return snapshot, nil
	}
	evidence, err := h.db.GetBackupEvidence(ctx, clusterInfo.ID)
	if err != nil {
		return operationBackupSnapshot{}, err
	}
	owners, locks := []string{}, []string{}
	if evidence != nil {
		_ = json.Unmarshal(evidence.SchedulerOwners, &owners)
		_ = json.Unmarshal(evidence.Locks, &locks)
	}
	freshFor := 10 * time.Minute
	if h.cfg != nil && h.cfg.Backup.RunEvery > 0 {
		freshFor = 2 * h.cfg.Backup.RunEvery
	}
	fresh := evidence != nil && time.Since(evidence.ObservedAt) >= 0 && time.Since(evidence.ObservedAt) <= freshFor
	if len(owners) == 1 {
		snapshot.Scheduler = owners[0]
	}
	snapshot.Ready = evidence != nil && fresh && evidence.RepositoryReachable && len(owners) == 1 && len(locks) == 0
	snapshot.LatestFull = evidence != nil && evidence.LatestFull != nil
	snapshot.Observed = map[string]any{
		"configured": true, "fresh": fresh,
		"repository_reachable": evidence != nil && evidence.RepositoryReachable,
		"scheduler_owners":     owners, "locks": locks, "latest_full": snapshot.LatestFull,
	}
	return snapshot, nil
}

func phase2Plan(desired phase2Desired) ([]string, string) {
	final := "verify DCS membership, primary routing " + routingSummary(desired.Routing) + ", and pgBackRest"
	switch desired.Action {
	case storage.OperationTypeRollingUpdate:
		return []string{
			"update and verify replicas one at a time",
			"controlled switchover before updating former leader",
			"apply fixed " + desired.UpdateTarget + " package update",
			final,
		}, "ROLLING UPDATE " + desired.UpdateTarget
	case storage.OperationTypePostgreSQLUpgrade:
		return []string{
				"run schema and pg_upgrade compatibility checks",
				"enter maintenance and upgrade PostgreSQL " + strconv.Itoa(int(desired.CurrentVersion)) + " to " + strconv.Itoa(int(desired.PostgreSQLVersion)),
				"rollback automatically if pg_upgrade fails",
				final,
			}, "UPGRADE POSTGRESQL " + strconv.Itoa(int(desired.CurrentVersion)) + " TO " +
				strconv.Itoa(int(desired.PostgreSQLVersion)) + " WITH DOWNTIME"
	default:
		return []string{
			"recheck DCS has no healthy leader and target remains a healthy replica",
			"promote " + desired.Target + "; unreplicated transactions may be lost",
			final,
		}, "EMERGENCY FAILOVER TO " + desired.Target + " ACCEPT POSSIBLE DATA LOSS"
	}
}

func (h *guardedOperationsHandler) phase2OperationInputs(
	ctx context.Context,
	clusterInfo *storage.Cluster,
	operationType string,
	desired []byte,
) ([]string, []byte, error) {
	var state phase2Desired
	if err := json.Unmarshal(desired, &state); err != nil || state.Action != operationType || len(state.Routing) == 0 {
		return nil, nil, errors.New("phase 2 desired state is invalid")
	}
	switch operationType {
	case storage.OperationTypeRollingUpdate:
		if state.Target != "" || state.InventoryTarget != "" || !validUpdateTarget(state.UpdateTarget) ||
			state.PostgreSQLVersion != 0 || len(state.VerificationNodes) < 3 {
			return nil, nil, errors.New("rolling update desired state is invalid")
		}
	case storage.OperationTypePostgreSQLUpgrade:
		if state.Target != "" || state.InventoryTarget != "" || state.UpdateTarget != "" ||
			state.CurrentVersion < 14 || state.PostgreSQLVersion <= state.CurrentVersion || state.PostgreSQLVersion > 18 ||
			!state.BackupEnabled || state.BackupScheduler == "" || len(state.VerificationNodes) < 3 {
			return nil, nil, errors.New("PostgreSQL upgrade desired state is invalid")
		}
	case storage.OperationTypeEmergencyFailover:
		if state.Target == "" || state.InventoryTarget == "" || state.UpdateTarget != "" ||
			state.PostgreSQLVersion != 0 || len(state.VerificationNodes) < 2 {
			return nil, nil, errors.New("emergency failover desired state is invalid")
		}
	default:
		return nil, nil, errors.New("unsupported phase 2 operation")
	}

	envs, extraVars, err := h.baseOperationInputs(ctx, clusterInfo)
	if err != nil {
		return nil, nil, err
	}
	extraVars["phase2_operation"] = operationType
	extraVars["patroni_cluster_name"] = clusterInfo.Name
	extraVars["operation_primary_routing_targets"] = state.Routing
	extraVars["phase2_pgbackrest_enabled"] = state.BackupEnabled
	extraVars["phase2_verification_nodes"] = state.VerificationNodes
	if state.BackupEnabled {
		extraVars["pgbackrest_scheduler_host"] = state.BackupScheduler
	}
	switch operationType {
	case storage.OperationTypeRollingUpdate:
		extraVars["target"] = state.UpdateTarget
	case storage.OperationTypePostgreSQLUpgrade:
		extraVars["postgresql_version"] = state.CurrentVersion
		extraVars["pg_old_version"] = state.CurrentVersion
		extraVars["pg_new_version"] = state.PostgreSQLVersion
	case storage.OperationTypeEmergencyFailover:
		extraVars["emergency_failover_candidate"] = state.Target
		extraVars["emergency_failover_inventory_target"] = state.InventoryTarget
	}
	payload, err := json.Marshal(extraVars)
	return envs, payload, err
}

func phase2OperationParams(operationType string, desired []byte) ([]byte, error) {
	var state phase2Desired
	if err := json.Unmarshal(desired, &state); err != nil || state.Action != operationType {
		return nil, errors.New("phase 2 desired state is invalid")
	}
	switch operationType {
	case storage.OperationTypeRollingUpdate:
		if !validUpdateTarget(state.UpdateTarget) {
			return nil, errors.New("rolling update desired state is invalid")
		}
		return mustJSON(phase2Params{UpdateTarget: state.UpdateTarget}), nil
	case storage.OperationTypePostgreSQLUpgrade:
		if state.PostgreSQLVersion <= state.CurrentVersion || state.PostgreSQLVersion > 18 {
			return nil, errors.New("PostgreSQL upgrade desired state is invalid")
		}
		return mustJSON(phase2Params{PostgreSQLVersion: state.PostgreSQLVersion}), nil
	case storage.OperationTypeEmergencyFailover:
		return nil, nil
	default:
		return nil, errors.New("unsupported phase 2 operation")
	}
}

func phase2DesiredTarget(desired []byte) (string, error) {
	var state phase2Desired
	if err := json.Unmarshal(desired, &state); err != nil || state.Target == "" {
		return "", errors.New("emergency failover target is required")
	}
	return state.Target, nil
}

func validUpdateTarget(target string) bool {
	return target == "postgres" || target == "patroni" || target == "system"
}
