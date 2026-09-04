package control

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/Starlight-Unit-Studio/Quantum-Control/internal/broker"
	"github.com/Starlight-Unit-Studio/Quantum-Control/internal/security"
)

type approvedExecutionRequest struct {
	PlanID            string `json:"plan_id"`
	ConfirmationToken string `json:"confirmation_token"`
}

func (s *Server) handleApprovedExecute(w http.ResponseWriter, r *http.Request) {
	if s.security.Plans == nil {
		writeProblem(w, r, http.StatusServiceUnavailable, "mutation_unavailable", "Operation plan storage is unavailable.")
		return
	}
	approvalBroker, ok := s.broker.(broker.ApprovalAPI)
	if !ok {
		writeProblem(w, r, http.StatusServiceUnavailable, "mutation_unavailable", "The privileged broker does not support approved mutations.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.RequestBodyLimit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request approvedExecutionRequest
	if err := decoder.Decode(&request); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeProblem(w, r, http.StatusRequestEntityTooLarge, "request_too_large", "The request body exceeds the configured limit.")
			return
		}
		writeProblem(w, r, http.StatusBadRequest, "invalid_json", "A valid approved execution request is required.")
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeProblem(w, r, http.StatusBadRequest, "invalid_json", "The request body must contain exactly one JSON object.")
		return
	}
	request.PlanID = strings.TrimSpace(request.PlanID)
	request.ConfirmationToken = strings.TrimSpace(request.ConfirmationToken)
	if request.PlanID == "" || len(request.PlanID) > 160 {
		writeProblem(w, r, http.StatusBadRequest, "invalid_plan_id", "plan_id is required and must match a cached plan.")
		return
	}
	if request.ConfirmationToken == "" || len(request.ConfirmationToken) > 512 {
		writeProblem(w, r, http.StatusBadRequest, "invalid_confirmation_token", "A bounded confirmation token is required.")
		return
	}
	plan, found := s.security.Plans.Get(request.PlanID)
	if !found {
		writeProblem(w, r, http.StatusNotFound, "plan_not_found", "The operation plan does not exist or has expired.")
		return
	}
	executor := actorFromContext(r.Context())
	parameters := security.PlanParametersMap(plan)
	s.appendAudit(security.AuditEvent{
		Event: "operation.attempt", Actor: executor,
		RequestID: requestIDFromContext(r.Context()), SessionID: sessionIDFromContext(r.Context()),
		PlanID: plan.ID, PlanDigest: plan.Digest, Action: plan.Action, Risk: plan.Risk,
		Status: "attempted", RollbackStatus: "not_started", Parameters: parameters,
	})
	response, err := approvalBroker.ExecuteApproved(r.Context(), plan, request.ConfirmationToken, actorCredentialFromContext(r.Context()))
	if err != nil {
		s.logger.ErrorContext(r.Context(), "approved broker execution transport failed", "request_id", requestIDFromContext(r.Context()), "plan_id", plan.ID, "executor", executor.ID, "action", plan.Action, "error", err)
		s.appendAudit(security.AuditEvent{
			Event: "operation.unknown", Actor: executor,
			RequestID: requestIDFromContext(r.Context()), SessionID: sessionIDFromContext(r.Context()),
			PlanID: plan.ID, PlanDigest: plan.Digest, Action: plan.Action, Risk: plan.Risk,
			Status: "unknown", RollbackStatus: "unknown", Parameters: parameters, ErrorCode: "broker_transport_failed",
		})
		writeProblem(w, r, http.StatusBadGateway, "broker_error", "The privileged broker request failed after the execution attempt was submitted.")
		return
	}
	errorCode := ""
	if response.Error != nil {
		errorCode = response.Error.Code
	}
	rollbackStatus := recoveryStatus(response.Result)
	if rollbackStatus == "" {
		rollbackStatus = "not_required"
	}
	s.appendAudit(security.AuditEvent{
		Event: "operation." + response.Status, Actor: executor,
		RequestID: response.RequestID, SessionID: sessionIDFromContext(r.Context()),
		PlanID: plan.ID, PlanDigest: plan.Digest, Action: plan.Action, Risk: plan.Risk,
		Status: response.Status, RollbackStatus: rollbackStatus, Parameters: parameters, ErrorCode: errorCode,
	})
	writeJSON(w, operationHTTPStatus(response.Status), response)
}

func recoveryStatus(result map[string]any) string {
	if result == nil {
		return ""
	}
	value, _ := result["recovery_status"].(string)
	return strings.TrimSpace(value)
}
