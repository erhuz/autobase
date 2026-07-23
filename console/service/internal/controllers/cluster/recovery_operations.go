package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strings"
	"time"

	"postgresql-cluster-console/internal/storage"
)

const recoveryPlaybook = "restore_pgcluster.yml"

var recoveryStanzaPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

type recoveryParams struct {
	SourceCluster      string `json:"source_cluster"`
	RecoveryTargetTime string `json:"recovery_target_time,omitempty"`
}

type recoveryDesired struct {
	Action                 string   `json:"action"`
	SourceCluster          string   `json:"source_cluster"`
	SourceClusterID        int64    `json:"source_cluster_id"`
	RecoveryCluster        string   `json:"recovery_cluster"`
	RecoveryClusterID      int64    `json:"recovery_cluster_id"`
	RecoveryTargetTime     string   `json:"recovery_target_time,omitempty"`
	BackupStanza           string   `json:"backup_stanza"`
	RecoveryNodes          []string `json:"recovery_nodes"`
	RecoveryInventoryNodes []string `json:"recovery_inventory_nodes"`
}

type recoveryInventoryState struct {
	nodes      []string
	inventory  []string
	identities map[string]bool
	masters    int
}

func (h *guardedOperationsHandler) recoveryPreflightState(
	ctx context.Context,
	recoveryCluster *storage.Cluster,
	operationType, target string,
	params []byte,
) (*guardedPreflight, error) {
	var request recoveryParams
	if len(params) == 0 || json.Unmarshal(params, &request) != nil || request.SourceCluster == "" {
		return nil, errors.New("recovery parameters are invalid")
	}
	if operationType != storage.OperationTypeRestore && operationType != storage.OperationTypePITR {
		return nil, errors.New("unsupported recovery operation")
	}
	sourceCluster, err := h.db.GetClusterByName(ctx, request.SourceCluster)
	if err != nil {
		return nil, err
	}

	refreshStarted := time.Now().UTC().Add(-2 * time.Second)
	h.clusterWatcher.HandleCluster(ctx, sourceCluster)
	h.clusterWatcher.HandleCluster(ctx, recoveryCluster)
	sourceCluster, err = h.db.GetCluster(ctx, sourceCluster.ID)
	if err != nil {
		return nil, err
	}
	recoveryCluster, err = h.db.GetCluster(ctx, recoveryCluster.ID)
	if err != nil {
		return nil, err
	}
	sourceServers, err := h.db.GetClusterServers(ctx, sourceCluster.ID)
	if err != nil {
		return nil, err
	}
	recoveryServers, err := h.db.GetClusterServers(ctx, recoveryCluster.ID)
	if err != nil {
		return nil, err
	}
	active, err := h.db.HasActiveOperation(ctx, recoveryCluster.ID)
	if err != nil {
		return nil, err
	}
	backup, err := h.operationBackupState(ctx, sourceCluster)
	if err != nil {
		return nil, err
	}
	evidence, err := h.db.GetBackupEvidence(ctx, sourceCluster.ID)
	if err != nil {
		return nil, err
	}

	sourceInventory, sourceInventoryOK := recoveryInventory(sourceCluster.Inventory)
	recoveryInventory, recoveryInventoryOK := recoveryInventory(recoveryCluster.Inventory)
	inventoriesDisjoint := recoveryInventoriesDisjoint(sourceInventory, recoveryInventory)
	recoveryMarked, stanza, recoveryConfigOK := recoveryConfiguration(sourceCluster, recoveryCluster)
	sourceTopology, _, _ := operationTopology(sourceServers)
	recoveryTopology, _, _ := operationTopology(recoveryServers)

	targetTime, targetTimeOK := time.Time{}, request.RecoveryTargetTime == ""
	if request.RecoveryTargetTime != "" {
		targetTime, err = time.Parse(time.RFC3339, request.RecoveryTargetTime)
		targetTimeOK = err == nil
		if targetTimeOK {
			request.RecoveryTargetTime = targetTime.UTC().Format(time.RFC3339Nano)
		}
	}
	walContinuous := evidence != nil && evidence.WalContinuous != nil && *evidence.WalContinuous
	pitrRangeOK := targetTimeOK && evidence != nil && evidence.LatestFull != nil &&
		!targetTime.Before(*evidence.LatestFull) && !targetTime.After(evidence.ObservedAt)
	paramsOK := target == "" &&
		((operationType == storage.OperationTypeRestore && request.RecoveryTargetTime == "") ||
			(operationType == storage.OperationTypePITR && request.RecoveryTargetTime != "" && pitrRangeOK))

	checks := []preflightCheck{
		{Name: "operation target omitted", OK: target == ""},
		{Name: "recovery parameters valid", OK: paramsOK},
		{Name: "source and recovery clusters differ", OK: sourceCluster.ID != recoveryCluster.ID && sourceCluster.Name != recoveryCluster.Name},
		{Name: "same project recovery boundary", OK: sourceCluster.ProjectID == recoveryCluster.ProjectID},
		{Name: "recovery cluster explicitly marked", OK: recoveryMarked},
		{Name: "source and recovery inventories valid", OK: sourceInventoryOK && recoveryInventoryOK && recoveryInventory.masters == 1},
		{Name: "source and recovery inventories disjoint", OK: inventoriesDisjoint},
		{Name: "source roles refreshed now", OK: topologyFresh(sourceServers, refreshStarted)},
		{Name: "pgBackRest recovery configuration valid", OK: recoveryConfigOK},
		{Name: "current full backup available", OK: backup.Configured && backup.Ready && backup.LatestFull},
		{Name: "WAL continuity verified", OK: operationType != storage.OperationTypePITR || walContinuous},
		{Name: "no active recovery-cluster mutation", OK: !active},
	}
	blockers := make([]string, 0)
	for _, check := range checks {
		if !check.OK {
			blockers = append(blockers, check.Name)
		}
	}

	desired := recoveryDesired{
		Action:        operationType,
		SourceCluster: sourceCluster.Name, SourceClusterID: sourceCluster.ID,
		RecoveryCluster: recoveryCluster.Name, RecoveryClusterID: recoveryCluster.ID,
		RecoveryTargetTime: request.RecoveryTargetTime, BackupStanza: stanza,
		RecoveryNodes: recoveryInventory.nodes, RecoveryInventoryNodes: recoveryInventory.inventory,
	}
	plan, confirmation := recoveryPlan(desired)
	combined := make([]topologyNode, 0, len(sourceTopology)+len(recoveryTopology))
	for _, node := range sourceTopology {
		node.Name = "source/" + node.Name
		combined = append(combined, node)
	}
	for _, node := range recoveryTopology {
		node.Name = "recovery/" + node.Name
		combined = append(combined, node)
	}
	hash, err := topologyHash(combined)
	if err != nil {
		return nil, err
	}
	return &guardedPreflight{
		observed: map[string]any{
			"source_cluster": sourceCluster.Name, "source_topology": sourceTopology,
			"recovery_cluster": recoveryCluster.Name, "recovery_topology": recoveryTopology,
			"inventories_disjoint": inventoriesDisjoint, "backup": backup.Observed,
			"wal_continuous": walContinuous,
		},
		desired: desired, checks: checks, blockers: blockers, plan: plan,
		affectedNodes: recoveryInventory.nodes, confirmation: confirmation, topologyHash: hash,
	}, nil
}

