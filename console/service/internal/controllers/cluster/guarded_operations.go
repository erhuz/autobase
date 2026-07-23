package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
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
	"github.com/rs/zerolog"
)

type guardedOperationsHandler struct {
	db             storage.IStorage
	dockerManager  xdocker.IManager
	logCollector   watcher.LogCollector
	clusterWatcher watcher.ClusterWatcher
	cfg            *configuration.Config
	log            zerolog.Logger
}

type guardedPreflight struct {
	observed      any
	desired       any
	checks        []preflightCheck
	blockers      []string
	plan          []string
	affectedNodes []string
	confirmation  string
	topologyHash  string
}

func NewGuardedOperationsHandler(db storage.IStorage, dockerManager xdocker.IManager, logCollector watcher.LogCollector, clusterWatcher watcher.ClusterWatcher, cfg *configuration.Config, log zerolog.Logger) *guardedOperationsHandler {
	return &guardedOperationsHandler{db: db, dockerManager: dockerManager, logCollector: logCollector, clusterWatcher: clusterWatcher, cfg: cfg, log: log}
}

func (h *guardedOperationsHandler) HandlePreflight(param clusterapi.PostClustersIDPreflightsParams) middleware.Responder {
	if param.Body == nil || param.Body.Type == nil {
		return clusterapi.NewPostClustersIDPreflightsBadRequest().WithPayload(controllers.MakeErrorPayload(errors.New("operation type is required"), controllers.BaseError))
	}
	ctx := param.HTTPRequest.Context()
	if _, ok := queryAnalyticsState(*param.Body.Type); !ok {
		return clusterapi.NewPostClustersIDPreflightsBadRequest().WithPayload(controllers.MakeErrorPayload(errors.New("unsupported operation type"), controllers.BaseError))
	}
	clusterInfo, err := h.db.GetCluster(ctx, param.ID)
	if err != nil {
		return clusterapi.NewPostClustersIDPreflightsBadRequest().WithPayload(controllers.MakeErrorPayload(errors.New("cluster state is unavailable"), controllers.BaseError))
	}
	state, err := h.preflightState(ctx, clusterInfo, *param.Body.Type)
	if err != nil {
		return clusterapi.NewPostClustersIDPreflightsBadRequest().WithPayload(controllers.MakeErrorPayload(errors.New("preflight checks failed"), controllers.BaseError))
	}
	preflight, err := h.db.CreateOperationPreflight(ctx, &storage.CreateOperationPreflightReq{
		ClusterID: clusterInfo.ID, Type: *param.Body.Type,
		Observed: mustJSON(state.observed), Desired: mustJSON(state.desired),
		Checks: mustJSON(state.checks), Blockers: mustJSON(state.blockers),
		Plan: mustJSON(state.plan), AffectedNodes: mustJSON(state.affectedNodes),
		Confirmation: state.confirmation, TopologyHash: state.topologyHash, ExpiresAt: time.Now().UTC().Add(10 * time.Minute),
	})
	if err != nil {
		return clusterapi.NewPostClustersIDPreflightsBadRequest().WithPayload(controllers.MakeErrorPayload(errors.New("preflight could not be stored"), controllers.BaseError))
	}
	return clusterapi.NewPostClustersIDPreflightsCreated().WithPayload(preflightModel(preflight))
}

