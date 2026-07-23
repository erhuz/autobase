package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"postgresql-cluster-console/internal/storage"
)

const lifecyclePlaybook = "lifecycle_pgcluster.yml"

type postgresqlParameter struct {
	Option string `json:"option"`
	Value  string `json:"value"`
}

type lifecycleParams struct {
	PostgreSQLParameters []postgresqlParameter `json:"postgresql_parameters"`
}

type lifecycleDesired struct {
	Action               string                   `json:"action"`
	Target               string                   `json:"target,omitempty"`
	PostgreSQLParameters []postgresqlParameter    `json:"postgresql_parameters,omitempty"`
	Routing              []operationRoutingTarget `json:"routing"`
	BackupEnabled        bool                     `json:"backup_enabled"`
	BackupScheduler      string                   `json:"backup_scheduler,omitempty"`
}

type lifecycleInventoryHost struct {
	Name     string
	Hostname string
	Groups   []string
	New      bool
}

var lifecycleParameterValues = map[string]*regexp.Regexp{
	"autovacuum_analyze_scale_factor":         regexp.MustCompile(`^(0(\.\d+)?|1(\.0+)?)$`),
	"autovacuum_vacuum_scale_factor":          regexp.MustCompile(`^(0(\.\d+)?|1(\.0+)?)$`),
	"deadlock_timeout":                        regexp.MustCompile(`^\d+(us|ms|s|min|h|d)?$`),
	"default_statistics_target":               regexp.MustCompile(`^\d+$`),
	"effective_cache_size":                    regexp.MustCompile(`^\d+(B|kB|MB|GB|TB)?$`),
	"idle_in_transaction_session_timeout":     regexp.MustCompile(`^\d+(us|ms|s|min|h|d)?$`),
	"log_checkpoints":                         regexp.MustCompile(`^(on|off|true|false)$`),
	"log_connections":                         regexp.MustCompile(`^(on|off|true|false)$`),
	"log_disconnections":                      regexp.MustCompile(`^(on|off|true|false)$`),
	"log_lock_waits":                          regexp.MustCompile(`^(on|off|true|false)$`),
	"log_min_duration_statement":              regexp.MustCompile(`^(-1|0|\d+(us|ms|s|min|h|d))$`),
	"log_statement":                           regexp.MustCompile(`^(none|ddl|mod|all)$`),
	"lock_timeout":                            regexp.MustCompile(`^\d+(us|ms|s|min|h|d)?$`),
	"maintenance_work_mem":                    regexp.MustCompile(`^\d+(B|kB|MB|GB|TB)?$`),
	"random_page_cost":                        regexp.MustCompile(`^\d+(\.\d+)?$`),
	"seq_page_cost":                           regexp.MustCompile(`^\d+(\.\d+)?$`),
	"statement_timeout":                       regexp.MustCompile(`^\d+(us|ms|s|min|h|d)?$`),
	"work_mem":                                regexp.MustCompile(`^\d+(B|kB|MB|GB|TB)?$`),
}

