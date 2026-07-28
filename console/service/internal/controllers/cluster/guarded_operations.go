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
	var params []byte
	if param.Body.Params != nil {
		params = mustJSON(param.Body.Params)
	}
	clusterInfo, err := h.db.GetCluster(ctx, param.ID)
	if err != nil {
		return clusterapi.NewPostClustersIDPreflightsBadRequest().WithPayload(controllers.MakeErrorPayload(errors.New("cluster state is unavailable"), controllers.BaseError))
	}
	state, err := h.preflightState(ctx, clusterInfo, *param.Body.Type, target, params)
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
	params, err := operationParams(preflight.Type, preflight.Desired)
	if err != nil {
		return clusterapi.NewPostClustersIDOperationsBadRequest().WithPayload(controllers.MakeErrorPayload(errors.New("preflight desired state is invalid"), controllers.BaseError))
	}

	clusterInfo, err := h.db.GetCluster(ctx, param.ID)
	if err != nil {
		return clusterapi.NewPostClustersIDOperationsBadRequest().WithPayload(controllers.MakeErrorPayload(errors.New("cluster state is unavailable"), controllers.BaseError))
	}
	current, err := h.preflightState(ctx, clusterInfo, preflight.Type, target, params)
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
	case storage.OperationTypeSwitchover, storage.OperationTypeReload, storage.OperationTypeRollingRestart, storage.OperationTypeReplicaReinit,
		storage.OperationTypeBackupFull, storage.OperationTypeBackupDiff,
		storage.OperationTypeQueryAnalyticsEnable, storage.OperationTypeQueryAnalyticsDisable,
		storage.OperationTypeNodeAdd, storage.OperationTypeNodeRemove, storage.OperationTypeConfigUpdate,
		storage.OperationTypeRollingUpdate, storage.OperationTypePostgreSQLUpgrade, storage.OperationTypeEmergencyFailover,
		storage.OperationTypeRestore, storage.OperationTypePITR, storage.OperationTypeDatabaseAdmin,
		storage.OperationTypeExtensionAdmin, storage.OperationTypePgBouncerAdmin:
		return true
	default:
		return false
	}
}

func (h *guardedOperationsHandler) preflightState(ctx context.Context, clusterInfo *storage.Cluster, operationType, target string, params []byte) (state *guardedPreflight, err error) {
	switch operationType {
	case storage.OperationTypeSwitchover:
		state, err = h.switchoverPreflightState(ctx, clusterInfo, target)
	case storage.OperationTypeReload, storage.OperationTypeRollingRestart:
		state, err = h.maintenancePreflightState(ctx, clusterInfo, operationType)
	case storage.OperationTypeReplicaReinit:
		state, err = h.replicaReinitPreflightState(ctx, clusterInfo, target)
	case storage.OperationTypeBackupFull, storage.OperationTypeBackupDiff:
		state, err = h.backupPreflightState(ctx, clusterInfo, operationType)
	case storage.OperationTypeQueryAnalyticsEnable, storage.OperationTypeQueryAnalyticsDisable:
		state, err = h.queryAnalyticsPreflightState(ctx, clusterInfo, operationType)
	case storage.OperationTypeNodeAdd, storage.OperationTypeNodeRemove, storage.OperationTypeConfigUpdate:
		state, err = h.lifecyclePreflightState(ctx, clusterInfo, operationType, target, params)
	case storage.OperationTypeRollingUpdate, storage.OperationTypePostgreSQLUpgrade, storage.OperationTypeEmergencyFailover:
		state, err = h.phase2PreflightState(ctx, clusterInfo, operationType, target, params)
	case storage.OperationTypeRestore, storage.OperationTypePITR:
		state, err = h.recoveryPreflightState(ctx, clusterInfo, operationType, target, params)
	case storage.OperationTypeDatabaseAdmin:
		state, err = h.databaseAdminPreflightState(ctx, clusterInfo, target, params)
	case storage.OperationTypeExtensionAdmin, storage.OperationTypePgBouncerAdmin:
		state, err = h.phase3ServicesPreflightState(ctx, clusterInfo, operationType, target, params)
	default:
		return nil, errors.New("unsupported operation type")
	}
	if err != nil {
		return nil, err
	}
	if _, credentialErr := h.managementCredential(ctx, clusterInfo); credentialErr != nil {
		state.blockers = append(state.blockers, "management credential attached")
	}
	if err = h.bindAutomationCredentialPreflight(ctx, clusterInfo, operationType, state); err != nil {
		return nil, err
	}
	return state, nil
}

