package broker

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Starlight-Unit-Studio/Quantum-Control/internal/buildinfo"
	"github.com/Starlight-Unit-Studio/Quantum-Control/internal/protocol"
	"github.com/Starlight-Unit-Studio/Quantum-Control/internal/security"
)

type Server struct {
	registry  *Registry
	token     string
	bodyLimit int64
	logger    *slog.Logger
	security  SecurityBoundary
}

func NewServer(registry *Registry, token string, bodyLimit int64, logger *slog.Logger) *Server {
	return NewServerWithSecurity(registry, token, bodyLimit, logger, SecurityBoundary{})
}

func NewServerWithSecurity(registry *Registry, token string, bodyLimit int64, logger *slog.Logger, boundary SecurityBoundary) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{registry: registry, token: token, bodyLimit: bodyLimit, logger: logger, security: boundary}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.Handle("GET /v1/operations", s.requireBrokerAuth(http.HandlerFunc(s.handleCatalog)))
	mux.Handle("POST /v1/plan", s.requireBrokerAuth(http.HandlerFunc(s.handlePlan)))
	mux.Handle("POST /v1/execute", s.requireBrokerAuth(http.HandlerFunc(s.handleExecute)))
	mux.Handle("POST /v1/confirm", s.requireBrokerAuth(http.HandlerFunc(s.handleConfirm)))
	mux.Handle("POST /v1/execute-approved", s.requireBrokerAuth(http.HandlerFunc(s.handleExecuteApproved)))
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "service": "qcored", "version": buildinfo.Version})
}

func (s *Server) handleCatalog(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"operations": s.registry.Catalog()})
}

func (s *Server) handlePlan(w http.ResponseWriter, r *http.Request) {
	request, ok := s.decodeRequest(w, r)
	if !ok {
		return
	}
	plan := s.registry.Plan(request)
	status := http.StatusOK
	if !plan.Valid {
		status = http.StatusBadRequest
	}
	writeJSON(w, status, plan)
}

func (s *Server) handleExecute(w http.ResponseWriter, r *http.Request) {
	request, ok := s.decodeRequest(w, r)
	if !ok {
		return
	}
	response := s.registry.Execute(r.Context(), request)
	s.logOperation(r, response, request.Actor, "")
	writeOperationResponse(w, response)
}

func (s *Server) handleConfirm(w http.ResponseWriter, r *http.Request) {
	if s.security.Grants == nil {
		writeProblem(w, http.StatusServiceUnavailable, "confirmation_unavailable", "broker confirmation state is unavailable")
		return
	}
	var envelope confirmationEnvelope
	if !s.decodeJSON(w, r, &envelope) {
		return
	}
	if problem := s.registry.ValidateApprovedPlan(envelope.Plan); problem != nil {
		writeProblem(w, http.StatusBadRequest, problem.Code, problem.Message)
		return
	}
	approver, err := s.security.authenticateActor(envelope.ActorToken)
	if err != nil {
		writeProblem(w, http.StatusUnauthorized, "approver_unauthorized", "valid human approver credential required")
		return
	}
	if approver.Kind != security.ActorHuman || !security.HasPermission(approver, security.PermissionConfirm) {
		writeProblem(w, http.StatusForbidden, "approver_forbidden", "actor may not issue confirmation grants")
		return
	}
	grant, err := s.security.Grants.Issue(envelope.Plan, approver)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "confirmation_rejected", "operation plan cannot be confirmed under current policy")
		return
	}
	s.logger.InfoContext(r.Context(), "broker confirmation issued",
		"grant_id", grant.Grant.ID,
		"plan_id", envelope.Plan.ID,
		"plan_digest", envelope.Plan.Digest,
		"plan_actor", envelope.Plan.Actor.ID,
		"approver", approver.ID,
		"action", envelope.Plan.Action,
	)
	writeJSON(w, http.StatusCreated, grant)
}

