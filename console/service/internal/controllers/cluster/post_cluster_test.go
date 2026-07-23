package cluster

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"postgresql-cluster-console/internal/configuration"
	"postgresql-cluster-console/internal/storage"
	"postgresql-cluster-console/internal/xdocker"
	"postgresql-cluster-console/models"
	"postgresql-cluster-console/pkg/tracer"
	clusterapi "postgresql-cluster-console/restapi/operations/cluster"

	"github.com/rs/zerolog"
)

type passiveImportStorage struct {
	storage.IStorage
	clusterReq  *storage.CreateClusterReq
	servers     []*storage.CreateServerReq
	secretReads int
	credentials int
	operations  int
	deleted     int64
	serverError int
}

func (s *passiveImportStorage) GetClusterByName(context.Context, string) (*storage.Cluster, error) {
	return nil, nil
}

func (s *passiveImportStorage) GetSecret(context.Context, int64) (*storage.SecretView, error) {
	s.secretReads++
	return nil, errors.New("secret read during passive import")
}

func (s *passiveImportStorage) GetSecretVal(context.Context, int64, string) ([]byte, error) {
	s.secretReads++
	return nil, errors.New("secret value read during passive import")
}

func (s *passiveImportStorage) CreateCluster(_ context.Context, req *storage.CreateClusterReq) (*storage.Cluster, error) {
	s.clusterReq = req
	return &storage.Cluster{ID: 42}, nil
}

func (s *passiveImportStorage) CreateServer(_ context.Context, req *storage.CreateServerReq) (*storage.Server, error) {
	s.servers = append(s.servers, req)
	if s.serverError == len(s.servers) {
		return nil, errors.New("server insert failed")
	}
	return &storage.Server{ID: int64(len(s.servers))}, nil
}

func (s *passiveImportStorage) DeleteCluster(_ context.Context, id int64) error {
	s.deleted = id
	return nil
}

func (s *passiveImportStorage) SetQueryAnalyticsCredential(context.Context, int64, string, string) error {
	s.credentials++
	return nil
}

func (s *passiveImportStorage) CreateOperation(context.Context, *storage.CreateOperationReq) (*storage.Operation, error) {
	s.operations++
	return &storage.Operation{}, nil
}

type passiveImportDocker struct {
	xdocker.IManager
	calls int
}

func (d *passiveImportDocker) ManageCluster(context.Context, *xdocker.ManageClusterConfig) (xdocker.InstanceID, error) {
	d.calls++
	return "", errors.New("automation started during passive import")
}

type passiveImportLogs struct{ calls int }

func (l *passiveImportLogs) StoreInDb(int64, xdocker.InstanceID, string) { l.calls++ }
func (l *passiveImportLogs) PrintToConsole(xdocker.InstanceID, string)   { l.calls++ }
func (l *passiveImportLogs) Stop()                                       {}

type deploymentSecretStorage struct{ passiveImportStorage }

func (s *deploymentSecretStorage) GetSecret(context.Context, int64) (*storage.SecretView, error) {
	return &storage.SecretView{Type: string(models.SecretTypePassword)}, nil
}

func (s *deploymentSecretStorage) GetSecretVal(context.Context, int64, string) ([]byte, error) {
	return []byte(`{"USERNAME":"deploy-user","PASSWORD":"deploy-secret"}`), nil
}

func (s *deploymentSecretStorage) GetSettingByName(context.Context, string) (*storage.Setting, error) {
	return nil, nil
}

type deploymentCaptureDocker struct {
	xdocker.IManager
	config *xdocker.ManageClusterConfig
}

func (d *deploymentCaptureDocker) ManageCluster(_ context.Context, config *xdocker.ManageClusterConfig) (xdocker.InstanceID, error) {
	d.config = config
	return "test-container", nil
}

func TestExistingClusterImportIsPassive(t *testing.T) {
	inventory := `{"all":{"children":{"master":{"hosts":{"10.0.0.1":{"hostname":"pg-1"}}},"replica":{"hosts":{"10.0.0.2":{"hostname":"pg-2"},"10.0.0.3":{"hostname":"pg-3"}}}}}}`
	existing := true
	store := &passiveImportStorage{}
	docker := &passiveImportDocker{}
	logs := &passiveImportLogs{}
	request := httptest.NewRequest("POST", "/clusters", nil)
	request = request.WithContext(context.WithValue(request.Context(), tracer.CtxCidKey{}, "test-cid"))
	handler := NewPostClusterHandler(store, docker, logs, &configuration.Config{}, zerolog.Nop())

	response := handler.Handle(clusterapi.PostClustersParams{
		HTTPRequest: request,
		Body: &models.RequestClusterCreate{
			Name: "imported", ProjectID: 1, EnvironmentID: 2, ExistingCluster: &existing,
			AuthInfo: &models.RequestClusterCreateAuthInfo{SecretID: 7},
			Envs: []string{
				"ANSIBLE_INVENTORY_JSON=" + base64.StdEncoding.EncodeToString([]byte(inventory)),
			},
			ExtraVars: map[string]interface{}{"postgresql_version": 16},
		},
	})

	ok, isOK := response.(*clusterapi.PostClustersOK)
	if !isOK || ok.Payload.ClusterID != 42 || ok.Payload.OperationID != 0 {
		t.Fatalf("response = %#v", response)
	}
	if store.secretReads != 0 || store.credentials != 0 || store.operations != 0 || docker.calls != 0 || logs.calls != 0 {
		t.Fatalf("managed mutation: secret=%d credential=%d operation=%d docker=%d logs=%d",
			store.secretReads, store.credentials, store.operations, docker.calls, logs.calls)
	}
	if store.clusterReq == nil || store.clusterReq.SecretID == nil || *store.clusterReq.SecretID != 7 ||
		store.clusterReq.Status != storage.ClusterStatusReady || store.clusterReq.ServerCount != 3 ||
		store.clusterReq.QueryAnalyticsManaged == nil || store.clusterReq.QueryAnalyticsDesired == nil ||
		*store.clusterReq.QueryAnalyticsManaged || *store.clusterReq.QueryAnalyticsDesired {
		t.Fatalf("cluster request = %+v", store.clusterReq)
	}
	var storedVars map[string]interface{}
	if err := json.Unmarshal(store.clusterReq.ExtraVars, &storedVars); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"ansible_user", "ansible_ssh_pass", "ansible_sudo_pass", "proxy_env"} {
		if _, found := storedVars[forbidden]; found {
			t.Fatalf("stored secret/deployment variable %q", forbidden)
		}
	}
	if len(store.servers) != 3 {
		t.Fatalf("servers = %d", len(store.servers))
	}
}

