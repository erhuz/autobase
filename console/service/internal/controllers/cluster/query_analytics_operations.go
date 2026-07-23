package cluster

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"postgresql-cluster-console/internal/storage"
	"postgresql-cluster-console/models"

	"github.com/go-openapi/strfmt"
	"github.com/jackc/pgx/v5"
)

const queryAnalyticsPlaybook = "query_analytics.yml"

type preflightCheck struct {
	Name string `json:"name"`
	OK   bool   `json:"ok"`
}

type topologyNode struct {
	Name     string `json:"name"`
	Role     string `json:"role"`
	Status   string `json:"status"`
	Timeline *int64 `json:"timeline,omitempty"`
}

func (h *guardedOperationsHandler) queryAnalyticsPreflightState(ctx context.Context, clusterInfo *storage.Cluster, operationType string) (*guardedPreflight, error) {
	refreshStarted := time.Now().UTC().Add(-2 * time.Second)
	h.clusterWatcher.HandleCluster(ctx, clusterInfo)
	servers, err := h.db.GetClusterServers(ctx, clusterInfo.ID)
	if err != nil {
		return nil, err
	}
	state, ok := queryAnalyticsState(operationType)
	if !ok {
		return nil, errors.New("unsupported operation type")
	}
	targetEnabled := state == "enabled"
	active, err := h.db.HasActiveOperation(ctx, clusterInfo.ID)
	if err != nil {
		return nil, err
	}

	nodes, leaderCount, healthyCount := operationTopology(servers)
	allNamed := true
	for _, node := range nodes {
		allNamed = allNamed && node.Name != ""
	}
	checks := []preflightCheck{
		{Name: "PostgreSQL 14-18", OK: clusterInfo.PostgreVersion >= 14 && clusterInfo.PostgreVersion <= 18},
		{Name: "at least three healthy nodes", OK: healthyCount >= 3},
		{Name: "all nodes healthy", OK: healthyCount == len(nodes)},
		{Name: "exactly one leader", OK: leaderCount == 1},
		{Name: "topology names resolved", OK: len(nodes) >= 3 && allNamed},
		{Name: "topology refreshed now", OK: topologyFresh(servers, refreshStarted)},
		{Name: "no active cluster mutation", OK: !active},
		{Name: "state change required", OK: clusterInfo.QueryAnalyticsDesired != targetEnabled},
	}
	if !targetEnabled {
		checks = append(checks, preflightCheck{Name: "query analytics managed", OK: clusterInfo.QueryAnalyticsManaged})
	}
	blockers := make([]string, 0)
	for _, check := range checks {
		if !check.OK {
			blockers = append(blockers, check.Name)
		}
	}
	plan, affected := queryAnalyticsPlan(nodes)
	confirmation := "DISABLE QUERY ANALYTICS"
	if targetEnabled {
		confirmation = "ENABLE QUERY ANALYTICS"
	}
	observed := map[string]any{
		"managed": clusterInfo.QueryAnalyticsManaged, "desired": clusterInfo.QueryAnalyticsDesired,
		"postgres_version": clusterInfo.PostgreVersion, "healthy_nodes": healthyCount,
		"node_count": len(nodes), "topology": nodes,
	}
	desired := map[string]any{"managed": true, "state": state, "extension_version": "2.3.2"}
	hash, err := topologyHash(nodes)
	if err != nil {
		return nil, err
	}
	return &guardedPreflight{
		observed: observed, desired: desired, checks: checks, blockers: blockers,
		plan: plan, affectedNodes: affected, confirmation: confirmation, topologyHash: hash,
	}, nil
}

func queryAnalyticsState(operationType string) (string, bool) {
	switch operationType {
	case storage.OperationTypeQueryAnalyticsEnable:
		return "enabled", true
	case storage.OperationTypeQueryAnalyticsDisable:
		return "disabled", true
	default:
		return "", false
	}
}