func (s *Server) handleExecuteApproved(w http.ResponseWriter, r *http.Request) {
	var envelope approvedExecutionEnvelope
	if !s.decodeJSON(w, r, &envelope) {
		return
	}
	response := rejectedApprovedResponse(envelope.Plan)
	if s.security.Grants == nil {
		response.Error = &protocol.Problem{Code: "confirmation_unavailable", Message: "broker confirmation state is unavailable"}
		response.FinishedAt = time.Now().UTC()
		writeOperationResponse(w, response)
		return
	}
	executor, err := s.security.authenticateActor(envelope.ActorToken)
	if err != nil || executor.Kind == security.ActorTCI || !security.HasPermission(executor, security.PermissionOperationMutate) {
		response.Error = &protocol.Problem{Code: "mutation_forbidden", Message: "executor is not authorized for service mutations"}
		response.FinishedAt = time.Now().UTC()
		writeOperationResponse(w, response)
		return
	}
	if problem := s.registry.ValidateApprovedPlan(envelope.Plan); problem != nil {
		response.Error = problem
		response.FinishedAt = time.Now().UTC()
		writeOperationResponse(w, response)
		return
	}
	if _, err := s.security.Grants.Consume(envelope.ConfirmationToken, envelope.Plan, envelope.Plan.Actor.ID, envelope.Plan.Action); err != nil {
		response.Error = &protocol.Problem{Code: "confirmation_rejected", Message: "confirmation grant is invalid, expired, stale, or already consumed"}
		response.FinishedAt = time.Now().UTC()
		writeOperationResponse(w, response)
		return
	}
	response = s.registry.ExecuteApproved(r.Context(), envelope.Plan, executor)
	s.logOperation(r, response, envelope.Plan.Actor.ID, executor.ID)
	writeOperationResponse(w, response)
}

func rejectedApprovedResponse(plan security.OperationPlan) protocol.OperationResponse {
	now := time.Now().UTC()
	requestID := plan.RequestID
	if requestID == "" {
		requestID = newID("request")
	}
	return protocol.OperationResponse{RequestID: requestID, Action: plan.Action, Status: "rejected", Risk: protocol.Risk(plan.Risk), StartedAt: now, AuditID: newID("audit")}
}

func (s *Server) logOperation(r *http.Request, response protocol.OperationResponse, planActor, executor string) {
	s.logger.InfoContext(r.Context(), "broker operation",
		"audit_id", response.AuditID,
		"request_id", response.RequestID,
		"plan_actor", planActor,
		"executor", executor,
		"action", response.Action,
		"risk", response.Risk,
		"status", response.Status,
		"duration_ms", response.FinishedAt.Sub(response.StartedAt).Milliseconds(),
	)
}

func (s *Server) decodeRequest(w http.ResponseWriter, r *http.Request) (protocol.OperationRequest, bool) {
	var request protocol.OperationRequest
	if !s.decodeJSON(w, r, &request) {
		return protocol.OperationRequest{}, false
	}
	return request, true
}

func (s *Server) decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, s.bodyLimit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_json", "valid JSON request required")
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeProblem(w, http.StatusBadRequest, "invalid_json", "request body must contain exactly one JSON object")
		return false
	}
	return true
}

func (s *Server) requireBrokerAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provided := strings.TrimSpace(r.Header.Get("X-Quantum-Broker-Token"))
		if len(provided) != len(s.token) || subtle.ConstantTimeCompare([]byte(provided), []byte(s.token)) != 1 {
			writeProblem(w, http.StatusUnauthorized, "unauthorized", "valid broker token required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeOperationResponse(w http.ResponseWriter, response protocol.OperationResponse) {
	status := http.StatusOK
	switch response.Status {
	case "rejected":
		status = http.StatusBadRequest
	case "failed":
		status = http.StatusInternalServerError
	}
	writeJSON(w, status, response)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeProblem(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": protocol.Problem{Code: code, Message: message}, "time": time.Now().UTC()})
}
