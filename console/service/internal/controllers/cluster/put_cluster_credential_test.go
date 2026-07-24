package cluster

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"postgresql-cluster-console/internal/configuration"
	"postgresql-cluster-console/internal/storage"
	"postgresql-cluster-console/models"
	clusterapi "postgresql-cluster-console/restapi/operations/cluster"

	"github.com/rs/zerolog"
)

type credentialStorage struct {
	storage.IStorage
	cluster *storage.Cluster
	secret  *storage.SecretView
	updated *storage.UpdateClusterReq
}

func (s *credentialStorage) GetCluster(context.Context, int64) (*storage.Cluster, error) {
	return s.cluster, nil
}

func (s *credentialStorage) GetSecret(context.Context, int64) (*storage.SecretView, error) {
	return s.secret, nil
}

func (s *credentialStorage) UpdateCluster(_ context.Context, req *storage.UpdateClusterReq) (*storage.Cluster, error) {
	s.updated = req
	copy := *s.cluster
	copy.SecretID = req.SecretID
	return &copy, nil
}

func (*credentialStorage) GetProject(context.Context, int64) (*storage.Project, error) {
	return &storage.Project{Name: "project"}, nil
}

func (*credentialStorage) GetEnvironment(context.Context, int64) (*storage.Environment, error) {
	return &storage.Environment{Name: "production"}, nil
}

func (*credentialStorage) GetClusterServers(context.Context, int64) ([]storage.Server, error) {
	return nil, nil
}

func TestAttachManagementCredentialValidatesProjectAndType(t *testing.T) {
	secretID := int64(7)
	store := &credentialStorage{
		cluster: &storage.Cluster{ID: 5, ProjectID: 3, CreatedAt: time.Now()},
		secret:  &storage.SecretView{ID: secretID, ProjectID: 3, Type: string(models.SecretTypeSSHKey)},
	}
	request := httptest.NewRequest("PUT", "/clusters/5/credential", nil)
	response := NewPutClusterCredentialHandler(store).Handle(clusterapi.PutClustersIDCredentialParams{
		ID: 5, HTTPRequest: request, Body: &models.RequestClusterCredential{SecretID: &secretID},
	})
	ok, valid := response.(*clusterapi.PutClustersIDCredentialOK)
	if !valid || store.updated == nil || ok.Payload.SecretID == nil || *ok.Payload.SecretID != secretID {
		t.Fatalf("response=%#v update=%+v", response, store.updated)
	}

	for _, invalid := range []*storage.SecretView{
		{ID: secretID, ProjectID: 4, Type: string(models.SecretTypeSSHKey)},
		{ID: secretID, ProjectID: 3, Type: string(models.SecretTypeHetzner)},
	} {
		store.secret, store.updated = invalid, nil
		response = NewPutClusterCredentialHandler(store).Handle(clusterapi.PutClustersIDCredentialParams{
			ID: 5, HTTPRequest: request, Body: &models.RequestClusterCredential{SecretID: &secretID},
		})
		if _, valid = response.(*clusterapi.PutClustersIDCredentialBadRequest); !valid || store.updated != nil {
			t.Fatalf("invalid secret accepted: %+v response=%#v", invalid, response)
		}
	}
}

func TestManagementCredentialBlocksPreflightAndExecution(t *testing.T) {
	store, target := switchoverFixture()
	handler := NewGuardedOperationsHandler(store, nil, nil, blockedPreflightWatcher{}, &configuration.Config{}, zerolog.Nop())
	store.cluster.SecretID = nil
	switchoverPreflight(t, handler, store, target)
	if string(store.preflight.Blockers) != `["management credential attached"]` {
		t.Fatalf("missing credential blockers=%s", store.preflight.Blockers)
	}

	credentialID := int64(7)
	store.cluster.SecretID = &credentialID
	switchoverPreflight(t, handler, store, target)
	store.cluster.SecretID = nil
	response := handler.HandleOperation(clusterapi.PostClustersIDOperationsParams{
		ID: 5, HTTPRequest: httptest.NewRequest("POST", "/clusters/5/operations", nil),
		Body: &models.RequestOperationStart{
			PreflightID: &store.preflight.ID, Confirmation: &store.preflight.Confirmation,
		},
	})
	if _, ok := response.(*clusterapi.PostClustersIDOperationsBadRequest); !ok || store.reserved != nil {
		t.Fatalf("detached credential response=%#v reserved=%+v", response, store.reserved)
	}
}
