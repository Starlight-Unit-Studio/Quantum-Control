package control

import (
	"net/http"
	"strings"

	"github.com/Starlight-Unit-Studio/Quantum-Control/internal/inventory"
	"github.com/Starlight-Unit-Studio/Quantum-Control/internal/security"
)

var defaultInventoryScanner inventory.Scanner = inventory.NewScanner(nil)

// ServeHTTP adds the read-only component inventory surface while preserving the
// broker-backed and security routes for all other requests.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet && (r.URL.Path == "/v1/components" || strings.HasPrefix(r.URL.Path, "/v1/components/")) {
		s.inventoryHandler(defaultInventoryScanner).ServeHTTP(w, r)
		return
	}
	s.Handler().ServeHTTP(w, r)
}

func (s *Server) inventoryHandler(scanner inventory.Scanner) http.Handler {
	if scanner == nil {
		scanner = inventory.NewScanner(nil)
	}
	mux := http.NewServeMux()
	mux.Handle("GET /v1/components", s.requirePermission(security.PermissionInventoryRead, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		snapshot, err := scanner.Snapshot(r.Context())
		if err != nil {
			s.logInventoryFailure(r, "snapshot", err)
			writeProblem(w, r, http.StatusInternalServerError, "inventory_unavailable", "The read-only component inventory could not be completed.")
			return
		}
		writeJSON(w, http.StatusOK, snapshot)
	})))
	mux.Handle("GET /v1/components/{id}", s.requirePermission(security.PermissionInventoryRead, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		component, found, err := scanner.Component(r.Context(), r.PathValue("id"))
		if err != nil {
			s.logInventoryFailure(r, "component", err)
			writeProblem(w, r, http.StatusInternalServerError, "inventory_unavailable", "The read-only component inventory could not be completed.")
			return
		}
		if !found {
			writeProblem(w, r, http.StatusNotFound, "component_not_found", "The requested component identifier is not part of the inventory contract.")
			return
		}
		writeJSON(w, http.StatusOK, component)
	})))
	return s.withRequestID(s.logRequests(mux))
}

func (s *Server) logInventoryFailure(r *http.Request, probe string, err error) {
	s.logger.ErrorContext(r.Context(), "component inventory failed",
		"request_id", requestIDFromContext(r.Context()),
		"probe", probe,
		"error", err,
	)
}
