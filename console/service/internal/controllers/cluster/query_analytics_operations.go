package cluster

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"postgresql-cluster-console/internal/configuration"
	"postgresql-cluster-console/internal/controllers"
	"postgresql-cluster-console/internal/storage"
	"postgresql-cluster-console/internal/watcher"
	"postgresql-cluster-console/internal/xdocker"
	"postgresql-cluster-console/models"
	"postgresql-cluster-console/pkg/tracer"
	clusterapi "postgresql-cluster-console/restapi/operations/cluster"

	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/strfmt"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"
)

const queryAnalyticsPlaybook = "query_analytics.yml"

type queryAnalyticsOperationsHandler struct {
	db             storage.IStorage
	dockerManager  xdocker.IManager
	logCollector   watcher.LogCollector
	clusterWatcher watcher.ClusterWatcher
	cfg            *configuration.Config
	log            zerolog.Logger
}

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

func NewQueryAnalyticsOperationsHandler(db storage.IStorage, dockerManager xdocker.IManager, logCollector watcher.LogCollector, clusterWatcher watcher.ClusterWatcher, cfg *configuration.Config, log zerolog.Logger) *queryAnalyticsOperationsHandler {
	return &queryAnalyticsOperationsHandler{db: db, dockerManager: dockerManager, logCollector: logCollector, clusterWatcher: clusterWatcher, cfg: cfg, log: log}
}

