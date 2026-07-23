package watcher

import (
	"context"
	"strings"
	"testing"

	"postgresql-cluster-console/internal/storage"
	"postgresql-cluster-console/internal/xdocker"
)

type logStorage struct {
	storage.IStorage
	log string
}

func (s *logStorage) UpdateOperation(_ context.Context, req *storage.UpdateOperationReq) (*storage.Operation, error) {
	if req.Logs != nil {
		s.log = *req.Logs
	}
	return &storage.Operation{ID: req.ID}, nil
}

type logDocker struct{ xdocker.IManager }

func (logDocker) StoreContainerLogs(_ context.Context, _ xdocker.InstanceID, store func(string)) {
	store(`{"password":"cleartext","task":"configure"}`)
}

func TestOperationLogIsRedactedBeforePersistence(t *testing.T) {
	store := &logStorage{}
	collector := NewLogCollector(store, logDocker{}).(*logCollector)
	collector.storeLogsFromContainer(7, "container", "test-cid")
	if strings.Contains(store.log, "cleartext") || !strings.Contains(store.log, "[REDACTED]") {
		t.Fatalf("stored log=%q", store.log)
	}
}
