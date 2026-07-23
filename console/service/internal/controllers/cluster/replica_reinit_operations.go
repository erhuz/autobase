package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"postgresql-cluster-console/internal/storage"
)

const (
	replicaReinitPlaybook    = "reinit_replica.yml"
	replicaReinitCloneMethod = "pg_basebackup"
)

type replicaReinitDesired struct {
	Target      string `json:"target"`
	CloneSource string `json:"clone_source"`
	CloneMethod string `json:"clone_method"`
	MaxLagBytes int64  `json:"max_lag_bytes"`
	DataLoss    bool   `json:"data_loss"`
}

func (h *guardedOperationsHandler) replicaReinitPreflightState(ctx context.Context, clusterInfo *storage.Cluster, target string) (*guardedPreflight, error) {
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
	leader, selected := topologyNode{}, topologyNode{}
	targetFound, allNamed, healthyOthers := false, true, 0
	for _, node := range nodes {
		allNamed = allNamed && node.Name != ""
		if leaderRole(node.Role) {
			leader = node
		}
		if node.Name == target {
			selected, targetFound = node, true
		} else if healthyStatus(node.Status) {
			healthyOthers++
		}
	}
	dcs := healthDCS(clusterInfo.ExtraVars, clusterInfo.Inventory)
	dcsReady := len(dcs.Members) > 0 && storage.GetPatroniConnectStatus(clusterInfo.Flags) == 1
	maxLag, validLagPolicy := switchoverLagPolicy(clusterInfo.ExtraVars)
	checks := []preflightCheck{
		{Name: "selected target is current replica", OK: targetFound && selected.Role == "replica"},
		{Name: "healthy clone source leader", OK: leaderCount == 1 && healthyStatus(leader.Status)},
		{Name: "another healthy member", OK: healthyOthers >= 1},
		{Name: "topology names resolved", OK: len(nodes) >= 2 && allNamed},
		{Name: "DCS configured and Patroni reachable", OK: dcsReady},
		{Name: "replica lag policy valid", OK: validLagPolicy},
		{Name: "topology refreshed now", OK: topologyFresh(servers, refreshStarted)},
		{Name: "no active cluster mutation", OK: !active},
	}
	blockers := make([]string, 0)
	for _, check := range checks {
		if !check.OK {
			blockers = append(blockers, check.Name)
		}
	}
	desired := replicaReinitDesired{
		Target: target, CloneSource: leader.Name, CloneMethod: replicaReinitCloneMethod,
		MaxLagBytes: maxLag, DataLoss: true,
	}
	hash, err := topologyHash(nodes)
	if err != nil {
		return nil, err
	}
	return &guardedPreflight{
		observed: map[string]any{
			"target": selected, "clone_source": leader, "topology": nodes,
			"dcs": map[string]any{
				"type": dcs.Type, "members": dcs.Members,
				"configured":        len(dcs.Members) > 0,
				"patroni_reachable": storage.GetPatroniConnectStatus(clusterInfo.Flags) == 1,
			},
		},
		desired: desired, checks: checks, blockers: blockers,
		plan: []string{
			"recheck " + target + " is a replica",
			"delete local PostgreSQL data on " + target,
			"clone " + target + " from leader " + leader.Name + " with " + replicaReinitCloneMethod,
			fmt.Sprintf("verify replica streaming with lag <= %d bytes", maxLag),
		},
		affectedNodes: []string{target},
		confirmation:  "REINITIALIZE REPLICA " + target + " DELETE LOCAL DATA",
		topologyHash:  hash,
	}, nil
}

func (h *guardedOperationsHandler) replicaReinitOperationInputs(ctx context.Context, clusterInfo *storage.Cluster, desired []byte) ([]string, []byte, error) {
	var state replicaReinitDesired
	if err := json.Unmarshal(desired, &state); err != nil || state.Target == "" || state.CloneSource == "" ||
		state.CloneMethod != replicaReinitCloneMethod || state.MaxLagBytes <= 0 || !state.DataLoss {
		return nil, nil, errors.New("replica reinit desired state is invalid")
	}
	envs, extraVars, err := h.baseOperationInputs(ctx, clusterInfo)
	if err != nil {
		return nil, nil, err
	}
	extraVars["patroni_cluster_name"] = clusterInfo.Name
	extraVars["replica_reinit_target_name"] = state.Target
	extraVars["replica_reinit_clone_source"] = state.CloneSource
	extraVars["replica_reinit_clone_method"] = state.CloneMethod
	extraVars["replica_reinit_max_lag_bytes"] = state.MaxLagBytes
	payload, err := json.Marshal(extraVars)
	return envs, payload, err
}