func (h *guardedOperationsHandler) recoveryOperationInputs(
	ctx context.Context,
	clusterInfo *storage.Cluster,
	operationType string,
	desired []byte,
) ([]string, []byte, error) {
	var state recoveryDesired
	if err := json.Unmarshal(desired, &state); err != nil ||
		state.Action != operationType ||
		state.SourceCluster == "" || state.SourceClusterID == clusterInfo.ID ||
		state.RecoveryCluster != clusterInfo.Name || state.RecoveryClusterID != clusterInfo.ID ||
		!recoveryStanzaPattern.MatchString(state.BackupStanza) ||
		len(state.RecoveryNodes) == 0 || len(state.RecoveryInventoryNodes) == 0 {
		return nil, nil, errors.New("recovery desired state is invalid")
	}
	if operationType == storage.OperationTypeRestore && state.RecoveryTargetTime != "" {
		return nil, nil, errors.New("restore desired state is invalid")
	}
	if operationType == storage.OperationTypePITR {
		if _, err := time.Parse(time.RFC3339, state.RecoveryTargetTime); err != nil {
			return nil, nil, errors.New("PITR desired state is invalid")
		}
	}

	envs, extraVars, err := h.baseOperationInputs(ctx, clusterInfo)
	if err != nil {
		return nil, nil, err
	}
	envs = append(envs, "ANSIBLE_RUN_TAGS=point_in_time_recovery")
	restoreCommand := "/usr/bin/pgbackrest --stanza=" + state.BackupStanza
	if operationType == storage.OperationTypePITR {
		restoreCommand += " --type=time --target=" + state.RecoveryTargetTime
	}
	restoreCommand += " restore"
	extraVars["recovery_operation"] = operationType
	extraVars["recovery_isolated"] = true
	extraVars["restore_source_cluster_name"] = state.SourceCluster
	extraVars["restore_target_cluster_name"] = state.RecoveryCluster
	extraVars["recovery_target_time"] = state.RecoveryTargetTime
	extraVars["recovery_verification_nodes"] = state.RecoveryInventoryNodes
	extraVars["cloud_provider"] = ""
	extraVars["patroni_cluster_name"] = state.RecoveryCluster
	extraVars["patroni_cluster_bootstrap_method"] = "pgbackrest"
	extraVars["pgbackrest_install"] = true
	extraVars["pgbackrest_auto_conf"] = false
	extraVars["pgbackrest_stanza"] = state.BackupStanza
	extraVars["pgbackrest_patroni_cluster_restore_command"] = restoreCommand
	extraVars["pgbackrest_patroni_cluster_clean_bootstrap"] = true
	extraVars["disable_archive_command"] = true
	extraVars["keep_patroni_dynamic_json"] = false
	payload, err := json.Marshal(extraVars)
	return envs, payload, err
}

