package storage

const (
	DefaultLimit       = 20
	InstanceTypeSmall  = "Small Size"
	InstanceTypeMedium = "Medium Size"
	InstanceTypeLarge  = "Large Size"

	OperationStatusQueued    = "queued"
	OperationStatusRunning   = "running"
	OperationStatusSucceeded = "succeeded"
	OperationStatusFailed    = "failed"
	OperationStatusCancelled = "cancelled"

	OperationStatusInProgress = OperationStatusRunning
	OperationStatusSuccess    = OperationStatusSucceeded

	OperationTypeDeploy                = "deploy"
	OperationTypeSwitchover            = "switchover"
	OperationTypeReload                = "reload"
	OperationTypeRollingRestart        = "rolling_restart"
	OperationTypeQueryAnalyticsEnable  = "query_analytics_enable"
	OperationTypeQueryAnalyticsDisable = "query_analytics_disable"

	ClusterStatusFailed      = "failed"
	ClusterStatusHealthy     = "healthy"
	ClusterStatusUnhealthy   = "unhealthy"
	ClusterStatusDegraded    = "degraded"
	ClusterStatusReady       = "ready"
	ClusterStatusUnavailable = "unavailable"
)

func IsTerminalOperationStatus(status string) bool {
	return status == OperationStatusSucceeded || status == OperationStatusFailed || status == OperationStatusCancelled
}

var (
	secretSortFields = map[string]string{
		"name":       "secret_name",
		"id":         "secret_id",
		"type":       "secret_type",
		"created_at": "created_at",
		"updated_at": "updated_at",
	}

	clusterSortFields = map[string]string{
		"name":             "cluster_name",
		"id":               "cluster_id",
		"created_at":       "created_at",
		"updated_at":       "updated_at",
		"environment":      "environment_id",
		"status":           "cluster_status",
		"project":          "project_id",
		"location":         "cluster_location",
		"server_count":     "server_count",
		"postgres_version": "postgres_version",
	}

	operationSortFields = map[string]string{
		"cluster_name": "cluster",
		"type":         "type",
		"status":       "status",
		"id":           "id",
		"started":      "started",
		"finished":     "finished",
		"cluster":      "cluster",
		"environment":  "environment",
	}
)
