package cluster

import (
	"context"
	"encoding/base64"
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
	if !supportedOperationType(*param.Body.Type) {
		return clusterapi.NewPostClustersIDPreflightsBadRequest().WithPayload(controllers.MakeErrorPayload(errors.New("unsupported operation type"), controllers.BaseError))
	}
	target := param.Body.Target
	clusterInfo, err := h.db.GetCluster(ctx, param.ID)
	if err != nil {
		return clusterapi.NewPostClustersIDPreflightsBadRequest().WithPayload(controllers.MakeErrorPayload(errors.New("cluster state is unavailable"), controllers.BaseError))
	}
	state, err := h.preflightState(ctx, clusterInfo, *param.Body.Type, target)
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
	target, err := operationTarget(preflight.Type, preflight.Desired)
	if err != nil {
		return clusterapi.NewPostClustersIDOperationsBadRequest().WithPayload(controllers.MakeErrorPayload(errors.New("preflight desired state is invalid"), controllers.BaseError))
	}

	clusterInfo, err := h.db.GetCluster(ctx, param.ID)
	if err != nil {
		return clusterapi.NewPostClustersIDOperationsBadRequest().WithPayload(controllers.MakeErrorPayload(errors.New("cluster state is unavailable"), controllers.BaseError))
	}
	current, err := h.preflightState(ctx, clusterInfo, preflight.Type, target)
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
		SanitizedParams: preflight.Desired,
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

	envs, extraVars, playbook, err := h.operationInputs(ctx, clusterInfo, preflight.Type, preflight.Desired)
	if err != nil {
		h.failLaunch(ctx, operation.ID, "Correct cluster credentials and run a fresh preflight.")
		return clusterapi.NewPostClustersIDOperationsBadRequest().WithPayload(controllers.MakeErrorPayload(errors.New("operation inputs are unavailable"), controllers.BaseError))
	}
	dockerID, err := h.dockerManager.ManageCluster(ctx, &xdocker.ManageClusterConfig{
		Envs: envs, ExtraVars: string(extraVars), Playbook: playbook,
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

func supportedOperationType(operationType string) bool {
	switch operationType {
	case storage.OperationTypeSwitchover, storage.OperationTypeReload, storage.OperationTypeRollingRestart,
		storage.OperationTypeQueryAnalyticsEnable, storage.OperationTypeQueryAnalyticsDisable:
		return true
	default:
		return false
	}
}

func (h *guardedOperationsHandler) preflightState(ctx context.Context, clusterInfo *storage.Cluster, operationType, target string) (*guardedPreflight, error) {
	switch operationType {
	case storage.OperationTypeSwitchover:
		return h.switchoverPreflightState(ctx, clusterInfo, target)
	case storage.OperationTypeReload, storage.OperationTypeRollingRestart:
		return h.maintenancePreflightState(ctx, clusterInfo, operationType)
	case storage.OperationTypeQueryAnalyticsEnable, storage.OperationTypeQueryAnalyticsDisable:
		return h.queryAnalyticsPreflightState(ctx, clusterInfo, operationType)
	default:
		return nil, errors.New("unsupported operation type")
	}
}

func (h *guardedOperationsHandler) operationInputs(ctx context.Context, clusterInfo *storage.Cluster, operationType string, desired []byte) ([]string, []byte, string, error) {
	switch operationType {
	case storage.OperationTypeSwitchover:
		envs, extraVars, err := h.switchoverOperationInputs(ctx, clusterInfo, desired)
		return envs, extraVars, switchoverPlaybook, err
	case storage.OperationTypeReload, storage.OperationTypeRollingRestart:
		envs, extraVars, playbook, err := h.maintenanceOperationInputs(ctx, clusterInfo, operationType, desired)
		return envs, extraVars, playbook, err
	case storage.OperationTypeQueryAnalyticsEnable, storage.OperationTypeQueryAnalyticsDisable:
		state, _ := queryAnalyticsState(operationType)
		envs, extraVars, err := h.queryAnalyticsOperationInputs(ctx, clusterInfo, state)
		return envs, extraVars, queryAnalyticsPlaybook, err
	default:
		return nil, nil, "", errors.New("unsupported operation type")
	}
}

func (h *guardedOperationsHandler) baseOperationInputs(ctx context.Context, clusterInfo *storage.Cluster) ([]string, map[string]any, error) {
	extraVars := map[string]any{}
	if len(clusterInfo.ExtraVars) != 0 {
		if err := json.Unmarshal(clusterInfo.ExtraVars, &extraVars); err != nil {
			return nil, nil, err
		}
	}
	envs := []string{"ANSIBLE_JSON_LOG_FILE=" + ansibleLogDir + "/" + clusterInfo.Name + ".json"}
	if len(clusterInfo.Inventory) != 0 {
		envs = append(envs, "ANSIBLE_INVENTORY_JSON="+base64.StdEncoding.EncodeToString(clusterInfo.Inventory))
	}
	if clusterInfo.SecretID != nil {
		secretValues, location, err := getSecretEnvs(ctx, h.log, h.db, *clusterInfo.SecretID, h.cfg.EncryptionKey)
		if err != nil {
			return nil, nil, err
		}
		if location == ExtraVarsParamLocation {
			for _, value := range secretValues {
				parts := strings.SplitN(value, "=", 2)
				if len(parts) == 2 {
					extraVars[parts[0]] = parts[1]
				}
			}
		} else {
			envs = append(envs, secretValues...)
		}
	}
	return envs, extraVars, nil
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