func (h *queryAnalyticsOperationsHandler) HandlePreflight(param clusterapi.PostClustersIDPreflightsParams) middleware.Responder {
	ctx := param.HTTPRequest.Context()
	clusterInfo, err := h.db.GetCluster(ctx, param.ID)
	if err != nil {
		return clusterapi.NewPostClustersIDPreflightsBadRequest().WithPayload(controllers.MakeErrorPayload(err, controllers.BaseError))
	}
	refreshStarted := time.Now().UTC().Add(-2 * time.Second)
	h.clusterWatcher.HandleCluster(ctx, clusterInfo)
	servers, err := h.db.GetClusterServers(ctx, clusterInfo.ID)
	if err != nil {
		return clusterapi.NewPostClustersIDPreflightsBadRequest().WithPayload(controllers.MakeErrorPayload(err, controllers.BaseError))
	}
	operationType := *param.Body.Type
	targetEnabled := operationType == storage.OperationTypeQueryAnalyticsEnable
	active, err := h.db.HasActiveOperation(ctx, clusterInfo.ID)
	if err != nil {
		return clusterapi.NewPostClustersIDPreflightsBadRequest().WithPayload(controllers.MakeErrorPayload(err, controllers.BaseError))
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
	state := "disabled"
	confirmation := "DISABLE QUERY ANALYTICS"
	if targetEnabled {
		state = "enabled"
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
		return clusterapi.NewPostClustersIDPreflightsBadRequest().WithPayload(controllers.MakeErrorPayload(err, controllers.BaseError))
	}
	preflight, err := h.db.CreateOperationPreflight(ctx, &storage.CreateOperationPreflightReq{
		ClusterID: clusterInfo.ID, Type: operationType, Observed: mustJSON(observed), Desired: mustJSON(desired),
		Checks: mustJSON(checks), Blockers: mustJSON(blockers), Plan: mustJSON(plan), AffectedNodes: mustJSON(affected),
		Confirmation: confirmation, TopologyHash: hash, ExpiresAt: time.Now().UTC().Add(10 * time.Minute),
	})
	if err != nil {
		return clusterapi.NewPostClustersIDPreflightsBadRequest().WithPayload(controllers.MakeErrorPayload(err, controllers.BaseError))
	}
	return clusterapi.NewPostClustersIDPreflightsCreated().WithPayload(preflightModel(preflight))
}

func (h *queryAnalyticsOperationsHandler) HandleOperation(param clusterapi.PostClustersIDOperationsParams) middleware.Responder {
	ctx := param.HTTPRequest.Context()
	preflight, err := h.db.GetOperationPreflight(ctx, *param.Body.PreflightID)
	if err != nil {
		return clusterapi.NewPostClustersIDOperationsBadRequest().WithPayload(controllers.MakeErrorPayload(err, controllers.BaseError))
	}
	if preflight.ClusterID != param.ID || preflight.ConsumedAt != nil || time.Now().After(preflight.ExpiresAt) {
		return clusterapi.NewPostClustersIDOperationsBadRequest().WithPayload(controllers.MakeErrorPayload(errors.New("preflight is stale or already used"), controllers.BaseError))
	}
	if *param.Body.Confirmation != preflight.Confirmation {
		return clusterapi.NewPostClustersIDOperationsBadRequest().WithPayload(controllers.MakeErrorPayload(errors.New("confirmation does not match preflight"), controllers.BaseError))
	}
	var blockers []string
	if err = json.Unmarshal(preflight.Blockers, &blockers); err != nil || len(blockers) != 0 {
		return clusterapi.NewPostClustersIDOperationsBadRequest().WithPayload(controllers.MakeErrorPayload(fmt.Errorf("preflight blocked: %s", strings.Join(blockers, ", ")), controllers.BaseError))
	}
	clusterInfo, err := h.db.GetCluster(ctx, param.ID)
	if err != nil {
		return clusterapi.NewPostClustersIDOperationsBadRequest().WithPayload(controllers.MakeErrorPayload(err, controllers.BaseError))
	}
	refreshStarted := time.Now().UTC().Add(-2 * time.Second)
	h.clusterWatcher.HandleCluster(ctx, clusterInfo)
	servers, err := h.db.GetClusterServers(ctx, clusterInfo.ID)
	if err != nil {
		return clusterapi.NewPostClustersIDOperationsBadRequest().WithPayload(controllers.MakeErrorPayload(err, controllers.BaseError))
	}
	if !topologyFresh(servers, refreshStarted) {
		return clusterapi.NewPostClustersIDOperationsBadRequest().WithPayload(controllers.MakeErrorPayload(errors.New("live topology refresh failed; run preflight again"), controllers.BaseError))
	}
	nodes, _, _ := operationTopology(servers)
	currentHash, err := topologyHash(nodes)
	if err != nil || currentHash != preflight.TopologyHash {
		return clusterapi.NewPostClustersIDOperationsBadRequest().WithPayload(controllers.MakeErrorPayload(errors.New("cluster topology changed; run preflight again"), controllers.BaseError))
	}

	cid := ctx.Value(tracer.CtxCidKey{}).(string)
	state := "disabled"
	if preflight.Type == storage.OperationTypeQueryAnalyticsEnable {
		state = "enabled"
	}
	operation, err := h.db.ReserveOperation(ctx, &storage.CreateOperationReq{
		ProjectID: clusterInfo.ProjectID, ClusterID: clusterInfo.ID, Type: preflight.Type, Cid: cid, Actor: "api-token",
		SanitizedParams:   mustJSON(map[string]any{"state": state, "extension_version": "2.3.2"}),
		PreflightSnapshot: mustJSON(map[string]any{"id": preflight.ID, "observed": json.RawMessage(preflight.Observed), "desired": json.RawMessage(preflight.Desired), "checks": json.RawMessage(preflight.Checks), "topology_hash": preflight.TopologyHash}),
		Plan:              preflight.Plan, AffectedNodes: preflight.AffectedNodes,
	})
	if err != nil {
		return clusterapi.NewPostClustersIDOperationsBadRequest().WithPayload(controllers.MakeErrorPayload(errors.New("another cluster mutation is active"), controllers.BaseError))
	}
	consumed, err := h.db.ConsumeOperationPreflight(ctx, preflight.ID)
	if err != nil || !consumed {
		h.failLaunch(ctx, operation.ID, "Run a fresh preflight and retry.")
		return clusterapi.NewPostClustersIDOperationsBadRequest().WithPayload(controllers.MakeErrorPayload(errors.New("preflight could not be consumed"), controllers.BaseError))
	}

	envs, extraVars, err := h.operationInputs(ctx, clusterInfo, state)
	if err != nil {
		h.failLaunch(ctx, operation.ID, "Correct cluster credentials and run a fresh preflight.")
		return clusterapi.NewPostClustersIDOperationsBadRequest().WithPayload(controllers.MakeErrorPayload(err, controllers.BaseError))
	}
	dockerID, err := h.dockerManager.ManageCluster(ctx, &xdocker.ManageClusterConfig{
		Envs: envs, ExtraVars: string(extraVars), Playbook: queryAnalyticsPlaybook,
		Mounts: []xdocker.Mount{{DockerPath: ansibleLogDir, HostPath: h.cfg.Docker.LogDir}},
	})
	if err != nil {
		h.failLaunch(ctx, operation.ID, "Inspect automation availability, then run a fresh preflight.")
		return clusterapi.NewPostClustersIDOperationsBadRequest().WithPayload(controllers.MakeErrorPayload(err, controllers.BaseError))
	}
	running := storage.OperationStatusRunning
	code := string(dockerID)
	if _, err = h.db.UpdateOperation(ctx, &storage.UpdateOperationReq{ID: operation.ID, Status: &running, DockerCode: &code}); err != nil {
		_ = h.dockerManager.RemoveContainer(ctx, dockerID)
		h.failLaunch(ctx, operation.ID, "Inspect Console database health, then run a fresh preflight.")
		return clusterapi.NewPostClustersIDOperationsBadRequest().WithPayload(controllers.MakeErrorPayload(err, controllers.BaseError))
	}
	h.logCollector.StoreInDb(operation.ID, dockerID, cid)
	return clusterapi.NewPostClustersIDOperationsAccepted().WithPayload(&models.ResponseOperationStart{OperationID: operation.ID, Status: running})
}

func (h *queryAnalyticsOperationsHandler) operationInputs(ctx context.Context, clusterInfo *storage.Cluster, state string) ([]string, []byte, error) {
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

func (h *queryAnalyticsOperationsHandler) failLaunch(ctx context.Context, operationID int64, next string) {
	status := storage.OperationStatusFailed
	_, _ = h.db.UpdateOperation(ctx, &storage.UpdateOperationReq{
		ID: operationID, Status: &status, FinalVerification: mustJSON(map[string]any{"automation_launched": false}), SafeNextAction: &next,
	})
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
