package operation

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"postgresql-cluster-console/internal/storage"
	operationapi "postgresql-cluster-console/restapi/operations/operation"
)

type operationDetailStorage struct {
	storage.IStorage
	operation *storage.Operation
}

func (s *operationDetailStorage) GetOperation(context.Context, int64) (*storage.Operation, error) {
	return s.operation, nil
}

func TestOperationDetailAndLogRedactLegacySecrets(t *testing.T) {
	now := time.Now().UTC()
	log := "PASSWORD=cleartext Authorization: Bearer abc123"
	store := &operationDetailStorage{operation: &storage.Operation{
		ID: 7, ClusterID: 5, Type: storage.OperationTypeQueryAnalyticsEnable, Status: storage.OperationStatusRunning,
		Actor: "api-token", SanitizedParams: []byte(`{"password":"cleartext","state":"enabled"}`),
		PreflightSnapshot: []byte(`{"token":"abc123","observed":{"node_count":3}}`),
		Plan:              []byte(`["run with password=cleartext"]`), AffectedNodes: []byte(`["postgresql-1"]`),
		FinalVerification: []byte(`{"stderr":"secret failure","verified":false}`),
		Log:               &log, CreatedAt: now, UpdatedAt: &now,
	}}
	request := httptest.NewRequest("GET", "/operations/7", nil)
	detailResponse := NewGetOperationHandler(store).Handle(operationapi.GetOperationsIDParams{ID: 7, HTTPRequest: request})
	detail, ok := detailResponse.(*operationapi.GetOperationsIDOK)
	if !ok {
		t.Fatalf("detail response=%#v", detailResponse)
	}
	payload := detail.Payload
	params := payload.SanitizedParams.(map[string]any)
	snapshot := payload.PreflightSnapshot.(map[string]any)
	verification := payload.FinalVerification.(map[string]any)
	if params["password"] != "[REDACTED]" || snapshot["token"] != "[REDACTED]" ||
		verification["stderr"] != "[REDACTED]" || strings.Contains(payload.Plan[0], "cleartext") ||
		payload.Finished != nil {
		t.Fatalf("detail payload=%+v", payload)
	}

	store.operation.Status = storage.OperationStatusFailed
	detailResponse = NewGetOperationHandler(store).Handle(operationapi.GetOperationsIDParams{ID: 7, HTTPRequest: request})
	if detailResponse.(*operationapi.GetOperationsIDOK).Payload.Finished == nil {
		t.Fatal("terminal operation has no finished timestamp")
	}
	logResponse := NewGetOperationLogHandler(store).Handle(operationapi.GetOperationsIDLogParams{ID: 7, HTTPRequest: request})
	logPayload, ok := logResponse.(*operationapi.GetOperationsIDLogOK)
	if !ok || !logPayload.XLogCompleted || strings.Contains(logPayload.Payload, "cleartext") || strings.Contains(logPayload.Payload, "abc123") {
		t.Fatalf("log response=%#v", logResponse)
	}
}
