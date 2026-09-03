package control

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Starlight-Unit-Studio/Quantum-Control/internal/broker"
	"github.com/Starlight-Unit-Studio/Quantum-Control/internal/buildinfo"
	"github.com/Starlight-Unit-Studio/Quantum-Control/internal/config"
	"github.com/Starlight-Unit-Studio/Quantum-Control/internal/protocol"
	"github.com/Starlight-Unit-Studio/Quantum-Control/internal/security"
)

// Server is the unprivileged public API layer. It can request only operations
// published by qcored and applies actor/permission policy before broker access.
type Server struct {
	broker   broker.API
	cfg      config.Control
	logger   *slog.Logger
	security SecurityDependencies
}

func NewServer(client broker.API, cfg config.Control, logger *slog.Logger) *Server {
	return NewServerWithSecurity(client, cfg, logger, defaultSecurityDependencies(cfg))
}

func NewServerWithSecurity(client broker.API, cfg config.Control, logger *slog.Logger, dependencies SecurityDependencies) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	if dependencies.Plans == nil {
		dependencies.Plans = security.NewPlanCache()
	}
	if dependencies.PlanBuilder.TTL <= 0 {
		dependencies.PlanBuilder = security.PlanBuilder{TTL: 5 * time.Minute}
	}
	return &Server{broker: client, cfg: cfg, logger: logger, security: dependencies}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleRoot)
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /readyz", s.handleReady)
	mux.Handle("GET /v1/control/info", s.requirePermission(security.PermissionControlRead, http.HandlerFunc(s.handleInfo)))
	mux.Handle("GET /v1/operations", s.requirePermission(security.PermissionOperationCatalog, http.HandlerFunc(s.handleOperations)))
	mux.Handle("POST /v1/operations/plan", s.requirePermission(security.PermissionOperationPlan, http.HandlerFunc(s.handlePlan)))
	mux.Handle("POST /v1/operations/execute", s.requirePermission(security.PermissionOperationExecute, http.HandlerFunc(s.handleExecute)))
	mux.Handle("GET /v1/system/status", s.requirePermission(security.PermissionControlRead, http.HandlerFunc(s.handleSystemStatus)))
	mux.Handle("GET /v1/services/{unit}", s.requirePermission(security.PermissionControlRead, http.HandlerFunc(s.handleServiceStatus)))
	mux.Handle("GET /v1/audit", s.requirePermission(security.PermissionAuditRead, http.HandlerFunc(s.handleAuditQuery)))
	mux.Handle("GET /v1/audit/integrity", s.requirePermission(security.PermissionAuditRead, http.HandlerFunc(s.handleAuditIntegrity)))
	mux.Handle("POST /v1/confirmations", s.requirePermission(security.PermissionConfirm, http.HandlerFunc(s.handleConfirmation)))
	return s.withRequestID(s.logRequests(mux))
}

func (s *Server) handleRoot(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"service": "quantum-control",
		"version": buildinfo.Version,
		"status":  "online",
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"service": "quantum-control",
		"version": buildinfo.Version,
	})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if err := s.securityReady(); err != nil {
		s.logger.ErrorContext(r.Context(), "security subsystem unavailable", "error", err)
		writeProblem(w, r, http.StatusServiceUnavailable, "security_unavailable", "The security subsystem is unavailable.")
		return
	}
	if err := s.broker.Health(r.Context()); err != nil {
		s.logBrokerFailure(r, "health", err)
		writeProblem(w, r, http.StatusServiceUnavailable, "broker_unavailable", "The privileged broker is unavailable.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ready", "broker": "qcored", "security": "ready"})
}

func (s *Server) handleInfo(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"service":    "quantum-control",
		"version":    buildinfo.Version,
		"commit":     buildinfo.Commit,
		"build_time": buildinfo.BuildTime,
		"broker":     "qcored",
		"capabilities": map[string]bool{
			"typed_operations":       true,
			"operation_plans":        true,
			"actor_permissions":      true,
			"confirmation_contracts": true,
			"durable_audit":          s.security.Audit != nil,
			"system_snapshot":        true,
			"service_status":         true,
			"mutations":              false,
			"web_ui":                 false,
		},
	})
}

func (s *Server) handleOperations(w http.ResponseWriter, r *http.Request) {
	operations, err := s.broker.Catalog(r.Context())
	if err != nil {
		s.brokerGatewayError(w, r, "catalog", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"operations": operations})
}

func (s *Server) handlePlan(w http.ResponseWriter, r *http.Request) {
	request, ok := s.decodeOperation(w, r)
	if !ok {
		return
	}
	s.prepareRequest(r, &request)
	brokerPlan, err := s.broker.Plan(r.Context(), request)
	if err != nil {
		s.brokerGatewayError(w, r, "plan", err)
		return
	}
	actor := actorFromContext(r.Context())
	plan, err := s.security.PlanBuilder.Build(actor, sessionIDFromContext(r.Context()), brokerPlan)
	if err != nil {
		writeProblem(w, r, http.StatusInternalServerError, "plan_contract_error", "The immutable operation plan could not be created.")
		return
	}
	s.security.Plans.Put(plan)
	event := "plan.created"
	status := "valid"
	if actor.Kind == security.ActorTCI {
		event = "proposal.created"
	}
	if !plan.Valid {
		status = "rejected"
	}
	s.appendAudit(security.AuditEvent{
		Event: event, Actor: actor, RequestID: plan.RequestID, SessionID: plan.SessionID,
		PlanID: plan.ID, PlanDigest: plan.Digest, Action: plan.Action, Risk: plan.Risk,
		Status: status, Parameters: security.PlanParametersMap(plan), ErrorCode: plan.ErrorCode,
	})
	httpStatus := http.StatusOK
	if !plan.Valid {
		httpStatus = http.StatusBadRequest
	}
	writeJSON(w, httpStatus, plan)
}

