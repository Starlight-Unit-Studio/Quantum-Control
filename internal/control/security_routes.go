package control

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/Starlight-Unit-Studio/Quantum-Control/internal/security"
)

type confirmationRequest struct {
	PlanID string `json:"plan_id"`
}

func (s *Server) handleAuditQuery(w http.ResponseWriter, r *http.Request) {
	if s.security.Audit == nil {
		writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Durable audit storage is unavailable.")
		return
	}
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 500 {
			writeProblem(w, r, http.StatusBadRequest, "invalid_limit", "limit must be an integer from 1 through 500.")
			return
		}
		limit = parsed
	}
	actorID := strings.TrimSpace(r.URL.Query().Get("actor_id"))
	action := strings.TrimSpace(r.URL.Query().Get("action"))
	if len(actorID) > 128 || len(action) > 160 {
		writeProblem(w, r, http.StatusBadRequest, "invalid_filter", "Audit filters exceed the supported length.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"integrity": s.security.Audit.Integrity(),
		"records":   s.security.Audit.Query(limit, actorID, action),
	})
}

func (s *Server) handleAuditIntegrity(w http.ResponseWriter, _ *http.Request) {
	if s.security.Audit == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"verified": false, "status": "unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, s.security.Audit.Integrity())
}

func (s *Server) handleConfirmation(w http.ResponseWriter, r *http.Request) {
	if s.security.Grants == nil || s.security.Plans == nil {
		writeProblem(w, r, http.StatusServiceUnavailable, "confirmation_unavailable", "Confirmation storage is unavailable.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.RequestBodyLimit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request confirmationRequest
	if err := decoder.Decode(&request); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeProblem(w, r, http.StatusRequestEntityTooLarge, "request_too_large", "The request body exceeds the configured limit.")
			return
		}
		writeProblem(w, r, http.StatusBadRequest, "invalid_json", "A valid confirmation request is required.")
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeProblem(w, r, http.StatusBadRequest, "invalid_json", "The request body must contain exactly one JSON object.")
		return
	}
	request.PlanID = strings.TrimSpace(request.PlanID)
	if request.PlanID == "" || len(request.PlanID) > 160 {
		writeProblem(w, r, http.StatusBadRequest, "invalid_plan_id", "plan_id is required and must match a cached plan.")
		return
	}
	plan, found := s.security.Plans.Get(request.PlanID)
	if !found {
		writeProblem(w, r, http.StatusNotFound, "plan_not_found", "The operation plan does not exist or has expired.")
		return
	}
	approver := actorFromContext(r.Context())
	grant, err := s.security.Grants.Issue(plan, approver)
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "confirmation_rejected", "The operation plan cannot be confirmed under the current policy.")
		return
	}
	s.appendAudit(security.AuditEvent{
		Event: "confirmation.issued", Actor: approver,
		RequestID: requestIDFromContext(r.Context()), SessionID: sessionIDFromContext(r.Context()),
		PlanID: plan.ID, PlanDigest: plan.Digest, Action: plan.Action, Risk: plan.Risk,
		Status: "issued", Parameters: map[string]string{},
	})
	writeJSON(w, http.StatusCreated, grant)
}
