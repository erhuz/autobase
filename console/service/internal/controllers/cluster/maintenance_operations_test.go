package cluster

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"postgresql-cluster-console/internal/configuration"
	"postgresql-cluster-console/internal/storage"
	"postgresql-cluster-console/models"
	"postgresql-cluster-console/pkg/tracer"
	clusterapi "postgresql-cluster-console/restapi/operations/cluster"

	"github.com/rs/zerolog"
)

func maintenancePreflight(t *testing.T, handler *guardedOperationsHandler, store *guardedOperationStorage, operationType string) {
	t.Helper()
	response := handler.HandlePreflight(clusterapi.PostClustersIDPreflightsParams{
		ID: 5, HTTPRequest: httptest.NewRequest("POST", "/clusters/5/preflights", nil),
		Body: &models.RequestOperationPreflight{Type: &operationType},
	})
	if _, ok := response.(*clusterapi.PostClustersIDPreflightsCreated); !ok || store.preflight == nil {
		t.Fatalf("preflight response=%#v preflight=%+v", response, store.preflight)
	}
}

func TestMaintenancePreflightGuards(t *testing.T) {
	for _, operationType := range []string{storage.OperationTypeReload, storage.OperationTypeRollingRestart} {
		t.Run(operationType, func(t *testing.T) {
			store, _ := switchoverFixture()
			handler := NewGuardedOperationsHandler(store, nil, nil, blockedPreflightWatcher{}, &configuration.Config{}, zerolog.Nop())
			maintenancePreflight(t, handler, store, operationType)

			var desired maintenanceDesired
			if err := json.Unmarshal(store.preflight.Desired, &desired); err != nil {
				t.Fatal(err)
			}
			if string(store.preflight.Blockers) != "[]" || desired.Action != operationType ||
				desired.ConfigurationChange || len(desired.Routing) != 1 {
				t.Fatalf("blockers=%s desired=%+v", store.preflight.Blockers, desired)
			}
		})
	}

	store, _ := switchoverFixture()
	store.servers[2].Status = "stopped"
	handler := NewGuardedOperationsHandler(store, nil, nil, blockedPreflightWatcher{}, &configuration.Config{}, zerolog.Nop())
	maintenancePreflight(t, handler, store, storage.OperationTypeRollingRestart)
	if !strings.Contains(string(store.preflight.Blockers), "at least two healthy replicas") {
		t.Fatalf("blockers=%s", store.preflight.Blockers)
	}
}

func TestMaintenanceRejectsRoleChangeAfterConfirmation(t *testing.T) {
	store, _ := switchoverFixture()
	handler := NewGuardedOperationsHandler(store, nil, nil, blockedPreflightWatcher{}, &configuration.Config{}, zerolog.Nop())
	maintenancePreflight(t, handler, store, storage.OperationTypeRollingRestart)

	store.servers[0].Role = "leader"
	store.servers[1].Role = "replica"
	request := httptest.NewRequest("POST", "/clusters/5/operations", nil)
	request = request.WithContext(context.WithValue(request.Context(), tracer.CtxCidKey{}, "test-cid"))
	response := handler.HandleOperation(clusterapi.PostClustersIDOperationsParams{
		ID: 5, HTTPRequest: request,
		Body: &models.RequestOperationStart{
			PreflightID: &store.preflight.ID, Confirmation: &store.preflight.Confirmation,
		},
	})
	if _, ok := response.(*clusterapi.PostClustersIDOperationsBadRequest); !ok || store.reserved != nil {
		t.Fatalf("response=%#v reserved=%+v", response, store.reserved)
	}
}

func TestMaintenanceLaunchesFixedAutomation(t *testing.T) {
	for operationType, playbook := range map[string]string{
		storage.OperationTypeReload:         reloadPlaybook,
		storage.OperationTypeRollingRestart: rollingRestartPlaybook,
	} {
		t.Run(operationType, func(t *testing.T) {
			store, _ := switchoverFixture()
			docker := &operationDocker{}
			logs := &operationLogs{}
			handler := NewGuardedOperationsHandler(store, docker, logs, blockedPreflightWatcher{}, &configuration.Config{}, zerolog.Nop())
			maintenancePreflight(t, handler, store, operationType)

			request := httptest.NewRequest("POST", "/clusters/5/operations", nil)
			request = request.WithContext(context.WithValue(request.Context(), tracer.CtxCidKey{}, "test-cid"))
			response := handler.HandleOperation(clusterapi.PostClustersIDOperationsParams{
				ID: 5, HTTPRequest: request,
				Body: &models.RequestOperationStart{
					PreflightID: &store.preflight.ID, Confirmation: &store.preflight.Confirmation,
				},
			})
			if _, ok := response.(*clusterapi.PostClustersIDOperationsAccepted); !ok {
				t.Fatalf("operation response=%#v", response)
			}
			var extraVars map[string]any
			if err := json.Unmarshal([]byte(docker.config.ExtraVars), &extraVars); err != nil {
				t.Fatal(err)
			}
			if store.reserved == nil || store.reserved.Type != operationType ||
				docker.config.Playbook != playbook || len(extraVars["operation_primary_routing_targets"].([]any)) != 1 ||
				docker.calls != 1 || logs.calls != 1 {
				t.Fatalf("reserved=%+v config=%+v vars=%+v docker=%d logs=%d", store.reserved, docker.config, extraVars, docker.calls, logs.calls)
			}
		})
	}
}
