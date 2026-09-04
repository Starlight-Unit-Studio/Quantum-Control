package control

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Starlight-Unit-Studio/Quantum-Control/internal/config"
	"github.com/Starlight-Unit-Studio/Quantum-Control/internal/protocol"
)

const testAPIToken = "01234567890123456789012345678901"

type fakeBroker struct {
	healthErr      error
	catalogErr     error
	planErr        error
	executeErr     error
	planValid      *bool
	executeStatus  string
	executeProblem *protocol.Problem
	last           protocol.OperationRequest
}

func (f *fakeBroker) Health(context.Context) error { return f.healthErr }
func (f *fakeBroker) Catalog(context.Context) ([]protocol.OperationDefinition, error) {
	if f.catalogErr != nil { return nil, f.catalogErr }
	return []protocol.OperationDefinition{{Action: "system.snapshot", Risk: protocol.RiskReadOnly, Implemented: true}}, nil
}
func (f *fakeBroker) Plan(_ context.Context, request protocol.OperationRequest) (protocol.OperationPlan, error) {
	f.last = request
	if f.planErr != nil { return protocol.OperationPlan{}, f.planErr }
	valid := true
	if f.planValid != nil { valid = *f.planValid }
	plan := protocol.OperationPlan{Request: request, Definition: protocol.OperationDefinition{Action: request.Action, Risk: protocol.RiskReadOnly, Implemented: true}, Valid: valid}
	if !valid { plan.Error = &protocol.Problem{Code: "unknown_action", Message: "action is not allowlisted"} }
	return plan, nil
}
func (f *fakeBroker) Execute(_ context.Context, request protocol.OperationRequest) (protocol.OperationResponse, error) {
	f.last = request
	if f.executeErr != nil { return protocol.OperationResponse{}, f.executeErr }
	status := f.executeStatus
	if status == "" { status = "completed" }
	return protocol.OperationResponse{RequestID: request.RequestID, Action: request.Action, Status: status, Risk: protocol.RiskReadOnly, StartedAt: time.Now().UTC(), FinishedAt: time.Now().UTC(), AuditID: "audit-1", Result: map[string]any{"hostname": "node-1", "unit": request.Parameters["unit"]}, Error: f.executeProblem}, nil
}

func TestReadyReportsBrokerFailureWithoutLeakingInternalError(t *testing.T) {
	broker := &fakeBroker{healthErr: errors.New("secret socket path /run/private.sock")}
	server := httptest.NewServer(newTestServer(broker, "")); defer server.Close()
	response, err := http.Get(server.URL + "/readyz"); if err != nil { t.Fatal(err) }; defer response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable { t.Fatalf("unexpected status: %d", response.StatusCode) }
	body, _ := io.ReadAll(response.Body); if strings.Contains(string(body), "/run/private.sock") { t.Fatalf("internal broker error leaked: %s", body) }
	if response.Header.Get("X-Quantum-Request-ID") == "" { t.Fatal("missing request ID header") }
}

func TestSystemStatusUsesTypedBrokerAction(t *testing.T) {
	broker := &fakeBroker{}; server := httptest.NewServer(newTestServer(broker, "")); defer server.Close()
	response, err := http.Get(server.URL + "/v1/system/status"); if err != nil { t.Fatal(err) }; defer response.Body.Close()
	if response.StatusCode != http.StatusOK { t.Fatalf("unexpected status: %d", response.StatusCode) }
	var result map[string]any; if err := json.NewDecoder(response.Body).Decode(&result); err != nil { t.Fatal(err) }
	if result["hostname"] != "node-1" || broker.last.Action != "system.snapshot" || broker.last.Actor != "service:loopback-readonly" { t.Fatalf("unexpected result/request: %#v %#v", result, broker.last) }
	if broker.last.RequestID == "" { t.Fatal("system status did not propagate the HTTP request ID") }
}

