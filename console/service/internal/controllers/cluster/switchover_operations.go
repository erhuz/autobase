package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"postgresql-cluster-console/internal/storage"
)

const (
	switchoverPlaybook      = "switchover_pgcluster.yml"
	defaultSwitchoverMaxLag = int64(1 << 20)
)

type switchoverRoutingTarget struct {
	Address string `json:"address"`
	Port    int64  `json:"port"`
}

type switchoverDesired struct {
	Target          string                    `json:"target"`
	PreviousLeader  string                    `json:"previous_leader"`
	MaxCandidateLag int64                     `json:"max_candidate_lag"`
	Routing         []switchoverRoutingTarget `json:"routing"`
}

func (h *guardedOperationsHandler) switchoverPreflightState(ctx context.Context, clusterInfo *storage.Cluster, target string) (*guardedPreflight, error) {
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

	nodes, leaderCount, _ := operationTopology(servers)
	var leader, candidate topologyNode
	var candidateLag *int64
	candidateFound := false
	for _, node := range nodes {
		if leaderRole(node.Role) {
			leader = node
		}
		if node.Name == target {
			candidate, candidateFound = node, true
		}
	}
	for _, server := range servers {
		if server.Name == target {
			candidateLag = server.Lag
			break
		}
	}
	maxLag, validLagPolicy := switchoverLagPolicy(clusterInfo.ExtraVars)
	candidateHealthy := candidateFound && !leaderRole(candidate.Role) && healthyStatus(candidate.Status)
	candidateLagOK := candidateHealthy && candidateLag != nil && *candidateLag >= 0 && *candidateLag <= maxLag
	dcs := healthDCS(clusterInfo.ExtraVars, clusterInfo.Inventory)
	dcsReady := len(dcs.Members) > 0 &&
		storage.GetPatroniConnectStatus(clusterInfo.Flags) == 1 &&
		clusterInfo.Status == storage.ClusterStatusHealthy
	routing := primaryRoutingTargets(clusterInfo.ConnectionInfo)

	checks := []preflightCheck{
		{Name: "selected replica", OK: target != "" && candidateFound},
		{Name: "healthy leader", OK: leaderCount == 1 && healthyStatus(leader.Status)},
		{Name: "selected replica healthy", OK: candidateHealthy},
		{Name: "candidate lag policy valid", OK: validLagPolicy},
		{Name: "candidate lag within policy", OK: candidateLagOK},
		{Name: "DCS configured and Patroni reachable", OK: dcsReady},
		{Name: "primary routing configured", OK: len(routing) > 0},
		{Name: "topology refreshed now", OK: topologyFresh(servers, refreshStarted)},
		{Name: "no active cluster mutation", OK: !active},
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
	plan := []string{
		"verify Patroni DCS access",
		fmt.Sprintf("promote selected replica %s", target),
		fmt.Sprintf("demote current leader %s", leader.Name),
		"verify new leader and every replica",
		fmt.Sprintf("verify primary routing %s", routingSummary(routing)),
	}
	observed := map[string]any{
		"leader": leader.Name, "candidate": candidate, "candidate_lag": candidateLag, "topology": nodes,
		"dcs": map[string]any{
			"type": dcs.Type, "members": dcs.Members,
			"configured":        len(dcs.Members) > 0,
			"patroni_reachable": storage.GetPatroniConnectStatus(clusterInfo.Flags) == 1,
		},
		"routing": routing,
	}
	desired := switchoverDesired{
		Target: target, PreviousLeader: leader.Name, MaxCandidateLag: maxLag, Routing: routing,
	}
	hash, err := topologyHash(nodes)
	if err != nil {
		return nil, err
	}
	return &guardedPreflight{
		observed: observed, desired: desired, checks: checks, blockers: blockers,
		plan: plan, affectedNodes: affected,
		confirmation: "SWITCHOVER " + leader.Name + " TO " + target,
		topologyHash: hash,
	}, nil
}

func operationTarget(operationType string, desired []byte) (string, error) {
	if operationType != storage.OperationTypeSwitchover {
		return "", nil
	}
	var state switchoverDesired
	if err := json.Unmarshal(desired, &state); err != nil || state.Target == "" {
		return "", errors.New("switchover target is required")
	}
	return state.Target, nil
}

func (h *guardedOperationsHandler) switchoverOperationInputs(ctx context.Context, clusterInfo *storage.Cluster, desired []byte) ([]string, []byte, error) {
	var state switchoverDesired
	if err := json.Unmarshal(desired, &state); err != nil || state.Target == "" || len(state.Routing) == 0 {
		return nil, nil, errors.New("switchover desired state is invalid")
	}
	envs, extraVars, err := h.baseOperationInputs(ctx, clusterInfo)
	if err != nil {
		return nil, nil, err
	}
	extraVars["patroni_cluster_name"] = clusterInfo.Name
	extraVars["patroni_switchover_candidate_name"] = state.Target
	extraVars["switchover_primary_routing_targets"] = state.Routing
	payload, err := json.Marshal(extraVars)
	return envs, payload, err
}

func switchoverLagPolicy(extraVars []byte) (int64, bool) {
	if len(extraVars) == 0 {
		return defaultSwitchoverMaxLag, true
	}
	var values map[string]any
	if err := json.Unmarshal(extraVars, &values); err != nil {
		return 0, false
	}
	raw, ok := values["patroni_maximum_lag_on_failover"]
	if !ok {
		return defaultSwitchoverMaxLag, true
	}
	switch value := raw.(type) {
	case float64:
		if value > 0 && value <= 1<<53 && value == float64(int64(value)) {
			return int64(value), true
		}
	case string:
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err == nil && parsed > 0 {
			return parsed, true
		}
	}
	return 0, false
}

func primaryRoutingTargets(connectionInfo any) []switchoverRoutingTarget {
	routing := healthRouting(connectionInfo)
	targets := make([]switchoverRoutingTarget, 0)
	for _, target := range routing.Targets {
		if target.Role == "primary" && target.Address != "" && target.Port != nil {
			targets = append(targets, switchoverRoutingTarget{Address: target.Address, Port: *target.Port})
		}
	}
	return targets
}

func routingSummary(targets []switchoverRoutingTarget) string {
	values := make([]string, len(targets))
	for i, target := range targets {
		values[i] = target.Address + ":" + strconv.FormatInt(target.Port, 10)
	}
	return strings.Join(values, ", ")
}
