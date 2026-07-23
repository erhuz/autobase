package operation

import (
	"encoding/json"

	"postgresql-cluster-console/internal/controllers"
	"postgresql-cluster-console/internal/redact"
	"postgresql-cluster-console/internal/storage"
	"postgresql-cluster-console/models"
	operationapi "postgresql-cluster-console/restapi/operations/operation"

	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/strfmt"
)

type getOperationHandler struct{ db storage.IStorage }

func NewGetOperationHandler(db storage.IStorage) operationapi.GetOperationsIDHandler {
	return &getOperationHandler{db: db}
}

func (h *getOperationHandler) Handle(param operationapi.GetOperationsIDParams) middleware.Responder {
	operation, err := h.db.GetOperation(param.HTTPRequest.Context(), param.ID)
	if err != nil {
		return operationapi.NewGetOperationsIDBadRequest().WithPayload(controllers.MakeErrorPayload(err, controllers.BaseError))
	}
	var params, snapshot, verification any
	var plan, affected []string
	_ = json.Unmarshal(operation.SanitizedParams, &params)
	_ = json.Unmarshal(operation.PreflightSnapshot, &snapshot)
	_ = json.Unmarshal(operation.Plan, &plan)
	_ = json.Unmarshal(operation.AffectedNodes, &affected)
	_ = json.Unmarshal(operation.FinalVerification, &verification)
	for i := range plan {
		plan[i] = redact.Text(plan[i])
	}
	for i := range affected {
		affected[i] = redact.Text(affected[i])
	}
	response := &models.ResponseOperationDetail{
		ID: operation.ID, ClusterID: operation.ClusterID, Type: operation.Type, Status: operation.Status,
		Actor: operation.Actor, SanitizedParams: redact.Value(params), PreflightSnapshot: redact.Value(snapshot), Plan: plan,
		AffectedNodes: affected, FinalVerification: redact.Value(verification), SafeNextAction: operation.SafeNextAction,
		Started: strfmt.DateTime(operation.CreatedAt),
	}
	if operation.UpdatedAt != nil && storage.IsTerminalOperationStatus(operation.Status) {
		finished := strfmt.DateTime(*operation.UpdatedAt)
		response.Finished = &finished
	}
	return operationapi.NewGetOperationsIDOK().WithPayload(response)
}
