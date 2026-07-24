package cluster

import (
	"errors"

	"postgresql-cluster-console/internal/controllers"
	"postgresql-cluster-console/internal/storage"
	"postgresql-cluster-console/models"
	clusterapi "postgresql-cluster-console/restapi/operations/cluster"

	"github.com/go-openapi/runtime/middleware"
)

type putClusterCredentialHandler struct {
	db storage.IStorage
}

func NewPutClusterCredentialHandler(db storage.IStorage) clusterapi.PutClustersIDCredentialHandler {
	return &putClusterCredentialHandler{db: db}
}

func (h *putClusterCredentialHandler) Handle(param clusterapi.PutClustersIDCredentialParams) middleware.Responder {
	if param.Body == nil || param.Body.SecretID == nil {
		return clusterapi.NewPutClustersIDCredentialBadRequest().WithPayload(controllers.MakeErrorPayload(errors.New("secret id is required"), controllers.BaseError))
	}
	ctx := param.HTTPRequest.Context()
	clusterInfo, err := h.db.GetCluster(ctx, param.ID)
	if err != nil {
		return clusterapi.NewPutClustersIDCredentialBadRequest().WithPayload(controllers.MakeErrorPayload(errors.New("cluster is unavailable"), controllers.BaseError))
	}
	secret, err := h.db.GetSecret(ctx, *param.Body.SecretID)
	if err != nil || secret == nil || secret.ProjectID != clusterInfo.ProjectID ||
		secret.Type != string(models.SecretTypeSSHKey) && secret.Type != string(models.SecretTypePassword) {
		return clusterapi.NewPutClustersIDCredentialBadRequest().WithPayload(controllers.MakeErrorPayload(errors.New("credential must be a same-project SSH key or password"), controllers.BaseError))
	}
	clusterInfo, err = h.db.UpdateCluster(ctx, &storage.UpdateClusterReq{ID: clusterInfo.ID, SecretID: &secret.ID})
	if err != nil {
		return clusterapi.NewPutClustersIDCredentialBadRequest().WithPayload(controllers.MakeErrorPayload(errors.New("credential could not be attached"), controllers.BaseError))
	}
	response, err := getClusterInfo(ctx, h.db, clusterInfo)
	if err != nil {
		return clusterapi.NewPutClustersIDCredentialBadRequest().WithPayload(controllers.MakeErrorPayload(errors.New("cluster is unavailable"), controllers.BaseError))
	}
	return clusterapi.NewPutClustersIDCredentialOK().WithPayload(response)
}
