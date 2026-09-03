package security

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Starlight-Unit-Studio/Quantum-Control/internal/protocol"
)

func TestActorRegistryAuthenticatesHumanAndRejectsTCIElevation(t *testing.T) {
	dir := t.TempDir()
	token := "human-token-that-is-long-enough-for-test"
	digest := sha256.Sum256([]byte(token))
	registry := ActorRegistry{
		Schema: ActorSchema,
		Actors: []ActorCredential{{
			Actor: Actor{ID: "human:rick", Kind: ActorHuman, DisplayName: "Rick", Roles: []string{"approver"}},
			TokenSHA256: hex.EncodeToString(digest[:]),
		}},
	}
	data, _ := json.Marshal(registry)
	path := filepath.Join(dir, "actors.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	authenticator, err := LoadActorRegistry(path)
	if err != nil {
		t.Fatal(err)
	}
	actor, ok := authenticator.AuthenticateBearer(token)
	if !ok || actor.ID != "human:rick" || !HasPermission(actor, PermissionConfirm) {
		t.Fatalf("unexpected authenticated actor: %#v", actor)
	}

	registry.Actors[0].Actor = Actor{ID: "tci:quantum", Kind: ActorTCI, DisplayName: "Quantum TCI", Roles: []string{"approver"}}
	data, _ = json.Marshal(registry)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadActorRegistry(path); err == nil {
		t.Fatal("TCI actor received an elevated approver role")
	}
}

func TestPlanDigestBindsActorAndExactParameters(t *testing.T) {
	actor := mustActor(t, Actor{ID: "service:coreui", Kind: ActorService, DisplayName: "CoreUI", Roles: []string{"service"}})
	builder := PlanBuilder{TTL: 5 * time.Minute, Now: func() time.Time { return time.Unix(100, 0) }}
	base := protocol.OperationPlan{
		Request: protocol.OperationRequest{RequestID: "req-1", Action: "service.status", Parameters: map[string]string{"unit": "ollama.service", "scope": "local"}},
		Definition: protocol.OperationDefinition{Action: "service.status", Risk: protocol.RiskReadOnly},
		Valid: true,
	}
	planA, err := builder.Build(actor, "session-1", base)
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPlanDigest(planA) {
		t.Fatal("fresh plan digest did not verify")
	}

	planB := clonePlan(planA)
	planB.Parameters[0], planB.Parameters[1] = planB.Parameters[1], planB.Parameters[0]
	if !VerifyPlanDigest(planB) {
		t.Fatal("parameter ordering changed canonical digest")
	}

	planB = clonePlan(planA)
	planB.Parameters[0].Value += "-changed"
	if VerifyPlanDigest(planB) {
		t.Fatal("modified parameter retained valid digest")
	}

	planB = clonePlan(planA)
	planB.Actor.ID = "service:other"
	if VerifyPlanDigest(planB) {
		t.Fatal("modified actor retained valid digest")
	}
}

func TestConfirmationGrantCannotReplayAndSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "grants.json")
	store, err := OpenGrantStore(path, 2*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(200, 0).UTC()
	store.now = func() time.Time { return now }
	actor := mustActor(t, Actor{ID: "service:coreui", Kind: ActorService, DisplayName: "CoreUI", Roles: []string{"service"}})
	approver := mustActor(t, Actor{ID: "human:rick", Kind: ActorHuman, DisplayName: "Rick", Roles: []string{"approver"}})
	plan := testConfirmPlan(t, actor, now)

	issued, err := store.Issue(plan, approver)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(issued.Token) == "" {
		t.Fatal("grant token missing")
	}
	if _, err := store.Consume(issued.Token, plan, actor.ID, "different.action"); err == nil {
		t.Fatal("grant was accepted for a different action")
	}
	if _, err := store.Consume(issued.Token, plan, actor.ID, plan.Action); err != nil {
		t.Fatalf("consume valid grant: %v", err)
	}
	if _, err := store.Consume(issued.Token, plan, actor.ID, plan.Action); err == nil {
		t.Fatal("grant replay succeeded")
	}

	reopened, err := OpenGrantStore(path, 2*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	reopened.now = func() time.Time { return now.Add(time.Second) }
	if _, err := reopened.Consume(issued.Token, plan, actor.ID, plan.Action); err == nil {
		t.Fatal("grant replay succeeded after process-style reopen")
	}
}

func TestConfirmationGrantExpiresAndTCICannotApprove(t *testing.T) {
	path := filepath.Join(t.TempDir(), "grants.json")
	store, err := OpenGrantStore(path, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(300, 0).UTC()
	store.now = func() time.Time { return now }
	actor := mustActor(t, Actor{ID: "service:worker", Kind: ActorService, DisplayName: "Worker", Roles: []string{"service"}})
	approver := mustActor(t, Actor{ID: "human:approver", Kind: ActorHuman, DisplayName: "Approver", Roles: []string{"approver"}})
	plan := testConfirmPlan(t, actor, now)
	issued, err := store.Issue(plan, approver)
	if err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return now.Add(2 * time.Minute) }
	if _, err := store.Consume(issued.Token, plan, actor.ID, plan.Action); err == nil {
		t.Fatal("expired grant was accepted")
	}

	tci := mustActor(t, Actor{ID: "tci:quantum", Kind: ActorTCI, DisplayName: "Quantum TCI", Roles: []string{"tci-proposer"}})
	store.now = func() time.Time { return now }
	if _, err := store.Issue(plan, tci); err == nil {
		t.Fatal("TCI minted its own confirmation")
	}
}

func TestAuditStoreSurvivesRestartRedactsSecretsAndDetectsTampering(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	store, err := OpenAuditStore(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(400, 0).UTC()
	store.now = func() time.Time { return now }
	actor := mustActor(t, Actor{ID: "human:auditor", Kind: ActorHuman, DisplayName: "Auditor", Roles: []string{"auditor"}})
	record, err := store.Append(AuditEvent{
		Event: "operation.failed", Actor: actor, RequestID: "req-1", Action: "demo",
		Status: "failed", Parameters: map[string]string{"unit": "demo.service", "api_token": "TOP-SECRET"},
		ErrorCode: "operation_failed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if record.Parameters["api_token"] != "[REDACTED]" || strings.Contains(mustJSON(t, record), "TOP-SECRET") {
		t.Fatalf("secret was not redacted: %#v", record)
	}
	if record.ErrorCode != "operation_failed" {
		t.Fatalf("stable error code missing: %#v", record)
	}

	reopened, err := OpenAuditStore(path)
	if err != nil {
		t.Fatal(err)
	}
	records := reopened.Query(10, actor.ID, "demo")
	if len(records) != 1 || reopened.Integrity()["verified"] != true {
		t.Fatalf("durable audit query failed: %#v", records)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(string(data), "demo.service", "tampered.service", 1))
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenAuditStore(path); err == nil {
		t.Fatal("tampered audit chain was accepted")
	}
}

func testConfirmPlan(t *testing.T, actor Actor, now time.Time) OperationPlan {
	t.Helper()
	plan := OperationPlan{
		Schema: PlanSchema, ID: "plan-test", RequestID: "req-test", SessionID: "session-test",
		Actor: actor, Action: "service.restart", Parameters: []Parameter{{Name: "unit", Value: "demo.service"}},
		Risk: "low", RequiresConfirmation: true, Valid: true, CreatedAt: now, ExpiresAt: now.Add(5 * time.Minute),
	}
	digest, err := PlanDigest(plan)
	if err != nil {
		t.Fatal(err)
	}
	plan.Digest = digest
	return plan
}

func mustActor(t *testing.T, actor Actor) Actor {
	t.Helper()
	if err := normalizeActor(&actor); err != nil {
		t.Fatal(err)
	}
	return actor
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