func (h *guardedOperationsHandler) operationInputs(ctx context.Context, clusterInfo *storage.Cluster, operationType string, desired []byte) ([]string, []byte, string, error) {
	var (
		envs      []string
		extraVars []byte
		playbook  string
		err       error
	)
	switch operationType {
	case storage.OperationTypeSwitchover:
		envs, extraVars, playbook = nil, nil, switchoverPlaybook
		envs, extraVars, err = h.switchoverOperationInputs(ctx, clusterInfo, desired)
	case storage.OperationTypeReload, storage.OperationTypeRollingRestart:
		envs, extraVars, playbook, err = h.maintenanceOperationInputs(ctx, clusterInfo, operationType, desired)
	case storage.OperationTypeReplicaReinit:
		playbook = replicaReinitPlaybook
		envs, extraVars, err = h.replicaReinitOperationInputs(ctx, clusterInfo, desired)
	case storage.OperationTypeBackupFull, storage.OperationTypeBackupDiff:
		playbook = backupPlaybook
		envs, extraVars, err = h.backupOperationInputs(ctx, clusterInfo, operationType, desired)
	case storage.OperationTypeQueryAnalyticsEnable, storage.OperationTypeQueryAnalyticsDisable:
		state, _ := queryAnalyticsState(operationType)
		playbook = queryAnalyticsPlaybook
		envs, extraVars, err = h.queryAnalyticsOperationInputs(ctx, clusterInfo, state)
	case storage.OperationTypeNodeAdd, storage.OperationTypeNodeRemove, storage.OperationTypeConfigUpdate:
		playbook = lifecyclePlaybook
		envs, extraVars, err = h.lifecycleOperationInputs(ctx, clusterInfo, operationType, desired)
	case storage.OperationTypeRollingUpdate, storage.OperationTypePostgreSQLUpgrade, storage.OperationTypeEmergencyFailover:
		playbook = phase2Playbook
		envs, extraVars, err = h.phase2OperationInputs(ctx, clusterInfo, operationType, desired)
	case storage.OperationTypeRestore, storage.OperationTypePITR:
		playbook = recoveryPlaybook
		envs, extraVars, err = h.recoveryOperationInputs(ctx, clusterInfo, operationType, desired)
	case storage.OperationTypeDatabaseAdmin:
		playbook = databaseAdminPlaybook
		envs, extraVars, err = h.databaseAdminOperationInputs(ctx, clusterInfo, desired)
	case storage.OperationTypeExtensionAdmin, storage.OperationTypePgBouncerAdmin:
		playbook = phase3ServicesPlaybook
		envs, extraVars, err = h.phase3ServicesOperationInputs(ctx, clusterInfo, operationType, desired)
	default:
		return nil, nil, "", errors.New("unsupported operation type")
	}
	if err != nil {
		return nil, nil, "", err
	}
	extraVars, err = h.injectAutomationCredentials(ctx, clusterInfo, operationType, extraVars)
	return envs, extraVars, playbook, err
}

func (h *guardedOperationsHandler) baseOperationInputs(ctx context.Context, clusterInfo *storage.Cluster) ([]string, map[string]any, error) {
	secretID, err := h.managementCredential(ctx, clusterInfo)
	if err != nil {
		return nil, nil, err
	}
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
	secretValues, location, err := getSecretEnvs(ctx, h.log, h.db, secretID, h.cfg.EncryptionKey)
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
	return envs, extraVars, nil
}

func (h *guardedOperationsHandler) managementCredential(ctx context.Context, clusterInfo *storage.Cluster) (int64, error) {
	if clusterInfo.SecretID == nil {
		return 0, errors.New("management credential is not attached")
	}
	secret, err := h.db.GetSecret(ctx, *clusterInfo.SecretID)
	if err != nil {
		return 0, err
	}
	if secret == nil {
		return 0, errors.New("management credential is unavailable")
	}
	if secret.ProjectID != clusterInfo.ProjectID ||
		secret.Type != string(models.SecretTypeSSHKey) && secret.Type != string(models.SecretTypePassword) {
		return 0, errors.New("management credential is invalid")
	}
	return secret.ID, nil
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