func (h *guardedOperationsHandler) lifecyclePreflightState(
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

	parameters, parametersOK := []postgresqlParameter(nil), true
	if operationType == storage.OperationTypeConfigUpdate {
		var request lifecycleParams
		if err = json.Unmarshal(params, &request); err != nil {
			parametersOK = false
		} else {
			parameters = request.PostgreSQLParameters
			parametersOK = validLifecycleParameters(parameters)
		}
	}

	inventory, inventoryOK := lifecycleInventory(clusterInfo.Inventory)
	selected, selectedOK := lifecycleInventoryHost{}, false
	if target != "" {
		selected, selectedOK = lifecycleInventoryTarget(inventory, target)
	}
	newNodes := make([]string, 0)
	for _, host := range inventory {
		if host.New {
			newNodes = append(newNodes, host.Name)
		}
	}
	sort.Strings(newNodes)

	nodes, leaderCount, healthyCount := operationTopology(servers)
	leader, healthyReplicas, allNamed := topologyNode{}, 0, true
	selectedNode, selectedNodeOK := topologyNode{}, false
	for _, node := range nodes {
		allNamed = allNamed && node.Name != ""
		if leaderRole(node.Role) {
			leader = node
		} else if healthyStatus(node.Status) {
			healthyReplicas++
		}
		if selectedOK && lifecycleHostMatches(selected, node.Name) {
			selectedNode, selectedNodeOK = node, true
		}
	}

	dcs, dcsReady := operationDCSState(clusterInfo)
	routing := primaryRoutingTargets(clusterInfo.ConnectionInfo)
	backupConfigured := backupEnabled(clusterInfo.ExtraVars)
	backupScheduler, backupReady := "", !backupConfigured
	backupObserved := map[string]any{"configured": backupConfigured}
	if backupConfigured {
		evidence, evidenceErr := h.db.GetBackupEvidence(ctx, clusterInfo.ID)
		if evidenceErr != nil {
			return nil, evidenceErr
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
			backupScheduler = owners[0]
		}
		backupReady = evidence != nil && fresh && evidence.RepositoryReachable && len(owners) == 1 && len(locks) == 0
		backupObserved = map[string]any{
			"configured": true, "fresh": fresh, "repository_reachable": evidence != nil && evidence.RepositoryReachable,
			"scheduler_owners": owners, "locks": locks,
		}
	}

	targetGroups := make([]string, 0)
	if selectedOK {
		targetGroups = selected.Groups
	}
	checks := []preflightCheck{
		{Name: "stored inventory valid", OK: inventoryOK},
		{Name: "healthy leader", OK: leaderCount == 1 && healthyStatus(leader.Status)},
		{Name: "all current members healthy", OK: len(nodes) >= 2 && healthyCount == len(nodes)},
		{Name: "topology names resolved", OK: allNamed},
		{Name: "DCS configured and Patroni reachable", OK: dcsReady},
		{Name: "primary routing configured", OK: len(routing) > 0},
		{Name: "backup verification ready", OK: backupReady},
		{Name: "topology refreshed now", OK: topologyFresh(servers, refreshStarted)},
		{Name: "no active cluster mutation", OK: !active},
	}
	switch operationType {
	case storage.OperationTypeNodeAdd:
		checks = append(checks,
			preflightCheck{Name: "operation parameters omitted", OK: len(params) == 0},
			preflightCheck{Name: "selected new inventory node", OK: selectedOK && target != ""},
			preflightCheck{Name: "exactly one new node", OK: len(newNodes) == 1 && selectedOK && newNodes[0] == selected.Name},
			preflightCheck{Name: "new node is replica only", OK: selectedOK && lifecycleReplicaOnly(selected)},
			preflightCheck{Name: "new node not already in topology", OK: selectedOK && !selectedNodeOK},
			preflightCheck{Name: "healthy failover replica retained", OK: healthyReplicas >= 1},
		)
	case storage.OperationTypeNodeRemove:
		checks = append(checks,
			preflightCheck{Name: "operation parameters omitted", OK: len(params) == 0},
			preflightCheck{Name: "selected inventory replica", OK: selectedOK && selectedNodeOK && selectedNode.Role == "replica"},
			preflightCheck{Name: "target is replica only", OK: selectedOK && lifecycleReplicaOnly(selected)},
			preflightCheck{Name: "target is not backup scheduler", OK: !backupConfigured || !lifecycleHostMatches(selected, backupScheduler)},
			preflightCheck{Name: "failover capacity remains after removal", OK: healthyReplicas >= 2},
		)
	case storage.OperationTypeConfigUpdate:
		checks = append(checks,
			preflightCheck{Name: "target omitted for cluster config", OK: target == ""},
			preflightCheck{Name: "supported non-secret PostgreSQL parameters", OK: parametersOK},
			preflightCheck{Name: "two healthy failover replicas", OK: healthyReplicas >= 2},
		)
	}

	blockers := make([]string, 0)
	for _, check := range checks {
		if !check.OK {
			blockers = append(blockers, check.Name)
		}
	}
	desiredTarget := ""
	if selectedOK {
		desiredTarget = selected.Name
	}
	desired := lifecycleDesired{
		Action: operationType, Target: desiredTarget, PostgreSQLParameters: parameters,
		Routing: routing, BackupEnabled: backupConfigured, BackupScheduler: backupScheduler,
	}
	affected := make([]string, 0, len(nodes)+1)
	if operationType == storage.OperationTypeConfigUpdate {
		for _, node := range nodes {
			affected = append(affected, node.Name)
		}
	} else if desiredTarget != "" {
		affected = append(affected, desiredTarget)
	}
	plan, confirmation := lifecyclePlan(clusterInfo.Name, desired, routing)
	hash, err := topologyHash(nodes)
	if err != nil {
		return nil, err
	}
	return &guardedPreflight{
		observed: map[string]any{
			"topology": nodes, "healthy_replicas": healthyReplicas, "dcs": dcs, "routing": routing,
			"inventory_target": desiredTarget, "target_groups": targetGroups, "new_nodes": newNodes, "backup": backupObserved,
		},
		desired: desired, checks: checks, blockers: blockers, plan: plan, affectedNodes: affected,
		confirmation: confirmation, topologyHash: hash,
	}, nil
}

