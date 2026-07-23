package cluster

import (
	"encoding/json"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"postgresql-cluster-console/internal/controllers"
	"postgresql-cluster-console/internal/storage"
	"postgresql-cluster-console/models"
	clusterapi "postgresql-cluster-console/restapi/operations/cluster"

	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/strfmt"
)

type getHealthHandler struct{ db storage.IStorage }

func NewGetHealthHandler(db storage.IStorage) clusterapi.GetClustersIDHealthHandler {
	return &getHealthHandler{db: db}
}

func (h *getHealthHandler) Handle(param clusterapi.GetClustersIDHealthParams) middleware.Responder {
	ctx := param.HTTPRequest.Context()
	cluster, err := h.db.GetCluster(ctx, param.ID)
	if err != nil {
		return clusterapi.NewGetClustersIDHealthBadRequest().WithPayload(controllers.MakeErrorPayload(err, controllers.BaseError))
	}
	servers, err := h.db.GetClusterServers(ctx, param.ID)
	if err != nil {
		return clusterapi.NewGetClustersIDHealthBadRequest().WithPayload(controllers.MakeErrorPayload(err, controllers.BaseError))
	}
	operations, err := h.db.GetClusterHealthOperations(ctx, param.ID)
	if err != nil {
		return clusterapi.NewGetClustersIDHealthBadRequest().WithPayload(controllers.MakeErrorPayload(err, controllers.BaseError))
	}
	backup, err := h.db.GetBackupEvidence(ctx, param.ID)
	if err != nil {
		return clusterapi.NewGetClustersIDHealthBadRequest().WithPayload(controllers.MakeErrorPayload(err, controllers.BaseError))
	}
	return clusterapi.NewGetClustersIDHealthOK().WithPayload(clusterHealthModel(cluster, servers, operations, backup, time.Now().UTC()))
}

func clusterHealthModel(cluster *storage.Cluster, servers []storage.Server, operations []storage.ClusterHealthOperation, backupEvidence *storage.BackupEvidence, now time.Time) *models.ResponseClusterHealth {
	observedAt := strfmt.DateTime(now)
	dcs := healthDCS(cluster.ExtraVars, cluster.Inventory)
	routing := healthRouting(cluster.ConnectionInfo)
	backup, recoverability := healthBackup(backupEvidence, now)

	// ponytail: live DCS/routing evidence belongs to T5; unknown beats inferred health.
	return &models.ResponseClusterHealth{
		ObservedAt:     &observedAt,
		Topology:       healthTopology(cluster, servers),
		Dcs:            dcs,
		Routing:        routing,
		Backup:         backup,
		Operation:      healthOperationSummary(operations),
		Recoverability: recoverability,
	}
}

func healthBackup(evidence *storage.BackupEvidence, now time.Time) (*models.HealthBackup, *models.HealthRecoverability) {
	if evidence == nil {
		return &models.HealthBackup{State: "not_observed", Locks: []string{}, Retention: map[string]any{}},
			&models.HealthRecoverability{
				State: models.HealthRecoverabilityStateDegraded,
				Reasons: []string{
					"backup_not_observed",
					"wal_continuity_not_observed",
					"restore_evidence_missing",
				},
			}
	}

	var retention map[string]any
	var locks, owners []string
	_ = json.Unmarshal(evidence.Retention, &retention)
	_ = json.Unmarshal(evidence.Locks, &locks)
	_ = json.Unmarshal(evidence.SchedulerOwners, &owners)
	if retention == nil {
		retention = map[string]any{}
	}
	if locks == nil {
		locks = []string{}
	}
	sort.Strings(locks)
	sort.Strings(owners)

	latest := evidence.LatestFull
	if evidence.LatestDifferential != nil && (latest == nil || evidence.LatestDifferential.After(*latest)) {
		latest = evidence.LatestDifferential
	}
	fresh := false
	if latest != nil && evidence.FreshnessSeconds > 0 {
		age := now.Sub(*latest)
		fresh = age >= 0 && age <= time.Duration(evidence.FreshnessSeconds)*time.Second
	}
	reasons := make([]string, 0, 8)
	if !evidence.RepositoryReachable {
		reasons = append(reasons, "backup_repository_unreachable")
	}
	if evidence.LatestFull == nil {
		reasons = append(reasons, "full_backup_missing")
	} else if !fresh {
		reasons = append(reasons, "backup_stale")
	}
	if len(retention) == 0 {
		reasons = append(reasons, "retention_not_observed")
	}
	if evidence.WalContinuous == nil {
		reasons = append(reasons, "wal_continuity_not_observed")
	} else if !*evidence.WalContinuous {
		reasons = append(reasons, "wal_gap")
	}
	if len(locks) != 0 {
		reasons = append(reasons, "backup_lock_active")
	}
	if len(owners) == 0 {
		reasons = append(reasons, "scheduler_owner_missing")
	} else if len(owners) > 1 {
		reasons = append(reasons, "duplicate_scheduler_owners")
	}
	if evidence.RestoreTestedAt == nil {
		reasons = append(reasons, "restore_evidence_missing")
	}

	state := "healthy"
	recoverabilityState := models.HealthRecoverabilityStateHealthy
	if len(reasons) != 0 {
		state = "degraded"
		recoverabilityState = models.HealthRecoverabilityStateDegraded
	}
	var owner *string
	if len(owners) == 1 {
		owner = &owners[0]
	}
	return &models.HealthBackup{
		State: state, RepositoryReachable: &evidence.RepositoryReachable,
		LatestFull: healthDateTime(evidence.LatestFull), LatestDifferential: healthDateTime(evidence.LatestDifferential),
		Retention: retention, WalContinuous: evidence.WalContinuous, Locks: locks,
		SchedulerOwner: owner, Fresh: &fresh,
		FreshnessPolicy: stringPointer((time.Duration(evidence.FreshnessSeconds) * time.Second).String()),
		RestoreTestedAt: healthDateTime(evidence.RestoreTestedAt),
	}, &models.HealthRecoverability{State: recoverabilityState, Reasons: reasons}
}

