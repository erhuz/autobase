package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"postgresql-cluster-console/internal/configuration"
	"postgresql-cluster-console/internal/storage"
	"postgresql-cluster-console/internal/xdocker"
	"postgresql-cluster-console/pkg/tracer"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type backupWatcher struct {
	db     storage.IStorage
	docker xdocker.IManager
	cfg    *configuration.Config
	log    zerolog.Logger
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewBackupWatcher(db storage.IStorage, docker xdocker.IManager, cfg *configuration.Config) *backupWatcher {
	return &backupWatcher{
		db: db, docker: docker, cfg: cfg,
		log: log.Logger.With().Str("module", "backup_watcher").Logger(),
	}
}

func (w *backupWatcher) Run() {
	if w.cancel != nil {
		return
	}
	w.ctx, w.cancel = context.WithCancel(context.Background())
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		w.loop()
	}()
}

func (w *backupWatcher) Stop() {
	if w.cancel == nil {
		return
	}
	w.cancel()
	w.wg.Wait()
	w.cancel = nil
}

func (w *backupWatcher) loop() {
	ticker := time.NewTicker(w.cfg.Backup.RunEvery)
	defer ticker.Stop()
	w.doWork()
	for {
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			w.doWork()
		}
	}
}

func (w *backupWatcher) doWork() {
	limit := int64(1000)
	projects, _, err := w.db.GetProjects(w.ctx, &limit, nil)
	if err != nil {
		w.log.Error().Err(err).Msg("backup project scan failed")
		return
	}
	for _, project := range projects {
		clusters, _, err := w.db.GetClusters(w.ctx, &storage.GetClustersReq{ProjectID: project.ID, Limit: &limit})
		if err != nil {
			w.log.Error().Err(err).Int64("project_id", project.ID).Msg("backup cluster scan failed")
			continue
		}
		for i := range clusters {
			if backupEnabled(clusters[i].ExtraVars) {
				w.observe(&clusters[i])
			}
		}
	}
}

func backupEnabled(raw []byte) bool {
	var values map[string]any
	if json.Unmarshal(raw, &values) != nil {
		return false
	}
	switch value := values["pgbackrest_install"].(type) {
	case bool:
		return value
	case string:
		return value == "true"
	default:
		return false
	}
}

func (w *backupWatcher) observe(clusterInfo *storage.Cluster) {
	cid := uuid.NewString()
	ctx, cancel := context.WithTimeout(context.WithValue(w.ctx, tracer.CtxCidKey{}, cid), w.cfg.Backup.Timeout)
	defer cancel()
	inputs := &guardedOperationsHandler{db: w.db, cfg: w.cfg, log: w.log}
	envs, extraVars, err := inputs.baseOperationInputs(ctx, clusterInfo)
	if err != nil {
		w.log.Error().Err(err).Int64("cluster_id", clusterInfo.ID).Msg("backup evidence inputs failed")
		return
	}
	envs = backupObserverEnvs(envs)
	extraVars["patroni_cluster_name"] = clusterInfo.Name
	delete(extraVars, "backup_operation_type")
	payload, err := json.Marshal(extraVars)
	if err != nil {
		return
	}
	id, err := w.docker.ManageCluster(ctx, &xdocker.ManageClusterConfig{
		Envs: envs, ExtraVars: string(payload), Playbook: backupPlaybook,
	})
	if err != nil {
		w.log.Error().Err(err).Int64("cluster_id", clusterInfo.ID).Msg("backup evidence automation failed")
		return
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if cleanupErr := w.docker.RemoveContainer(cleanupCtx, id); cleanupErr != nil {
			w.log.Warn().Err(cleanupErr).Int64("cluster_id", clusterInfo.ID).Msg("backup evidence container cleanup failed")
		}
	}()

	var evidence *storage.BackupEvidence
	var parseErr error
	w.docker.StoreContainerLogs(ctx, id, func(message string) {
		if strings.Contains(message, storage.BackupEvidenceMarker) {
			evidence, parseErr = storage.DecodeBackupEvidence(message, clusterInfo.ID)
		}
	})
	status, err := w.docker.GetStatus(ctx, id)
	if err != nil || status != "exited" || evidence == nil {
		w.log.Error().Err(errors.Join(err, parseErr)).Str("status", status).Int64("cluster_id", clusterInfo.ID).Msg("backup evidence incomplete")
		return
	}
	if err = w.db.UpsertBackupEvidence(ctx, evidence); err != nil {
		w.log.Error().Err(err).Int64("cluster_id", clusterInfo.ID).Msg("backup evidence store failed")
	}
}

func backupObserverEnvs(envs []string) []string {
	result := envs[:0]
	for _, value := range envs {
		if !strings.HasPrefix(value, "ANSIBLE_JSON_LOG_FILE=") {
			result = append(result, value)
		}
	}
	return result
}