func recoveryOperationParams(operationType string, desired []byte) ([]byte, error) {
	var state recoveryDesired
	if err := json.Unmarshal(desired, &state); err != nil || state.Action != operationType || state.SourceCluster == "" {
		return nil, errors.New("recovery desired state is invalid")
	}
	switch operationType {
	case storage.OperationTypeRestore:
		if state.RecoveryTargetTime != "" {
			return nil, errors.New("restore desired state is invalid")
		}
	case storage.OperationTypePITR:
		if _, err := time.Parse(time.RFC3339, state.RecoveryTargetTime); err != nil {
			return nil, errors.New("PITR desired state is invalid")
		}
	default:
		return nil, errors.New("unsupported recovery operation")
	}
	return mustJSON(recoveryParams{SourceCluster: state.SourceCluster, RecoveryTargetTime: state.RecoveryTargetTime}), nil
}

func recoveryPlan(desired recoveryDesired) ([]string, string) {
	source := desired.SourceCluster
	target := desired.RecoveryCluster
	selection := "latest recoverable pgBackRest state"
	confirmation := "RESTORE " + source + " TO ISOLATED " + target + " DELETE RECOVERY TARGET DATA"
	if desired.Action == storage.OperationTypePITR {
		selection = "pgBackRest point in time " + desired.RecoveryTargetTime
		confirmation = "PITR " + source + " TO " + desired.RecoveryTargetTime + " ON ISOLATED " + target +
			" DELETE RECOVERY TARGET DATA"
	}
	return []string{
		"recheck source roles, backup evidence, and inventory isolation",
		"clean recovery target data and restore " + selection,
		"verify recovery target Patroni leader, DCS membership, and pgBackRest repository",
	}, confirmation
}

func recoveryConfiguration(sourceCluster, recoveryCluster *storage.Cluster) (bool, string, bool) {
	var sourceVars, recoveryVars map[string]any
	if json.Unmarshal(sourceCluster.ExtraVars, &sourceVars) != nil ||
		json.Unmarshal(recoveryCluster.ExtraVars, &recoveryVars) != nil {
		return false, "", false
	}
	recoveryMarked, _ := recoveryVars["recovery_target"].(bool)
	stanza, _ := sourceVars["pgbackrest_stanza"].(string)
	if stanza == "" {
		stanza = sourceCluster.Name
	}
	return recoveryMarked, stanza,
		backupEnabled(sourceCluster.ExtraVars) && backupEnabled(recoveryCluster.ExtraVars) &&
			recoveryStanzaPattern.MatchString(stanza)
}

func recoveryInventory(raw []byte) (recoveryInventoryState, bool) {
	inventory, ok := lifecycleInventory(raw)
	state := recoveryInventoryState{identities: make(map[string]bool)}
	if !ok {
		return state, false
	}
	for _, host := range inventory {
		state.identities[strings.ToLower(host.Name)] = true
		if host.Hostname != "" {
			state.identities[strings.ToLower(host.Hostname)] = true
		}
		postgres, master := false, false
		for _, group := range host.Groups {
			postgres = postgres || group == "master" || group == "replica"
			master = master || group == "master"
		}
		if postgres {
			name := host.Hostname
			if name == "" {
				name = host.Name
			}
			state.nodes = append(state.nodes, name)
			state.inventory = append(state.inventory, host.Name)
		}
		if master {
			state.masters++
		}
	}
	sort.Strings(state.nodes)
	sort.Strings(state.inventory)
	return state, len(state.nodes) > 0
}

func recoveryInventoriesDisjoint(source, recovery recoveryInventoryState) bool {
	if len(source.identities) == 0 || len(recovery.identities) == 0 {
		return false
	}
	for identity := range source.identities {
		if recovery.identities[identity] {
			return false
		}
	}
	return true
}
