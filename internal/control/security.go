package control

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Starlight-Unit-Studio/Quantum-Control/internal/config"
	"github.com/Starlight-Unit-Studio/Quantum-Control/internal/security"
)

type SecurityDependencies struct {
	Authenticator security.Authenticator
	Audit         *security.AuditStore
	Plans         *security.PlanCache
	PlanBuilder   security.PlanBuilder
}

func LoadSecurityDependencies(cfg config.Control) (SecurityDependencies, error) {
	authenticators := make(security.MultiAuthenticator, 0, 2)
	if cfg.ActorFile != "" {
		registry, err := security.LoadActorRegistry(cfg.ActorFile)
		if err != nil {
			return SecurityDependencies{}, fmt.Errorf("load actor registry: %w", err)
		}
		authenticators = append(authenticators, registry)
	}
	if cfg.APIToken != "" {
		legacy, err := security.NewStaticAuthenticator(security.LegacyServiceActor(), cfg.APIToken)
		if err != nil {
			return SecurityDependencies{}, fmt.Errorf("initialize legacy API identity: %w", err)
		}
		authenticators = append(authenticators, legacy)
	}

	audit, err := security.OpenAuditStore(cfg.AuditPath)
	if err != nil {
		return SecurityDependencies{}, fmt.Errorf("open audit store: %w", err)
	}
	var authenticator security.Authenticator
	if len(authenticators) > 0 {
		authenticator = authenticators
	}
	return SecurityDependencies{
		Authenticator: authenticator,
		Audit:         audit,
		Plans:         security.NewPlanCache(),
		PlanBuilder:   security.PlanBuilder{TTL: cfg.PlanTTL},
	}, nil
}

func defaultSecurityDependencies(cfg config.Control) SecurityDependencies {
	dependencies := SecurityDependencies{
		Plans:       security.NewPlanCache(),
		PlanBuilder: security.PlanBuilder{TTL: 5 * time.Minute},
	}
	if cfg.APIToken != "" {
		legacy, err := security.NewStaticAuthenticator(security.LegacyServiceActor(), cfg.APIToken)
		if err == nil {
			dependencies.Authenticator = legacy
		}
	}
	return dependencies
}

type actorContextKey struct{}
type sessionContextKey struct{}
type actorCredentialContextKey struct{}

func actorFromContext(ctx context.Context) security.Actor {
	actor, _ := ctx.Value(actorContextKey{}).(security.Actor)
	return actor
}

func sessionIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(sessionContextKey{}).(string)
	return value
}

func actorCredentialFromContext(ctx context.Context) string {
	value, _ := ctx.Value(actorCredentialContextKey{}).(string)
	return value
}

func (s *Server) requirePermission(permission security.Permission, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actor, credential, ok := s.authenticateRequest(r)
		if !ok {
			w.Header().Set("WWW-Authenticate", `Bearer realm="Quantum Control"`)
			writeProblem(w, r, http.StatusUnauthorized, "unauthorized", "A valid actor credential is required.")
			return
		}
		if !security.HasPermission(actor, permission) {
			writeProblem(w, r, http.StatusForbidden, "forbidden", "The authenticated actor does not have permission for this operation.")
			return
		}
		sessionID := strings.TrimSpace(r.Header.Get("X-Quantum-Session-ID"))
		if sessionID == "" {
			sessionID = "session-" + requestIDFromContext(r.Context())
		} else if !validCorrelationID(sessionID) {
			writeProblem(w, r, http.StatusBadRequest, "invalid_session_id", "X-Quantum-Session-ID does not match policy.")
			return
		}
		ctx := context.WithValue(r.Context(), actorContextKey{}, actor)
		ctx = context.WithValue(ctx, sessionContextKey{}, sessionID)
		ctx = context.WithValue(ctx, actorCredentialContextKey{}, credential)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) authenticateRequest(r *http.Request) (security.Actor, string, bool) {
	const prefix = "Bearer "
	header := r.Header.Get("Authorization")
	if s.security.Authenticator == nil {
		if strings.TrimSpace(header) != "" {
			return security.Actor{}, "", false
		}
		return security.LocalReadOnlyActor(), "", true
	}
	if !strings.HasPrefix(header, prefix) {
		return security.Actor{}, "", false
	}
	provided := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	actor, ok := s.security.Authenticator.AuthenticateBearer(provided)
	if !ok {
		return security.Actor{}, "", false
	}
	return actor, provided, true
}

func validCorrelationID(value string) bool {
	if len(value) < 1 || len(value) > 160 {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' || r == ':' || r == '@' {
			continue
		}
		return false
	}
	return true
}

func (s *Server) appendAudit(event security.AuditEvent) {
	if s.security.Audit == nil {
		return
	}
	if _, err := s.security.Audit.Append(event); err != nil {
		s.logger.Error("append audit record failed", "event", event.Event, "error", err)
	}
}

func (s *Server) securityReady() error {
	if s.security.Plans == nil {
		return errors.New("operation plan cache unavailable")
	}
	return nil
}
