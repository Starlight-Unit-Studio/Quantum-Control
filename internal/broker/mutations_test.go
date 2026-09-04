package broker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Starlight-Unit-Studio/Quantum-Control/internal/protocol"
	"github.com/Starlight-Unit-Studio/Quantum-Control/internal/security"
	"github.com/Starlight-Unit-Studio/Quantum-Control/internal/servicecontrol"
)

const (
	brokerTestToken   = "broker-token-012345678901234567890123456789"
	mutatorTestToken  = "mutator-token-01234567890123456789012345678"
	approverTestToken = "approver-token-0123456789012345678901234567"
	tciBrokerToken    = "tci-token-012345678901234567890123456789012"
)

type mutableProbe struct { mu sync.Mutex; state string }
func (p *mutableProbe) Snapshot(context.Context) (map[string]any, error) { return map[string]any{"hostname":"mutation-test"}, nil }
func (p *mutableProbe) ServiceStatus(_ context.Context, unit string) (map[string]any, error) { p.mu.Lock(); defer p.mu.Unlock(); return map[string]any{"unit":unit,"load_state":"loaded","active_state":p.state,"sub_state":map[string]string{"active":"running","inactive":"dead"}[p.state]}, nil }
func (p *mutableProbe) set(state string) { p.mu.Lock(); p.state = state; p.mu.Unlock() }

type fakeServiceMutator struct { probe *mutableProbe; calls []string; failRestart bool }
func (m *fakeServiceMutator) Start(_ context.Context, unit string) error { m.calls=append(m.calls,"start "+unit); m.probe.set("active"); return nil }
func (m *fakeServiceMutator) Stop(_ context.Context, unit string) error { m.calls=append(m.calls,"stop "+unit); m.probe.set("inactive"); return nil }
func (m *fakeServiceMutator) Restart(_ context.Context, unit string) error { m.calls=append(m.calls,"restart "+unit); if m.failRestart { m.failRestart=false; return errors.New("injected restart failure") }; m.probe.set("active"); return nil }
type healthyService struct{}
func (healthyService) Check(context.Context,string) error { return nil }

func TestMutationCatalogOnlyAllowsCompiledQuantumRuntimeUnit(t *testing.T) {
	probe:=&mutableProbe{state:"active"}; registry:=NewRegistry(probe)
	if err:=registry.EnableServiceMutations(&fakeServiceMutator{probe:probe},healthyService{},servicecontrol.DefaultPolicy(),time.Second,10*time.Millisecond); err!=nil { t.Fatal(err) }
	plan:=registry.Plan(protocol.OperationRequest{Action:"service.restart",Parameters:map[string]string{"unit":"apache2.service"}})
	if plan.Valid || plan.Error==nil || plan.Error.Code!="invalid_parameters" { t.Fatalf("arbitrary syntactically valid unit was accepted: %#v",plan) }
	plan=registry.Plan(protocol.OperationRequest{Action:"service.restart",Parameters:map[string]string{"unit":"quantum-runtime.service"}})
	if !plan.Valid || !plan.RequiresConfirmation || plan.Definition.Risk!=protocol.RiskLow { t.Fatalf("runtime restart plan policy mismatch: %#v",plan) }
	response:=registry.Execute(context.Background(),protocol.OperationRequest{Action:"service.restart",Parameters:map[string]string{"unit":"quantum-runtime.service"},Confirmation:"caller-controlled-string"})
	if response.Status!="rejected" || response.Error==nil || response.Error.Code!="confirmation_verifier_required" { t.Fatalf("legacy execution bypassed structured confirmation gate: %#v",response) }
}

func TestBrokerApprovedRestartConsumesGrantAndRejectsReplay(t *testing.T) {
	server,registry,boundary,mutator,actors:=newMutationBrokerServer(t,false); defer server.Close()
	plan:=buildMutationPlan(t,registry,actors.mutator,"session-one","service.restart","quantum-runtime.service")
	grant:=confirmPlan(t,server.URL,plan,approverTestToken)
	response:=executeApproved(t,server.URL,plan,grant.Token,mutatorTestToken)
	if response.Status!="completed" || len(mutator.calls)!=1 || mutator.calls[0]!="restart quantum-runtime.service" { t.Fatalf("approved restart failed: response=%#v calls=%#v",response,mutator.calls) }
	if response.Result["health_verified"]!=true || response.Result["recovery_status"]!="not_required" { t.Fatalf("postcondition metadata missing: %#v",response.Result) }
	replay:=executeApproved(t,server.URL,plan,grant.Token,mutatorTestToken)
	if replay.Status!="rejected" || replay.Error==nil || replay.Error.Code!="confirmation_rejected" { t.Fatalf("grant replay succeeded: %#v",replay) }
	if _,err:=security.OpenGrantStore(boundary.path,2*time.Minute); err!=nil { t.Fatalf("durable consumed grant could not be reopened: %v",err) }
}

