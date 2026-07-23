package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"postgresql-cluster-console/internal/storage"
)

const (
	reloadPlaybook         = "reload_pgcluster.yml"
	rollingRestartPlaybook = "restart_pgcluster.yml"
)

type maintenanceDesired struct {
	Action              string                   `json:"action"`
	ConfigurationChange bool                     `json:"configuration_change"`
	Routing             []operationRoutingTarget `json:"routing"`
}

func (h *guardedOperationsHandler) maintenancePreflightState(ctx context.Context, clusterInfo *storage.Cluster, operationType string) (*guardedPreflight, error) {
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

	nodes, leaderCount, healthyCount := operationTopology(servers)
	leader, healthyReplicas, allNamed := topologyNode{}, 0, true
	for _, node := range nodes {
		allNamed = allNamed && node.Name != ""
		if leaderRole(node.Role) {
			leader = node
		} else if healthyStatus(node.Status) {
			healthyReplicas++
		}
	}
	dcs, dcsReady := operationDCSState(clusterInfo)
	routing := primaryRoutingTargets(clusterInfo.ConnectionInfo)
	checks := []preflightCheck{
		{Name: "healthy leader", OK: leaderCount == 1 && healthyStatus(leader.Status)},
		{Name: "all members healthy", OK: len(nodes) >= 2 && healthyCount == len(nodes)},
		{Name: "topology names resolved", OK: allNamed},
		{Name: "DCS configured and Patroni reachable", OK: dcsReady},
		{Name: "primary routing configured", OK: len(routing) > 0},
		{Name: "topology refreshed now", OK: topologyFresh(servers, refreshStarted)},
		{Name: "no active cluster mutation", OK: !active},
	}
	if operationType == storage.OperationTypeRollingRestart {
		checks = append(checks, preflightCheck{Name: "at least two healthy replicas", OK: healthyReplicas >= 2})
	}
	blockers := make([]string, 0)
	for _, check := range checks {
		if !check.OK {
			blockers = append(blockers, check.Name)
		}
	}
	affected := make([]string, len(nodes))
	for i, node := range nodes {
		affected[i] = node.Name
	}
	plan, confirmation := maintenancePlan(operationType, nodes, leader.Name, routing)
	hash, err := topologyHash(nodes)
	if err != nil {
		return nil, err
	}
	return &guardedPreflight{
		observed: map[string]any{
			"topology": nodes, "healthy_replicas": healthyReplicas,
			"dcs": dcs, "routing": routing,
		},
		desired: maintenanceDesired{
			Action: operationType, ConfigurationChange: false, Routing: routing,
		},
		checks: checks, blockers: blockers, plan: plan, affectedNodes: affected,
		confirmation: confirmation, topologyHash: hash,
	}, nil
}

func maintenancePlan(operationType string, nodes []topologyNode, leader string, routing []operationRoutingTarget) ([]string, string) {
	if operationType == storage.OperationTypeReload {
		return []string{
			"reload PostgreSQL configuration through Patroni",
			"verify leader, replicas, and primary routing " + routingSummary(routing),
		}, "RELOAD CLUSTER"
	}
	plan := make([]string, 0, len(nodes)+3)
	for _, node := range nodes {
		if !leaderRole(node.Role) {
			plan = append(plan, "restart and verify replica "+node.Name)
		}
	}
	plan = append(plan,
		"controlled switchover from "+leader,
		"restart and verify former leader "+leader,
		"verify topology and routing after every stage",
	)
	return plan, "ROLLING RESTART"
}

func (h *guardedOperationsHandler) maintenanceOperationInputs(ctx context.Context, clusterInfo *storage.Cluster, operationType string, desired []byte) ([]string, []byte, string, error) {
	var state maintenanceDesired
	if err := json.Unmarshal(desired, &state); err != nil || state.Action != operationType ||
		state.ConfigurationChange || len(state.Routing) == 0 {
		return nil, nil, "", errors.New("maintenance desired state is invalid")
	}
	envs, extraVars, err := h.baseOperationInputs(ctx, clusterInfo)
	if err != nil {
		return nil, nil, "", err
	}
	extraVars["patroni_cluster_name"] = clusterInfo.Name
	extraVars["operation_primary_routing_targets"] = state.Routing
	playbook := reloadPlaybook
	if operationType == storage.OperationTypeRollingRestart {
		extraVars["patroni_switchover_candidate_name"] = ""
		playbook = rollingRestartPlaybook
	}
	payload, err := json.Marshal(extraVars)
	return envs, payload, playbook, err
}

func operationDCSState(clusterInfo *storage.Cluster) (map[string]any, bool) {
	dcs := healthDCS(clusterInfo.ExtraVars, clusterInfo.Inventory)
	ready := len(dcs.Members) > 0 &&
		storage.GetPatroniConnectStatus(clusterInfo.Flags) == 1 &&
		clusterInfo.Status == storage.ClusterStatusHealthy
	return map[string]any{
		"type": dcs.Type, "members": dcs.Members,
		"configured":        len(dcs.Members) > 0,
		"patroni_reachable": storage.GetPatroniConnectStatus(clusterInfo.Flags) == 1,
	}, ready
}
