package convert

import (
	"encoding/json"
	"postgresql-cluster-console/internal/storage"
	"postgresql-cluster-console/models"
	"strings"

	"github.com/go-openapi/strfmt"
)

func ClusterToSwagger(cl *storage.Cluster, servers []storage.Server, environmentCode, projectCode string) *models.ClusterInfo {
	clusterInfo := &models.ClusterInfo{
		ConnectionInfo: visibleConnectionInfo(cl.ConnectionInfo),
		CreationTime:   strfmt.DateTime(cl.CreatedAt),
		ClusterLocation: func() string {
			if cl.Location != nil {
				return *cl.Location
			}

			return ""
		}(),
		Environment:     environmentCode,
		ID:              cl.ID,
		Servers:         make([]*models.ClusterInfoInstance, 0, len(servers)),
		Name:            cl.Name,
		Description:     cl.Description,
		PostgresVersion: cl.PostgreVersion,
		ProjectName:     projectCode,
		SecretID:        cl.SecretID,
		AutomationCredentials: &models.ClusterInfoAutomationCredentials{
			PostgresSuperuserSecretID:   cl.PostgresSuperuserSecretID,
			PostgresReplicationSecretID: cl.PostgresReplicationSecretID,
			PatroniRestapiSecretID:      cl.PatroniRestapiSecretID,
		},
		Status: cl.Status,
	}

	// Add extra_vars (as JSON string)
	if cl.ExtraVars != nil && len(cl.ExtraVars) > 0 {
		clusterInfo.ExtraVars = string(cl.ExtraVars)
	}

	// Add inventory (as string)
	if cl.Inventory != nil && len(cl.Inventory) > 0 {
		clusterInfo.Inventory = string(cl.Inventory)
	}

	for _, server := range servers {
		clusterInfo.Servers = append(clusterInfo.Servers, &models.ClusterInfoInstance{
			ID:             server.ID,
			IP:             server.IpAddress.String(),
			Lag:            server.Lag,
			Name:           server.Name,
			PendingRestart: server.PendingRestart,
			Role:           server.Role,
			Status:         server.Status,
			Tags:           storage.VisibleServerTags(server.Tags),
			Timeline:       server.Timeline,
		})
	}

	return clusterInfo
}

func visibleConnectionInfo(raw any) any {
	data, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var value any
	if err = json.Unmarshal(data, &value); err != nil {
		return nil
	}
	redactConnectionPasswords(value)
	return value
}

func redactConnectionPasswords(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if strings.EqualFold(key, "password") {
				delete(typed, key)
			} else {
				redactConnectionPasswords(child)
			}
		}
	case []any:
		for _, child := range typed {
			redactConnectionPasswords(child)
		}
	}
}
