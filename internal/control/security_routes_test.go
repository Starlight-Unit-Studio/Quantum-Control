package control

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/Starlight-Unit-Studio/Quantum-Control/internal/security"
)

const (
	testAuditorToken = "auditor-token-012345678901234567890123456789"
	testTCIToken     = "tci-token-012345678901234567890123456789012"
)

func TestAuditAPIRequiresAuditPermissionAndHasNoMutationRoute(t *testing.T) {
	dependencies := testSecurityDependencies(t)
	server := httptest.NewServer(NewServerWithSecurity(&fakeBroker{}, testConfig(""), discardLogger(), dependencies))
	defer server.Close()

	request, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/audit", nil)
	request.Header.Set("Authorization", "Bearer "+testTCIToken)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("TCI audit read returned %d", response.StatusCode)
	}

	request, _ = http.NewRequest(http.MethodGet, server.URL+"/v1/audit", nil)
	request.Header.Set("Authorization", "Bearer "+testAuditorToken)
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("auditor read returned %d", response.StatusCode)
	}

	request, _ = http.NewRequest(http.MethodPost, server.URL+"/v1/audit", bytes.NewBufferString(`{"forged":true}`))
	request.Header.Set("Authorization", "Bearer "+testAuditorToken)
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusMethodNotAllowed && response.StatusCode != http.StatusNotFound {
		t.Fatalf("audit mutation route unexpectedly exists: %d", response.StatusCode)
	}
}

func TestTCICanPlanButCannotExecuteOrConfirm(t *testing.T) {
	dependencies := testSecurityDependencies(t)
	server := httptest.NewServer(NewServerWithSecurity(&fakeBroker{}, testConfig(""), discardLogger(), dependencies))
	defer server.Close()

	request, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/operations/plan", bytes.NewBufferString(`{"action":"system.snapshot"}`))
	request.Header.Set("Authorization", "Bearer "+testTCIToken)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("TCI proposal returned %d", response.StatusCode)
	}

	request, _ = http.NewRequest(http.MethodPost, server.URL+"/v1/operations/execute", bytes.NewBufferString(`{"action":"system.snapshot"}`))
	request.Header.Set("Authorization", "Bearer "+testTCIToken)
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("TCI executed operation: %d", response.StatusCode)
	}

	request, _ = http.NewRequest(http.MethodPost, server.URL+"/v1/confirmations", bytes.NewBufferString(`{"plan_id":"anything"}`))
	request.Header.Set("Authorization", "Bearer "+testTCIToken)
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("TCI reached confirmation minting route: %d", response.StatusCode)
	}
}

func TestAuthenticatedActorOverridesCallerActorField(t *testing.T) {
	dependencies := testSecurityDependencies(t)
	broker := &fakeBroker{}
	server := httptest.NewServer(NewServerWithSecurity(broker, testConfig(""), discardLogger(), dependencies))
	defer server.Close()

	request, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/operations/plan", bytes.NewBufferString(`{"action":"system.snapshot","actor":"human:forged"}`))
	request.Header.Set("Authorization", "Bearer "+testTCIToken)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if broker.last.Actor != "tci:quantum" {
		t.Fatalf("caller actor field was trusted: %q", broker.last.Actor)
	}
}

func testSecurityDependencies(t *testing.T) SecurityDependencies {
	t.Helper()
	dir := t.TempDir()
	audit, err := security.OpenAuditStore(filepath.Join(dir, "audit.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	grants, err := security.OpenGrantStore(filepath.Join(dir, "grants.json"), 2*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	auditor, err := security.NewStaticAuthenticator(
		security.Actor{ID: "human:auditor", Kind: security.ActorHuman, DisplayName: "Auditor", Roles: []string{"auditor"}},
		testAuditorToken,
	)
	if err != nil {
		t.Fatal(err)
	}
	tci, err := security.NewStaticAuthenticator(
		security.Actor{ID: "tci:quantum", Kind: security.ActorTCI, DisplayName: "Quantum TCI", Roles: []string{"tci-proposer"}},
		testTCIToken,
	)
	if err != nil {
		t.Fatal(err)
	}
	return SecurityDependencies{
		Authenticator: security.MultiAuthenticator{auditor, tci},
		Audit:         audit,
		Grants:        grants,
		Plans:         security.NewPlanCache(),
		PlanBuilder:   security.PlanBuilder{TTL: 5 * time.Minute},
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