func TestServiceStatusUsesPathAsTypedParameter(t *testing.T) {
	broker := &fakeBroker{}; server := httptest.NewServer(newTestServer(broker, "")); defer server.Close()
	response, err := http.Get(server.URL + "/v1/services/quantum-runtime.service"); if err != nil { t.Fatal(err) }; response.Body.Close()
	if response.StatusCode != http.StatusOK || broker.last.Parameters["unit"] != "quantum-runtime.service" { t.Fatalf("unexpected status/request: %d %#v", response.StatusCode, broker.last) }
}

func TestControlAuthenticationRequiresBearerScheme(t *testing.T) {
	server := httptest.NewServer(newTestServer(&fakeBroker{}, testAPIToken)); defer server.Close()
	request, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/control/info", nil); request.Header.Set("Authorization", testAPIToken)
	response, err := http.DefaultClient.Do(request); if err != nil { t.Fatal(err) }; response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized { t.Fatalf("raw token without Bearer scheme was accepted: %d", response.StatusCode) }
}

func TestControlAuthenticationAcceptsValidBearerToken(t *testing.T) {
	server := httptest.NewServer(newTestServer(&fakeBroker{}, testAPIToken)); defer server.Close()
	request, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/control/info", nil); request.Header.Set("Authorization", "Bearer "+testAPIToken)
	response, err := http.DefaultClient.Do(request); if err != nil { t.Fatal(err) }; response.Body.Close()
	if response.StatusCode != http.StatusOK { t.Fatalf("valid bearer token was rejected: %d", response.StatusCode) }
}

func TestInvalidPlanReturnsBadRequest(t *testing.T) {
	valid := false; broker := &fakeBroker{planValid: &valid}; server := httptest.NewServer(newTestServer(broker, "")); defer server.Close()
	response, err := http.Post(server.URL+"/v1/operations/plan", "application/json", bytes.NewBufferString(`{"action":"shell.exec"}`)); if err != nil { t.Fatal(err) }; defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest { t.Fatalf("invalid plan returned %d", response.StatusCode) }
}

func TestRejectedExecutionReturnsBadRequest(t *testing.T) {
	broker := &fakeBroker{executeStatus: "rejected", executeProblem: &protocol.Problem{Code: "unknown_action", Message: "action is not allowlisted"}}
	server := httptest.NewServer(newTestServer(broker, "")); defer server.Close()
	response, err := http.Post(server.URL+"/v1/operations/execute", "application/json", bytes.NewBufferString(`{"action":"shell.exec"}`)); if err != nil { t.Fatal(err) }; defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest { t.Fatalf("rejected execution returned %d", response.StatusCode) }
}

func TestOversizedOperationBodyReturnsRequestTooLarge(t *testing.T) {
	cfg := testConfig(""); cfg.RequestBodyLimit = 1024; logger := slog.New(slog.NewTextHandler(io.Discard, nil)); server := httptest.NewServer(NewServer(&fakeBroker{}, cfg, logger).Handler()); defer server.Close()
	body := `{"action":"system.snapshot","confirmation":"` + strings.Repeat("x", 2048) + `"}`
	response, err := http.Post(server.URL+"/v1/operations/plan", "application/json", strings.NewReader(body)); if err != nil { t.Fatal(err) }; response.Body.Close()
	if response.StatusCode != http.StatusRequestEntityTooLarge { t.Fatalf("oversized request returned %d", response.StatusCode) }
}

func newTestServer(client *fakeBroker, token string) http.Handler { logger := slog.New(slog.NewTextHandler(io.Discard, nil)); return NewServer(client, testConfig(token), logger).Handler() }
func testConfig(token string) config.Control { return config.Control{Listen: "127.0.0.1:17440", APIToken: token, AuditPath: "/tmp/quantum-control-audit-test.jsonl", PlanTTL: 5 * time.Minute, BrokerSocket: "/tmp/qcored.sock", BrokerToken: testAPIToken, RequestBodyLimit: 1 << 20, HeaderTimeout: 10 * time.Second, IdleTimeout: 90 * time.Second, BrokerTimeout: 15 * time.Second} }