func TestApprovedMutationBindsSessionActorActionAndParameters(t *testing.T) {
	server,registry,_,_,actors:=newMutationBrokerServer(t,false); defer server.Close()
	plan:=buildMutationPlan(t,registry,actors.mutator,"session-bind","service.restart","quantum-runtime.service")
	grant:=confirmPlan(t,server.URL,plan,approverTestToken)
	changedSession:=plan; changedSession.SessionID="session-other"; changedSession.Digest=mustPlanDigest(t,changedSession)
	response:=executeApproved(t,server.URL,changedSession,grant.Token,mutatorTestToken)
	if response.Status!="rejected" || response.Error==nil || response.Error.Code!="confirmation_rejected" { t.Fatalf("session-modified plan crossed grant boundary: %#v",response) }
	changedActor:=plan; changedActor.Actor=actors.tci; changedActor.Digest=mustPlanDigest(t,changedActor)
	response=executeApproved(t,server.URL,changedActor,grant.Token,mutatorTestToken)
	if response.Status!="rejected" || response.Error==nil || response.Error.Code!="confirmation_rejected" { t.Fatalf("actor-modified plan crossed grant boundary: %#v",response) }
	changedParameters:=plan; changedParameters.Parameters=[]security.Parameter{{Name:"unit",Value:"apache2.service"}}; changedParameters.Digest=mustPlanDigest(t,changedParameters)
	response=executeApproved(t,server.URL,changedParameters,grant.Token,mutatorTestToken)
	if response.Status!="rejected" || response.Error==nil || response.Error.Code!="invalid_parameters" { t.Fatalf("parameter-modified plan crossed broker policy: %#v",response) }
	changedAction:=plan; changedAction.Action="service.start"; changedAction.Digest=mustPlanDigest(t,changedAction)
	response=executeApproved(t,server.URL,changedAction,grant.Token,mutatorTestToken)
	if response.Status!="rejected" || response.Error==nil || response.Error.Code!="confirmation_rejected" { t.Fatalf("action-modified plan crossed grant boundary: %#v",response) }
	response=executeApproved(t,server.URL,plan,grant.Token,tciBrokerToken)
	if response.Status!="rejected" || response.Error==nil || response.Error.Code!="mutation_forbidden" { t.Fatalf("TCI executed approved mutation: %#v",response) }
	response=executeApproved(t,server.URL,plan,grant.Token,mutatorTestToken)
	if response.Status!="completed" { t.Fatalf("valid original plan/grant did not survive rejected tamper attempts: %#v",response) }
}

func TestExpiredPlanFailsBeforeGrantConsumption(t *testing.T) {
	server,registry,_,_,actors:=newMutationBrokerServer(t,false); defer server.Close()
	plan:=buildMutationPlan(t,registry,actors.mutator,"session-expire","service.restart","quantum-runtime.service"); grant:=confirmPlan(t,server.URL,plan,approverTestToken)
	expired:=plan; expired.CreatedAt=time.Now().Add(-10*time.Minute).UTC(); expired.ExpiresAt=time.Now().Add(-5*time.Minute).UTC(); expired.Digest=mustPlanDigest(t,expired)
	response:=executeApproved(t,server.URL,expired,grant.Token,mutatorTestToken)
	if response.Status!="rejected" || response.Error==nil || response.Error.Code!="expired_plan" { t.Fatalf("expired plan was not rejected: %#v",response) }
	response=executeApproved(t,server.URL,plan,grant.Token,mutatorTestToken)
	if response.Status!="completed" { t.Fatalf("expired tamper attempt consumed valid grant: %#v",response) }
}

func TestFailedRestartAttemptsDefinedRecoveryOnce(t *testing.T) {
	server,registry,_,mutator,actors:=newMutationBrokerServer(t,true); defer server.Close()
	plan:=buildMutationPlan(t,registry,actors.mutator,"session-recover","service.restart","quantum-runtime.service"); grant:=confirmPlan(t,server.URL,plan,approverTestToken)
	response:=executeApproved(t,server.URL,plan,grant.Token,mutatorTestToken)
	if response.Status!="failed" || response.Error==nil || response.Error.Code!="service_action_failed" { t.Fatalf("injected failure was not reported: %#v",response) }
	if response.Result["recovery_status"]!="succeeded" { t.Fatalf("recovery was not recorded: %#v",response.Result) }
	if len(mutator.calls)!=2 || mutator.calls[0]!="restart quantum-runtime.service" || mutator.calls[1]!="start quantum-runtime.service" { t.Fatalf("unexpected retry/recovery behavior: %#v",mutator.calls) }
}

type mutationActors struct { mutator security.Actor; approver security.Actor; tci security.Actor }
type testGrantBoundary struct { path string }

