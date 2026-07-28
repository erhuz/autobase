package cluster

import (
	"errors"

	"postgresql-cluster-console/internal/configuration"
	"postgresql-cluster-console/internal/controllers"
	"postgresql-cluster-console/internal/storage"
	clusterapi "postgresql-cluster-console/restapi/operations/cluster"

	"github.com/go-openapi/runtime/middleware"
)

type putClusterAutomationCredentialsHandler struct {
	db  storage.IStorage
	cfg *configuration.Config
}

func NewPutClusterAutomationCredentialsHandler(
	db storage.IStorage,
	cfg *configuration.Config,
) clusterapi.PutClustersIDAutomationCredentialsHandler {
	return &putClusterAutomationCredentialsHandler{db: db, cfg: cfg}
}

func (h *putClusterAutomationCredentialsHandler) Handle(
	param clusterapi.PutClustersIDAutomationCredentialsParams,
) middleware.Responder {
	if param.Body == nil {
		return clusterapi.NewPutClustersIDAutomationCredentialsBadRequest().WithPayload(
			controllers.MakeErrorPayload(errors.New("automation credential bindings are required"), controllers.BaseError),
		)
	}
	ctx := param.HTTPRequest.Context()
	clusterInfo, err := h.db.GetCluster(ctx, param.ID)
	if err != nil {
		return clusterapi.NewPutClustersIDAutomationCredentialsBadRequest().WithPayload(
			controllers.MakeErrorPayload(errors.New("cluster is unavailable"), controllers.BaseError),
		)
	}
	credentials := storage.AutomationCredentials{
		PostgresSuperuserSecretID:   param.Body.PostgresSuperuserSecretID,
		PostgresReplicationSecretID: param.Body.PostgresReplicationSecretID,
		PatroniRestapiSecretID:      param.Body.PatroniRestapiSecretID,
	}
	seen := make(map[int64]bool, 3)
	for _, secretID := range []*int64{
		credentials.PostgresSuperuserSecretID,
		credentials.PostgresReplicationSecretID,
		credentials.PatroniRestapiSecretID,
	} {
		if secretID == nil {
			continue
		}
		if seen[*secretID] {
			return clusterapi.NewPutClustersIDAutomationCredentialsBadRequest().WithPayload(
				controllers.MakeErrorPayload(errors.New("each automation purpose requires a separate password secret"), controllers.BaseError),
			)
		}
		seen[*secretID] = true
		if _, _, err = loadAutomationCredential(
			ctx, h.db, automationEncryptionKey(h.cfg), clusterInfo.ProjectID, *secretID,
		); err != nil {
			return clusterapi.NewPutClustersIDAutomationCredentialsBadRequest().WithPayload(
				controllers.MakeErrorPayload(errors.New("credentials must be same-project password secrets with username and password"), controllers.BaseError),
			)
		}
	}
	clusterInfo, err = h.db.SetClusterAutomationCredentials(ctx, clusterInfo.ID, credentials)
	if err != nil {
		return clusterapi.NewPutClustersIDAutomationCredentialsBadRequest().WithPayload(
			controllers.MakeErrorPayload(errors.New("automation credentials could not be attached"), controllers.BaseError),
		)
	}
	response, err := getClusterInfo(ctx, h.db, clusterInfo)
	if err != nil {
		return clusterapi.NewPutClustersIDAutomationCredentialsBadRequest().WithPayload(
			controllers.MakeErrorPayload(errors.New("cluster is unavailable"), controllers.BaseError),
		)
	}
	return clusterapi.NewPutClustersIDAutomationCredentialsOK().WithPayload(response)
}