func (h *guardedOperationsHandler) queryAnalyticsOperationInputs(ctx context.Context, clusterInfo *storage.Cluster, state string) ([]string, []byte, error) {
	extraVars := map[string]any{}
	if len(clusterInfo.ExtraVars) != 0 {
		if err := json.Unmarshal(clusterInfo.ExtraVars, &extraVars); err != nil {
			return nil, nil, err
		}
	}
	extraVars["query_analytics_state"] = state
	extraVars["enable_pg_stat_monitor"] = state == "enabled"
	extraVars["pg_stat_monitor_version"] = "2.3.2"
	extraVars["query_analytics_monitor_username"] = h.cfg.QueryAnalytics.Username
	extraVars["query_analytics_collector_cidrs"] = h.cfg.QueryAnalytics.CollectorCIDRs
	extraVars["patroni_cluster_name"] = clusterInfo.Name

	envs := []string{"ANSIBLE_JSON_LOG_FILE=" + ansibleLogDir + "/" + clusterInfo.Name + ".json"}
	if len(clusterInfo.Inventory) != 0 {
		envs = append(envs, "ANSIBLE_INVENTORY_JSON="+base64.StdEncoding.EncodeToString(clusterInfo.Inventory))
	}
	if clusterInfo.SecretID != nil {
		secretValues, location, err := getSecretEnvs(ctx, h.log, h.db, *clusterInfo.SecretID, h.cfg.EncryptionKey)
		if err != nil {
			return nil, nil, err
		}
		if location == ExtraVarsParamLocation {
			for _, value := range secretValues {
				parts := strings.SplitN(value, "=", 2)
				if len(parts) == 2 {
					extraVars[parts[0]] = parts[1]
				}
			}
		} else {
			envs = append(envs, secretValues...)
		}
	}
	if state == "enabled" {
		password, err := h.db.GetQueryAnalyticsCredential(ctx, clusterInfo.ID, h.cfg.EncryptionKey)
		if errors.Is(err, pgx.ErrNoRows) {
			password, err = randomMonitoringPassword()
			if err == nil {
				err = h.db.SetQueryAnalyticsCredential(ctx, clusterInfo.ID, password, h.cfg.EncryptionKey)
			}
		}
		if err != nil {
			return nil, nil, err
		}
		extraVars["query_analytics_monitor_password"] = password
	}
	encoded, err := json.Marshal(extraVars)
	return envs, encoded, err
}

func operationTopology(servers []storage.Server) ([]topologyNode, int, int) {
	nodes := make([]topologyNode, 0, len(servers))
	leaders, healthy := 0, 0
	for _, server := range servers {
		role, status := strings.ToLower(server.Role), strings.ToLower(server.Status)
		if role == "leader" || role == "primary" || role == "master" {
			leaders++
		}
		if status == "running" || status == "streaming" {
			healthy++
		}
		nodes = append(nodes, topologyNode{Name: server.Name, Role: role, Status: status, Timeline: server.Timeline})
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Name < nodes[j].Name })
	return nodes, leaders, healthy
}

func topologyFresh(servers []storage.Server, since time.Time) bool {
	if len(servers) == 0 {
		return false
	}
	for _, server := range servers {
		if server.UpdatedAt == nil || server.UpdatedAt.Before(since) {
			return false
		}
	}
	return true
}

func queryAnalyticsPlan(nodes []topologyNode) ([]string, []string) {
	plan, affected := []string{}, []string{}
	var leader string
	for _, node := range nodes {
		if node.Role == "leader" || node.Role == "primary" || node.Role == "master" {
			leader = node.Name
			continue
		}
		plan = append(plan, "configure and verify replica "+node.Name)
		affected = append(affected, node.Name)
	}
	plan = append(plan, "controlled switchover from "+leader, "configure and verify former leader "+leader, "verify Patroni health and extension state on every node")
	affected = append(affected, leader)
	return plan, affected
}

func topologyHash(nodes []topologyNode) (string, error) {
	payload, err := json.Marshal(nodes)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func mustJSON(value any) []byte {
	payload, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return payload
}

func preflightModel(preflight *storage.OperationPreflight) *models.ResponseOperationPreflight {
	var observed, desired any
	var checks []preflightCheck
	var blockers, plan, affected []string
	_ = json.Unmarshal(preflight.Observed, &observed)
	_ = json.Unmarshal(preflight.Desired, &desired)
	_ = json.Unmarshal(preflight.Checks, &checks)
	_ = json.Unmarshal(preflight.Blockers, &blockers)
	_ = json.Unmarshal(preflight.Plan, &plan)
	_ = json.Unmarshal(preflight.AffectedNodes, &affected)
	modelChecks := make([]*models.ResponsePreflightCheck, len(checks))
	for i, check := range checks {
		modelChecks[i] = &models.ResponsePreflightCheck{Name: check.Name, Ok: check.OK}
	}
	return &models.ResponseOperationPreflight{
		ID: preflight.ID, Type: preflight.Type, Observed: observed, Desired: desired, Checks: modelChecks,
		Blockers: blockers, Plan: plan, AffectedNodes: affected, Confirmation: preflight.Confirmation,
		ExpiresAt: strfmt.DateTime(preflight.ExpiresAt),
	}
}

func randomMonitoringPassword() (string, error) {
	bytes := make([]byte, 24)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