func TestDeploymentSecretIsNotPersisted(t *testing.T) {
	inventory := `{"all":{"children":{"master":{"hosts":{"10.0.0.1":{}}},"replica":{"hosts":{"10.0.0.2":{},"10.0.0.3":{}}}}}}`
	store := &deploymentSecretStorage{}
	docker := &deploymentCaptureDocker{}
	request := httptest.NewRequest("POST", "/clusters", nil)
	request = request.WithContext(context.WithValue(request.Context(), tracer.CtxCidKey{}, "test-cid"))
	response := NewPostClusterHandler(store, docker, &passiveImportLogs{}, &configuration.Config{}, zerolog.Nop()).Handle(
		clusterapi.PostClustersParams{
			HTTPRequest: request,
			Body: &models.RequestClusterCreate{
				Name: "new", AuthInfo: &models.RequestClusterCreateAuthInfo{SecretID: 7},
				Envs:      []string{"ANSIBLE_INVENTORY_JSON=" + base64.StdEncoding.EncodeToString([]byte(inventory))},
				ExtraVars: map[string]interface{}{"postgresql_version": 13},
			},
		},
	)
	if _, ok := response.(*clusterapi.PostClustersOK); !ok || store.clusterReq == nil || docker.config == nil {
		t.Fatalf("response=%#v cluster=%+v docker=%+v", response, store.clusterReq, docker.config)
	}
	if string(store.clusterReq.ExtraVars) != `{"postgresql_version":13}` {
		t.Fatalf("persisted extra vars = %s", store.clusterReq.ExtraVars)
	}
	var deploymentVars map[string]interface{}
	if err := json.Unmarshal([]byte(docker.config.ExtraVars), &deploymentVars); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"ansible_user", "ansible_ssh_pass", "ansible_sudo_pass"} {
		if deploymentVars[key] == nil {
			t.Errorf("deployment secret %q missing", key)
		}
	}
}

func TestExistingClusterImportRequiresCompleteRegistration(t *testing.T) {
	existing := true
	request := httptest.NewRequest("POST", "/clusters", nil)
	request = request.WithContext(context.WithValue(request.Context(), tracer.CtxCidKey{}, "test-cid"))

	t.Run("inventory", func(t *testing.T) {
		store := &passiveImportStorage{}
		response := NewPostClusterHandler(store, &passiveImportDocker{}, &passiveImportLogs{}, &configuration.Config{}, zerolog.Nop()).Handle(
			clusterapi.PostClustersParams{
				HTTPRequest: request,
				Body: &models.RequestClusterCreate{
					Name: "imported", ExistingCluster: &existing,
					Envs: []string{"ANSIBLE_INVENTORY_JSON={secret-invalid-json"},
				},
			},
		)
		if _, ok := response.(*clusterapi.PostClustersBadRequest); !ok || store.clusterReq != nil {
			t.Fatalf("response=%#v cluster=%+v", response, store.clusterReq)
		}
	})

	t.Run("rollback", func(t *testing.T) {
		store := &passiveImportStorage{serverError: 2}
		inventory := `{"all":{"children":{"master":{"hosts":{"10.0.0.1":{}}},"replica":{"hosts":{"10.0.0.2":{}}}}}}`
		response := NewPostClusterHandler(store, &passiveImportDocker{}, &passiveImportLogs{}, &configuration.Config{}, zerolog.Nop()).Handle(
			clusterapi.PostClustersParams{
				HTTPRequest: request,
				Body: &models.RequestClusterCreate{
					Name: "imported", ExistingCluster: &existing,
					Envs: []string{"ANSIBLE_INVENTORY_JSON=" + base64.StdEncoding.EncodeToString([]byte(inventory))},
				},
			},
		)
		if _, ok := response.(*clusterapi.PostClustersBadRequest); !ok || store.deleted != 42 {
			t.Fatalf("response=%#v rollback=%d", response, store.deleted)
		}
	})
}