func (s *Server) handleExecute(w http.ResponseWriter, r *http.Request) {
	request, ok := s.decodeOperation(w, r)
	if !ok {
		return
	}
	s.prepareRequest(r, &request)
	response, err := s.broker.Execute(r.Context(), request)
	if err != nil {
		s.brokerGatewayError(w, r, "execute", err)
		return
	}
	actor := actorFromContext(r.Context())
	errorCode := ""
	if response.Error != nil {
		errorCode = response.Error.Code
	}
	s.appendAudit(security.AuditEvent{
		Event: "operation." + response.Status, Actor: actor,
		RequestID: response.RequestID, SessionID: sessionIDFromContext(r.Context()),
		Action: response.Action, Risk: string(response.Risk), Status: response.Status,
		Parameters: request.Parameters, ErrorCode: errorCode,
	})
	writeJSON(w, operationHTTPStatus(response.Status), response)
}

func (s *Server) handleSystemStatus(w http.ResponseWriter, r *http.Request) {
	request := protocol.OperationRequest{
		RequestID: requestIDFromContext(r.Context()),
		Action:    "system.snapshot",
		Actor:     actorFromContext(r.Context()).ID,
	}
	response, err := s.broker.Execute(r.Context(), request)
	if err != nil {
		s.brokerGatewayError(w, r, "system.snapshot", err)
		return
	}
	if response.Status != "completed" {
		writeJSON(w, operationHTTPStatus(response.Status), response)
		return
	}
	writeJSON(w, http.StatusOK, response.Result)
}

func (s *Server) handleServiceStatus(w http.ResponseWriter, r *http.Request) {
	unit := r.PathValue("unit")
	request := protocol.OperationRequest{
		RequestID:  requestIDFromContext(r.Context()),
		Action:     "service.status",
		Actor:      actorFromContext(r.Context()).ID,
		Parameters: map[string]string{"unit": unit},
	}
	response, err := s.broker.Execute(r.Context(), request)
	if err != nil {
		s.brokerGatewayError(w, r, "service.status", err)
		return
	}
	if response.Status != "completed" {
		writeJSON(w, operationHTTPStatus(response.Status), response)
		return
	}
	writeJSON(w, http.StatusOK, response.Result)
}

func (s *Server) decodeOperation(w http.ResponseWriter, r *http.Request) (protocol.OperationRequest, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.RequestBodyLimit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request protocol.OperationRequest
	if err := decoder.Decode(&request); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeProblem(w, r, http.StatusRequestEntityTooLarge, "request_too_large", "The request body exceeds the configured limit.")
			return protocol.OperationRequest{}, false
		}
		writeProblem(w, r, http.StatusBadRequest, "invalid_json", err.Error())
		return protocol.OperationRequest{}, false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeProblem(w, r, http.StatusBadRequest, "invalid_json", "The request body must contain exactly one JSON object.")
		return protocol.OperationRequest{}, false
	}
	return request, true
}

func (s *Server) prepareRequest(r *http.Request, request *protocol.OperationRequest) {
	request.Actor = actorFromContext(r.Context()).ID
	request.Confirmation = ""
	if strings.TrimSpace(request.RequestID) == "" {
		request.RequestID = requestIDFromContext(r.Context())
	}
}

func (s *Server) brokerGatewayError(w http.ResponseWriter, r *http.Request, operation string, err error) {
	s.logBrokerFailure(r, operation, err)
	writeProblem(w, r, http.StatusBadGateway, "broker_error", "The privileged broker request failed.")
}

func (s *Server) logBrokerFailure(r *http.Request, operation string, err error) {
	s.logger.ErrorContext(r.Context(), "broker request failed",
		"request_id", requestIDFromContext(r.Context()),
		"operation", operation,
		"error", err,
	)
}

func (s *Server) withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := newRequestID()
		w.Header().Set("X-Quantum-Request-ID", requestID)
		ctx := context.WithValue(r.Context(), requestIDKey{}, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		s.logger.InfoContext(r.Context(), "http request",
			"request_id", requestIDFromContext(r.Context()),
			"method", r.Method,
			"path", r.URL.Path,
			"status", recorder.status,
			"duration_ms", time.Since(started).Milliseconds(),
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(status int) {
	if r.wroteHeader {
		return
	}
	r.status = status
	r.wroteHeader = true
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(data []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	return r.ResponseWriter.Write(data)
}

func operationHTTPStatus(status string) int {
	switch status {
	case "completed":
		return http.StatusOK
	case "rejected":
		return http.StatusBadRequest
	case "failed":
		return http.StatusInternalServerError
	default:
		return http.StatusBadGateway
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeProblem(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error":      protocol.Problem{Code: code, Message: message},
		"request_id": requestIDFromContext(r.Context()),
	})
}

type requestIDKey struct{}

func requestIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey{}).(string)
	return value
}

func newRequestID() string {
	var value [12]byte
	if _, err := rand.Read(value[:]); err != nil {
		return fmt.Sprintf("request-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(value[:])
}