func (h *guardedOperationsHandler) HandleOperation(param clusterapi.PostClustersIDOperationsParams) middleware.Responder {
	if param.Body == nil || param.Body.PreflightID == nil || param.Body.Confirmation == nil {
		return clusterapi.NewPostClustersIDOperationsBadRequest().WithPayload(controllers.MakeErrorPayload(errors.New("preflight id and confirmation are required"), controllers.BaseError))
	}
	ctx := param.HTTPRequest.Context()
	preflight, err := h.db.GetOperationPreflight(ctx, *param.Body.PreflightID)
	if err != nil {
		return clusterapi.NewPostClustersIDOperationsBadRequest().WithPayload(controllers.MakeErrorPayload(errors.New("preflight is unavailable"), controllers.BaseError))
	}
	if preflight.ClusterID != param.ID || preflight.ConsumedAt != nil || time.Now().After(preflight.ExpiresAt) {
		return clusterapi.NewPostClustersIDOperationsBadRequest().WithPayload(controllers.MakeErrorPayload(errors.New("preflight is stale or already used"), controllers.BaseError))
	}
	if *param.Body.Confirmation != preflight.Confirmation {
		return clusterapi.NewPostClustersIDOperationsBadRequest().WithPayload(controllers.MakeErrorPayload(errors.New("confirmation does not match preflight"), controllers.BaseError))
	}
	var blockers []string
	if err = json.Unmarshal(preflight.Blockers, &blockers); err != nil {
		return clusterapi.NewPostClustersIDOperationsBadRequest().WithPayload(controllers.MakeErrorPayload(errors.New("preflight blockers are invalid"), controllers.BaseError))
	}
	if len(blockers) != 0 {
		return clusterapi.NewPostClustersIDOperationsBadRequest().WithPayload(controllers.MakeErrorPayload(fmt.Errorf("preflight blocked: %s", strings.Join(blockers, ", ")), controllers.BaseError))
	}
	state, ok := queryAnalyticsState(preflight.Type)
	if !ok {
		return clusterapi.NewPostClustersIDOperationsBadRequest().WithPayload(controllers.MakeErrorPayload(errors.New("unsupported operation type"), controllers.BaseError))
	}

	clusterInfo, err := h.db.GetCluster(ctx, param.ID)
	if err != nil {
		return clusterapi.NewPostClustersIDOperationsBadRequest().WithPayload(controllers.MakeErrorPayload(errors.New("cluster state is unavailable"), controllers.BaseError))
	}
	current, err := h.preflightState(ctx, clusterInfo, preflight.Type)
	if err != nil {
		return clusterapi.NewPostClustersIDOperationsBadRequest().WithPayload(controllers.MakeErrorPayload(errors.New("cluster state could not be rechecked"), controllers.BaseError))
	}
	if len(current.blockers) != 0 {
		return clusterapi.NewPostClustersIDOperationsBadRequest().WithPayload(controllers.MakeErrorPayload(errors.New("cluster guards changed; run preflight again"), controllers.BaseError))
	}
	if current.topologyHash != preflight.TopologyHash ||
		!sameJSON(preflight.Observed, mustJSON(current.observed)) ||
		!sameJSON(preflight.Desired, mustJSON(current.desired)) {
		return clusterapi.NewPostClustersIDOperationsBadRequest().WithPayload(controllers.MakeErrorPayload(errors.New("cluster state changed; run preflight again"), controllers.BaseError))
	}

	cid, _ := ctx.Value(tracer.CtxCidKey{}).(string)
	operation, err := h.db.ReserveOperation(ctx, &storage.CreateOperationReq{
		ProjectID: clusterInfo.ProjectID, ClusterID: clusterInfo.ID, Type: preflight.Type, Cid: cid, Actor: "api-token",
		SanitizedParams: mustJSON(map[string]any{"state": state, "extension_version": "2.3.2"}),
		PreflightSnapshot: mustJSON(map[string]any{
			"id": preflight.ID, "type": preflight.Type, "observed": json.RawMessage(preflight.Observed),
			"desired": json.RawMessage(preflight.Desired), "checks": json.RawMessage(preflight.Checks),
			"blockers": json.RawMessage(preflight.Blockers), "topology_hash": preflight.TopologyHash,
		}),
		Plan: preflight.Plan, AffectedNodes: preflight.AffectedNodes,
	})
	if err != nil {
		return clusterapi.NewPostClustersIDOperationsBadRequest().WithPayload(controllers.MakeErrorPayload(errors.New("another cluster mutation is active"), controllers.BaseError))
	}
	consumed, err := h.db.ConsumeOperationPreflight(ctx, preflight.ID)
	if err != nil || !consumed {
		h.failLaunch(ctx, operation.ID, "Run a fresh preflight and retry.")
		return clusterapi.NewPostClustersIDOperationsBadRequest().WithPayload(controllers.MakeErrorPayload(errors.New("preflight could not be consumed"), controllers.BaseError))
	}

	envs, extraVars, err := h.queryAnalyticsOperationInputs(ctx, clusterInfo, state)
	if err != nil {
		h.failLaunch(ctx, operation.ID, "Correct cluster credentials and run a fresh preflight.")
		return clusterapi.NewPostClustersIDOperationsBadRequest().WithPayload(controllers.MakeErrorPayload(errors.New("operation inputs are unavailable"), controllers.BaseError))
	}
	dockerID, err := h.dockerManager.ManageCluster(ctx, &xdocker.ManageClusterConfig{
		Envs: envs, ExtraVars: string(extraVars), Playbook: queryAnalyticsPlaybook,
		Mounts: []xdocker.Mount{{DockerPath: ansibleLogDir, HostPath: h.cfg.Docker.LogDir}},
	})
	if err != nil {
		h.failLaunch(ctx, operation.ID, "Inspect automation availability, then run a fresh preflight.")
		return clusterapi.NewPostClustersIDOperationsBadRequest().WithPayload(controllers.MakeErrorPayload(errors.New("automation launch failed"), controllers.BaseError))
	}
	running := storage.OperationStatusRunning
	code := string(dockerID)
	if _, err = h.db.UpdateOperation(ctx, &storage.UpdateOperationReq{ID: operation.ID, Status: &running, DockerCode: &code}); err != nil {
		_ = h.dockerManager.RemoveContainer(ctx, dockerID)
		h.failLaunch(ctx, operation.ID, "Inspect Console database health, then run a fresh preflight.")
		return clusterapi.NewPostClustersIDOperationsBadRequest().WithPayload(controllers.MakeErrorPayload(errors.New("operation state update failed"), controllers.BaseError))
	}
	h.logCollector.StoreInDb(operation.ID, dockerID, cid)
	return clusterapi.NewPostClustersIDOperationsAccepted().WithPayload(&models.ResponseOperationStart{OperationID: operation.ID, Status: running})
}

func (h *guardedOperationsHandler) preflightState(ctx context.Context, clusterInfo *storage.Cluster, operationType string) (*guardedPreflight, error) {
	switch operationType {
	case storage.OperationTypeQueryAnalyticsEnable, storage.OperationTypeQueryAnalyticsDisable:
		return h.queryAnalyticsPreflightState(ctx, clusterInfo, operationType)
	default:
		return nil, errors.New("unsupported operation type")
	}
}

func (h *guardedOperationsHandler) failLaunch(ctx context.Context, operationID int64, next string) {
	status := storage.OperationStatusFailed
	_, _ = h.db.UpdateOperation(ctx, &storage.UpdateOperationReq{
		ID: operationID, Status: &status,
		FinalVerification: mustJSON(map[string]any{"automation_launched": false}),
		SafeNextAction:    &next,
	})
}

func sameJSON(left, right []byte) bool {
	var a, b any
	return json.Unmarshal(left, &a) == nil && json.Unmarshal(right, &b) == nil && reflect.DeepEqual(a, b)
}