func (h *guardedOperationsHandler) lifecycleOperationInputs(
	ctx context.Context,
	clusterInfo *storage.Cluster,
	operationType string,
	desired []byte,
) ([]string, []byte, error) {
	var state lifecycleDesired
	if err := json.Unmarshal(desired, &state); err != nil || state.Action != operationType || len(state.Routing) == 0 ||
		(state.BackupEnabled && state.BackupScheduler == "") {
		return nil, nil, errors.New("lifecycle desired state is invalid")
	}
	switch operationType {
	case storage.OperationTypeNodeAdd, storage.OperationTypeNodeRemove:
		if state.Target == "" || len(state.PostgreSQLParameters) != 0 {
			return nil, nil, errors.New("lifecycle target is invalid")
		}
	case storage.OperationTypeConfigUpdate:
		if state.Target != "" || !validLifecycleParameters(state.PostgreSQLParameters) {
			return nil, nil, errors.New("lifecycle parameters are invalid")
		}
	default:
		return nil, nil, errors.New("unsupported lifecycle operation")
	}

	envs, extraVars, err := h.baseOperationInputs(ctx, clusterInfo)
	if err != nil {
		return nil, nil, err
	}
	extraVars["lifecycle_operation"] = operationType
	extraVars["lifecycle_target"] = state.Target
	extraVars["patroni_cluster_name"] = clusterInfo.Name
	extraVars["operation_primary_routing_targets"] = state.Routing
	extraVars["lifecycle_pgbackrest_enabled"] = state.BackupEnabled
	if state.BackupEnabled {
		extraVars["pgbackrest_scheduler_host"] = state.BackupScheduler
	}
	if operationType == storage.OperationTypeNodeRemove {
		extraVars["node_to_remove"] = state.Target
	}
	if operationType == storage.OperationTypeConfigUpdate {
		extraVars["postgresql_parameters_overrides"] = state.PostgreSQLParameters
	}
	payload, err := json.Marshal(extraVars)
	return envs, payload, err
}

func operationParams(operationType string, desired []byte) ([]byte, error) {
	if operationType != storage.OperationTypeConfigUpdate {
		return nil, nil
	}
	var state lifecycleDesired
	if err := json.Unmarshal(desired, &state); err != nil || !validLifecycleParameters(state.PostgreSQLParameters) {
		return nil, errors.New("lifecycle parameters are invalid")
	}
	return mustJSON(lifecycleParams{PostgreSQLParameters: state.PostgreSQLParameters}), nil
}

func lifecyclePlan(clusterName string, desired lifecycleDesired, routing []operationRoutingTarget) ([]string, string) {
	final := []string{
		"recheck Patroni roles and failover capacity",
		"verify DCS membership, primary routing " + routingSummary(routing) + ", and pgBackRest",
	}
	switch desired.Action {
	case storage.OperationTypeNodeAdd:
		return append([]string{"add inventory replica " + desired.Target}, final...), "ADD REPLICA " + desired.Target
	case storage.OperationTypeNodeRemove:
		return append([]string{"remove inventory replica " + desired.Target + " and delete local PostgreSQL data"}, final...),
			"REMOVE REPLICA " + desired.Target + " DELETE LOCAL DATA"
	default:
		options := make([]string, len(desired.PostgreSQLParameters))
		for i, parameter := range desired.PostgreSQLParameters {
			options[i] = parameter.Option
		}
		return append([]string{"apply supported PostgreSQL parameters to " + clusterName}, final...),
			"CONFIGURE POSTGRESQL " + strings.Join(options, ",")
	}
}

func validLifecycleParameters(parameters []postgresqlParameter) bool {
	if len(parameters) == 0 {
		return false
	}
	seen := make(map[string]bool, len(parameters))
	for _, parameter := range parameters {
		pattern, supported := lifecycleParameterValues[parameter.Option]
		if !supported || seen[parameter.Option] || !pattern.MatchString(parameter.Value) {
			return false
		}
		seen[parameter.Option] = true
	}
	return true
}

func lifecycleInventory(raw []byte) (map[string]lifecycleInventoryHost, bool) {
	var inventory struct {
		All struct {
			Children map[string]struct {
				Hosts map[string]map[string]any `json:"hosts"`
			} `json:"children"`
		} `json:"all"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &inventory) != nil {
		return nil, false
	}
	hosts := map[string]lifecycleInventoryHost{}
	for group, members := range inventory.All.Children {
		for name, values := range members.Hosts {
			host := hosts[name]
			host.Name = name
			host.Groups = append(host.Groups, group)
			if hostname, ok := values["hostname"].(string); ok {
				host.Hostname = hostname
			}
			if newNode, ok := values["new_node"].(bool); ok && newNode {
				host.New = true
			}
			hosts[name] = host
		}
	}
	for name, host := range hosts {
		sort.Strings(host.Groups)
		hosts[name] = host
	}
	return hosts, len(hosts) > 0
}

func lifecycleInventoryTarget(inventory map[string]lifecycleInventoryHost, target string) (lifecycleInventoryHost, bool) {
	var selected lifecycleInventoryHost
	found := false
	for _, host := range inventory {
		if lifecycleHostMatches(host, target) {
			if found && selected.Name != host.Name {
				return lifecycleInventoryHost{}, false
			}
			selected, found = host, true
		}
	}
	return selected, found
}

func lifecycleHostMatches(host lifecycleInventoryHost, target string) bool {
	return target != "" && (target == host.Name || target == host.Hostname)
}

func lifecycleReplicaOnly(host lifecycleInventoryHost) bool {
	return len(host.Groups) == 1 && host.Groups[0] == "replica"
}

func lifecycleDesiredTarget(desired []byte) (string, error) {
	var state lifecycleDesired
	if err := json.Unmarshal(desired, &state); err != nil || state.Target == "" {
		return "", fmt.Errorf("lifecycle target is required")
	}
	return state.Target, nil
}