func newMutationBrokerServer(t *testing.T, failRestart bool) (*httptest.Server,*Registry,testGrantBoundary,*fakeServiceMutator,mutationActors) {
	t.Helper(); probe:=&mutableProbe{state:"active"}; mutator:=&fakeServiceMutator{probe:probe,failRestart:failRestart}; registry:=NewRegistry(probe)
	if err:=registry.EnableServiceMutations(mutator,healthyService{},servicecontrol.DefaultPolicy(),time.Second,10*time.Millisecond); err!=nil { t.Fatal(err) }
	mutatorAuth,mutatorActor:=testAuthenticator(t,security.Actor{ID:"service:mutation-executor",Kind:security.ActorService,DisplayName:"Mutation executor",Roles:[]string{"mutator"}},mutatorTestToken)
	approverAuth,approverActor:=testAuthenticator(t,security.Actor{ID:"human:rick",Kind:security.ActorHuman,DisplayName:"Rick",Roles:[]string{"approver"}},approverTestToken)
	tciAuth,tciActor:=testAuthenticator(t,security.Actor{ID:"tci:quantum",Kind:security.ActorTCI,DisplayName:"Quantum TCI",Roles:[]string{"tci-proposer"}},tciBrokerToken)
	grantPath:=filepath.Join(t.TempDir(),"grants.json"); grants,err:=security.OpenGrantStore(grantPath,2*time.Minute); if err!=nil { t.Fatal(err) }
	boundary:=SecurityBoundary{Actors:security.MultiAuthenticator{mutatorAuth,approverAuth,tciAuth},Grants:grants}; logger:=slog.New(slog.NewTextHandler(io.Discard,nil)); server:=httptest.NewServer(NewServerWithSecurity(registry,brokerTestToken,1<<20,logger,boundary).Handler())
	return server,registry,testGrantBoundary{path:grantPath},mutator,mutationActors{mutator:mutatorActor,approver:approverActor,tci:tciActor}
}

func testAuthenticator(t *testing.T, actor security.Actor, token string) (security.Authenticator,security.Actor) { t.Helper(); auth,err:=security.NewStaticAuthenticator(actor,token); if err!=nil { t.Fatal(err) }; normalized,ok:=auth.AuthenticateBearer(token); if !ok { t.Fatal("test actor authentication failed") }; return auth,normalized }
func buildMutationPlan(t *testing.T, registry *Registry, actor security.Actor, session,action,unit string) security.OperationPlan { t.Helper(); brokerPlan:=registry.Plan(protocol.OperationRequest{RequestID:"request-test",Actor:actor.ID,Action:action,Parameters:map[string]string{"unit":unit}}); if !brokerPlan.Valid { t.Fatalf("broker plan invalid: %#v",brokerPlan) }; plan,err:=(security.PlanBuilder{TTL:5*time.Minute}).Build(actor,session,brokerPlan); if err!=nil { t.Fatal(err) }; return plan }
func confirmPlan(t *testing.T, baseURL string, plan security.OperationPlan, actorToken string) security.GrantResponse { t.Helper(); payload,_:=json.Marshal(confirmationEnvelope{Plan:plan,ActorToken:actorToken}); request,_:=http.NewRequest(http.MethodPost,baseURL+"/v1/confirm",bytes.NewReader(payload)); request.Header.Set("Content-Type","application/json"); request.Header.Set("X-Quantum-Broker-Token",brokerTestToken); response,err:=http.DefaultClient.Do(request); if err!=nil { t.Fatal(err) }; defer response.Body.Close(); if response.StatusCode!=http.StatusCreated { body,_:=io.ReadAll(response.Body); t.Fatalf("confirm returned %d: %s",response.StatusCode,body) }; var grant security.GrantResponse; if err:=json.NewDecoder(response.Body).Decode(&grant); err!=nil { t.Fatal(err) }; return grant }
func executeApproved(t *testing.T, baseURL string, plan security.OperationPlan, grantToken,actorToken string) protocol.OperationResponse { t.Helper(); payload,_:=json.Marshal(approvedExecutionEnvelope{Plan:plan,ConfirmationToken:grantToken,ActorToken:actorToken}); request,_:=http.NewRequest(http.MethodPost,baseURL+"/v1/execute-approved",bytes.NewReader(payload)); request.Header.Set("Content-Type","application/json"); request.Header.Set("X-Quantum-Broker-Token",brokerTestToken); response,err:=http.DefaultClient.Do(request); if err!=nil { t.Fatal(err) }; defer response.Body.Close(); var result protocol.OperationResponse; if err:=json.NewDecoder(response.Body).Decode(&result); err!=nil { t.Fatal(err) }; return result }
func mustPlanDigest(t *testing.T, plan security.OperationPlan) string { t.Helper(); digest,err:=security.PlanDigest(plan); if err!=nil { t.Fatal(err) }; return digest }