func healthDateTime(value *time.Time) *strfmt.DateTime {
	if value == nil {
		return nil
	}
	result := strfmt.DateTime(*value)
	return &result
}

func stringPointer(value string) *string { return &value }

func healthTopology(cluster *storage.Cluster, servers []storage.Server) *models.HealthTopology {
	sorted := append([]storage.Server(nil), servers...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	topology := &models.HealthTopology{
		State:            cluster.Status,
		ObservedAt:       healthTopologyObservedAt(cluster, sorted),
		PatroniReachable: healthPatroniReachable(cluster),
		Members:          make([]*models.HealthMember, 0, len(sorted)),
		Replicas:         make([]*models.HealthMember, 0, len(sorted)),
	}
	for _, server := range sorted {
		member := &models.HealthMember{
			Name: server.Name, Role: server.Role, State: server.Status, Timeline: server.Timeline,
			Lag: server.Lag, PendingRestart: server.PendingRestart,
		}
		topology.Members = append(topology.Members, member)
		if server.Role == "leader" {
			topology.Leader = member
		} else if server.Role != "" && server.Role != "N/A" {
			topology.Replicas = append(topology.Replicas, member)
		}
	}
	return topology
}

func healthTopologyObservedAt(cluster *storage.Cluster, servers []storage.Server) *strfmt.DateTime {
	latest := cluster.UpdatedAt
	for i := range servers {
		if servers[i].UpdatedAt != nil && (latest == nil || servers[i].UpdatedAt.After(*latest)) {
			latest = servers[i].UpdatedAt
		}
	}
	if latest == nil {
		return nil
	}
	observedAt := strfmt.DateTime(*latest)
	return &observedAt
}

func healthPatroniReachable(cluster *storage.Cluster) *bool {
	if storage.GetPatroniConnectStatus(cluster.Flags) == 0 && cluster.Status != storage.ClusterStatusUnavailable {
		return nil
	}
	reachable := cluster.Status != storage.ClusterStatusUnavailable
	return &reachable
}

func healthDCS(extraVars, inventory []byte) *models.HealthDCS {
	var config map[string]any
	_ = json.Unmarshal(extraVars, &config)
	dcsType, _ := config["dcs_type"].(string)

	var parsed struct {
		All struct {
			Children map[string]struct {
				Hosts map[string]any `json:"hosts"`
			} `json:"children"`
		} `json:"all"`
	}
	_ = json.Unmarshal(inventory, &parsed)
	groups := map[string]string{"etcd": "etcd_cluster", "consul": "consul_instances"}
	if dcsType == "" {
		for _, candidate := range []string{"etcd", "consul"} {
			if len(parsed.All.Children[groups[candidate]].Hosts) > 0 {
				dcsType = candidate
				break
			}
		}
	}
	members := make([]string, 0)
	for name := range parsed.All.Children[groups[dcsType]].Hosts {
		members = append(members, name)
	}
	sort.Strings(members)
	state := "not_observed"
	if len(members) > 0 {
		state = "configured_not_observed"
	}
	return &models.HealthDCS{State: state, Type: dcsType, Members: members}
}

func healthRouting(raw any) *models.HealthRouting {
	info := healthObject(raw)
	addresses, ports := info["address"], info["port"]
	targets := make([]*models.HealthRoutingTarget, 0)
	if addressMap, ok := addresses.(map[string]any); ok {
		for _, role := range healthRoutingRoles(addressMap) {
			for _, address := range healthStrings(addressMap[role]) {
				targets = append(targets, &models.HealthRoutingTarget{
					Role: role, Address: address, Port: healthPort(roleValue(ports, role)),
				})
			}
		}
	} else if portMap, ok := ports.(map[string]any); ok {
		for _, role := range healthRoutingRoles(portMap) {
			for _, address := range healthStrings(addresses) {
				targets = append(targets, &models.HealthRoutingTarget{
					Role: role, Address: address, Port: healthPort(portMap[role]),
				})
			}
		}
	} else {
		for _, address := range healthStrings(addresses) {
			targets = append(targets, &models.HealthRoutingTarget{
				Role: "unknown", Address: address, Port: healthPort(ports),
			})
		}
	}
	state := "not_observed"
	if len(targets) > 0 {
		state = "configured_not_observed"
	}
	return &models.HealthRouting{State: state, Targets: targets}
}

func healthObject(raw any) map[string]any {
	var data []byte
	switch value := raw.(type) {
	case string:
		data = []byte(value)
	case []byte:
		data = value
	default:
		data, _ = json.Marshal(value)
	}
	var result map[string]any
	_ = json.Unmarshal(data, &result)
	return result
}

func healthRoutingRoles(values map[string]any) []string {
	roles := make([]string, 0, 4)
	for _, role := range []string{"primary", "replica", "replica_sync", "replica_async"} {
		if _, ok := values[role]; ok {
			roles = append(roles, role)
		}
	}
	return roles
}

func roleValue(value any, role string) any {
	if values, ok := value.(map[string]any); ok {
		return values[role]
	}
	return value
}

func healthStrings(value any) []string {
	var values []string
	switch typed := value.(type) {
	case string:
		for _, item := range strings.Split(typed, ",") {
			if item = strings.TrimSpace(item); item != "" && item != "N/A" {
				candidate := item
				if !strings.Contains(candidate, "://") {
					candidate = "//" + candidate
				}
				parsed, err := url.Parse(candidate)
				if err == nil && parsed.Hostname() != "" {
					values = append(values, parsed.Hostname())
				}
			}
		}
	case []any:
		for _, item := range typed {
			values = append(values, healthStrings(item)...)
		}
	}
	return values
}

func healthPort(value any) *int64 {
	var port int64
	switch typed := value.(type) {
	case float64:
		port = int64(typed)
	case string:
		port, _ = strconv.ParseInt(typed, 10, 64)
	}
	if port < 1 || port > 65535 {
		return nil
	}
	return &port
}

func healthOperationSummary(operations []storage.ClusterHealthOperation) *models.HealthOperationSummary {
	sorted := append([]storage.ClusterHealthOperation(nil), operations...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].CreatedAt.Equal(sorted[j].CreatedAt) {
			return sorted[i].ID > sorted[j].ID
		}
		return sorted[i].CreatedAt.After(sorted[j].CreatedAt)
	})
	result := &models.HealthOperationSummary{}
	for i := range sorted {
		operation := healthOperation(&sorted[i])
		if result.Active == nil && (sorted[i].Status == storage.OperationStatusQueued || sorted[i].Status == storage.OperationStatusRunning) {
			result.Active = operation
		}
		if result.Latest == nil && storage.IsTerminalOperationStatus(sorted[i].Status) {
			result.Latest = operation
		}
		if result.Unresolved == nil && sorted[i].Status == storage.OperationStatusFailed {
			result.Unresolved = operation
		}
	}
	return result
}

func healthOperation(operation *storage.ClusterHealthOperation) *models.HealthOperation {
	result := &models.HealthOperation{
		ID: operation.ID, Type: operation.Type, Status: operation.Status,
		Started: strfmt.DateTime(operation.CreatedAt), SafeNextAction: operation.SafeNextAction,
	}
	if operation.UpdatedAt != nil && storage.IsTerminalOperationStatus(operation.Status) {
		finished := strfmt.DateTime(*operation.UpdatedAt)
		result.Finished = &finished
	}
	return result
}
