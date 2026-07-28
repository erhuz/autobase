package cluster

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"postgresql-cluster-console/internal/configuration"
	"postgresql-cluster-console/internal/storage"
	"postgresql-cluster-console/models"
	clusterapi "postgresql-cluster-console/restapi/operations/cluster"
)

type automationCredentialStorage struct {
	storage.IStorage
	cluster *storage.Cluster
	secrets map[int64]*storage.SecretView
	values  map[int64][]byte
	updates int
}

func (s *automationCredentialStorage) GetCluster(context.Context, int64) (*storage.Cluster, error) {
	return s.cluster, nil
}

func (s *automationCredentialStorage) GetSecret(_ context.Context, id int64) (*storage.SecretView, error) {
	return s.secrets[id], nil
}

func (s *automationCredentialStorage) GetSecretVal(_ context.Context, id int64, _ string) ([]byte, error) {
	return s.values[id], nil
}

func (s *automationCredentialStorage) SetClusterAutomationCredentials(
	_ context.Context,
	_ int64,
	credentials storage.AutomationCredentials,
) (*storage.Cluster, error) {
	s.updates++
	copy := *s.cluster
	copy.PostgresSuperuserSecretID = credentials.PostgresSuperuserSecretID
	copy.PostgresReplicationSecretID = credentials.PostgresReplicationSecretID
	copy.PatroniRestapiSecretID = credentials.PatroniRestapiSecretID
	s.cluster = &copy
	return s.cluster, nil
}

func (*automationCredentialStorage) GetProject(context.Context, int64) (*storage.Project, error) {
	return &storage.Project{Name: "project"}, nil
}

func (*automationCredentialStorage) GetEnvironment(context.Context, int64) (*storage.Environment, error) {
	return &storage.Environment{Name: "production"}, nil
}

func (*automationCredentialStorage) GetClusterServers(context.Context, int64) ([]storage.Server, error) {
	return nil, nil
}

func TestAttachAutomationCredentialsAtomically(t *testing.T) {
	ids := []int64{8, 9, 10}
	store := &automationCredentialStorage{
		cluster: &storage.Cluster{ID: 5, ProjectID: 3, CreatedAt: time.Now()},
		secrets: map[int64]*storage.SecretView{},
		values:  map[int64][]byte{},
	}
	for _, id := range ids {
		store.secrets[id] = &storage.SecretView{
			ID: id, ProjectID: 3, Type: string(models.SecretTypePassword), CreatedAt: time.Now(),
		}
		store.values[id] = []byte(`{"USERNAME":"service","PASSWORD":"never-return-this"}`)
	}
	handler := NewPutClusterAutomationCredentialsHandler(store, &configuration.Config{})
	request := httptest.NewRequest("PUT", "/clusters/5/automation-credentials", nil)
	response := handler.Handle(clusterapi.PutClustersIDAutomationCredentialsParams{
		ID:          5,
		HTTPRequest: request,
		Body: &models.RequestClusterAutomationCredentials{
			PostgresSuperuserSecretID:   &ids[0],
			PostgresReplicationSecretID: &ids[1],
			PatroniRestapiSecretID:      &ids[2],
		},
	})
	ok, valid := response.(*clusterapi.PutClustersIDAutomationCredentialsOK)
	encoded, _ := json.Marshal(ok.Payload)
	if !valid || store.updates != 1 ||
		ok.Payload.AutomationCredentials == nil ||
		*ok.Payload.AutomationCredentials.PostgresSuperuserSecretID != ids[0] ||
		strings.Contains(string(encoded), "never-return-this") {
		t.Fatalf("response=%#v updates=%d payload=%s", response, store.updates, encoded)
	}

	response = handler.Handle(clusterapi.PutClustersIDAutomationCredentialsParams{
		ID:          5,
		HTTPRequest: request,
		Body: &models.RequestClusterAutomationCredentials{
			PostgresSuperuserSecretID:   &ids[0],
			PostgresReplicationSecretID: &ids[0],
		},
	})
	if _, valid = response.(*clusterapi.PutClustersIDAutomationCredentialsBadRequest); !valid || store.updates != 1 {
		t.Fatalf("shared-purpose credential accepted: response=%#v updates=%d", response, store.updates)
	}

	store.secrets[ids[0]].ProjectID = 4
	response = handler.Handle(clusterapi.PutClustersIDAutomationCredentialsParams{
		ID:          5,
		HTTPRequest: request,
		Body: &models.RequestClusterAutomationCredentials{
			PostgresSuperuserSecretID: &ids[0],
		},
	})
	if _, valid = response.(*clusterapi.PutClustersIDAutomationCredentialsBadRequest); !valid || store.updates != 1 {
		t.Fatalf("cross-project credential accepted: response=%#v updates=%d", response, store.updates)
	}

	response = handler.Handle(clusterapi.PutClustersIDAutomationCredentialsParams{
		ID: 5, HTTPRequest: request, Body: &models.RequestClusterAutomationCredentials{},
	})
	if _, valid = response.(*clusterapi.PutClustersIDAutomationCredentialsOK); !valid || store.updates != 2 ||
		store.cluster.PostgresSuperuserSecretID != nil ||
		store.cluster.PostgresReplicationSecretID != nil ||
		store.cluster.PatroniRestapiSecretID != nil {
		t.Fatalf("detach response=%#v cluster=%+v updates=%d", response, store.cluster, store.updates)
	}
}
