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
)

// Server exposes the privileged operation registry over a protected Unix
// socket. The transport does not accept arbitrary commands.
type Server struct {
	registry  *Registry
	token     string
	bodyLimit int64
	logger    *slog.Logger
}

func NewServer(registry *Registry, token string, bodyLimit int64, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{registry: registry, token: token, bodyLimit: bodyLimit, logger: logger}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.Handle("GET /v1/operations", s.requireBrokerAuth(http.HandlerFunc(s.handleCatalog)))
	mux.Handle("POST /v1/plan", s.requireBrokerAuth(http.HandlerFunc(s.handlePlan)))
	mux.Handle("POST /v1/execute", s.requireBrokerAuth(http.HandlerFunc(s.handleExecute)))
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"service": "qcored",
		"version": buildinfo.Version,
	})
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
	s.logger.InfoContext(r.Context(), "broker operation",
		"audit_id", response.AuditID,
		"request_id", response.RequestID,
		"actor", request.Actor,
		"action", response.Action,
		"risk", response.Risk,
		"status", response.Status,
		"duration_ms", response.FinishedAt.Sub(response.StartedAt).Milliseconds(),
	)
	status := http.StatusOK
	switch response.Status {
	case "rejected":
		status = http.StatusBadRequest
	case "failed":
		status = http.StatusInternalServerError
	}
	writeJSON(w, status, response)
}

func (s *Server) decodeRequest(w http.ResponseWriter, r *http.Request) (protocol.OperationRequest, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, s.bodyLimit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request protocol.OperationRequest
	if err := decoder.Decode(&request); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_json", err.Error())
		return protocol.OperationRequest{}, false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeProblem(w, http.StatusBadRequest, "invalid_json", "request body must contain exactly one JSON object")
		return protocol.OperationRequest{}, false
	}
	return request, true
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

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeProblem(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": protocol.Problem{Code: code, Message: message},
		"time":  time.Now().UTC(),
	})
}
