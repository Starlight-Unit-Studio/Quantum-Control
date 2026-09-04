package broker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Starlight-Unit-Studio/Quantum-Control/internal/protocol"
	"github.com/Starlight-Unit-Studio/Quantum-Control/internal/security"
	"github.com/Starlight-Unit-Studio/Quantum-Control/internal/servicecontrol"
)

type serviceMutator interface {
	Start(context.Context, string) error
	Stop(context.Context, string) error
	Restart(context.Context, string) error
}

type healthChecker interface {
	Check(context.Context, string) error
}

type serviceTarget struct {
	healthURL string
}

type operationError struct {
	code    string
	message string
}

func (e operationError) Error() string { return e.message }

func newOperationError(code, message string) error {
	return operationError{code: code, message: message}
}

func problemFromOperationError(err error) *protocol.Problem {
	var typed operationError
	if errors.As(err, &typed) {
		return &protocol.Problem{Code: typed.code, Message: typed.message}
	}
	return &protocol.Problem{Code: "operation_failed", Message: err.Error()}
}

// EnableServiceMutations adds the first privileged operation set. The supplied
// deployment policy can only narrow the compile-time service allowlist.
func (r *Registry) EnableServiceMutations(
	mutator serviceMutator,
	health healthChecker,
	policy servicecontrol.Policy,
	transactionTimeout time.Duration,
	pollInterval time.Duration,
) error {
	if mutator == nil {
		return errors.New("service mutator is required")
	}
	if health == nil {
		return errors.New("service health checker is required")
	}
	if transactionTimeout < time.Second || transactionTimeout > 2*time.Minute {
		return errors.New("service transaction timeout must be between 1 second and 2 minutes")
	}
	if pollInterval < 10*time.Millisecond || pollInterval > 5*time.Second {
		return errors.New("service poll interval must be between 10 milliseconds and 5 seconds")
	}

	r.mutator = mutator
	r.health = health
	r.transactionTimeout = transactionTimeout
	r.pollInterval = pollInterval
	r.serviceTargets = make(map[string]serviceTarget)
	units := policy.AllowedUnits()
	for _, unit := range units {
		spec, ok := policy.Spec(unit)
		if !ok {
			return fmt.Errorf("service policy lost target %q", unit)
		}
		r.serviceTargets[unit] = serviceTarget{healthURL: spec.HealthURL}
	}

	register := func(action, summary string, risk protocol.Risk) {
		r.register(protocol.OperationDefinition{
			Action:               action,
			Summary:              summary,
			Risk:                 risk,
			RequiresConfirmation: true,
			Implemented:          true,
			Parameters: []protocol.ParameterDefinition{{
				Name:          "unit",
				Description:   "Exact service unit from the compiled mutation allowlist",
				Required:      true,
				AllowedValues: append([]string{}, units...),
			}},
		}, func(ctx context.Context, request protocol.OperationRequest) (map[string]any, error) {
			return r.runServiceTransaction(ctx, action, request.Parameters["unit"])
		})
	}
	register("service.start", "Start an allowlisted managed service", protocol.RiskLow)
	register("service.stop", "Stop an allowlisted managed service", protocol.RiskHigh)
	register("service.restart", "Restart an allowlisted managed service", protocol.RiskLow)
	return nil
}

// ValidateApprovedPlan replays qcored policy over the immutable public plan.
// A valid digest alone is insufficient: action metadata and the current service
// policy must still match the broker catalog at execution time.
func (r *Registry) ValidateApprovedPlan(plan security.OperationPlan) *protocol.Problem {
	now := r.now().UTC()
	if plan.Schema != security.PlanSchema || !plan.Valid || !plan.RequiresConfirmation {
		return &protocol.Problem{Code: "invalid_plan", Message: "operation plan is not executable"}
	}
	if !security.VerifyPlanDigest(plan) {
		return &protocol.Problem{Code: "invalid_plan_digest", Message: "operation plan digest verification failed"}
	}
	if strings.TrimSpace(plan.ID) == "" || strings.TrimSpace(plan.Actor.ID) == "" || strings.TrimSpace(plan.SessionID) == "" {
		return &protocol.Problem{Code: "invalid_plan", Message: "operation plan identity or correlation data is missing"}
	}
	if plan.CreatedAt.IsZero() || plan.ExpiresAt.IsZero() || !plan.ExpiresAt.After(plan.CreatedAt) {
		return &protocol.Problem{Code: "invalid_plan_time", Message: "operation plan time bounds are invalid"}
	}
	if plan.CreatedAt.After(now.Add(30*time.Second)) || !plan.ExpiresAt.After(now) {
		return &protocol.Problem{Code: "expired_plan", Message: "operation plan is stale or expired"}
	}
	if plan.ExpiresAt.Sub(plan.CreatedAt) > 15*time.Minute {
		return &protocol.Problem{Code: "invalid_plan_time", Message: "operation plan lifetime exceeds policy"}
	}

	parameters, problem := exactPlanParameters(plan)
	if problem != nil {
		return problem
	}
	request := protocol.OperationRequest{
		RequestID:  plan.RequestID,
		Action:     plan.Action,
		Actor:      plan.Actor.ID,
		Parameters: parameters,
	}
	operation, problem := r.validate(request, false)
	if problem != nil {
		return problem
	}
	if !operation.definition.RequiresConfirmation || string(operation.definition.Risk) != plan.Risk {
		return &protocol.Problem{Code: "stale_plan_policy", Message: "operation policy changed after the plan was created"}
	}
	return nil
}

func exactPlanParameters(plan security.OperationPlan) (map[string]string, *protocol.Problem) {
	result := make(map[string]string, len(plan.Parameters))
	for _, parameter := range plan.Parameters {
		name := strings.TrimSpace(parameter.Name)
		if name == "" {
			return nil, &protocol.Problem{Code: "invalid_plan", Message: "operation plan contains an empty parameter name"}
		}
		if _, exists := result[name]; exists {
			return nil, &protocol.Problem{Code: "invalid_plan", Message: "operation plan contains duplicate parameters"}
		}
		result[name] = parameter.Value
	}
	return result, nil
}

// ExecuteApproved runs a plan only after the server has independently
// authenticated an executor and atomically consumed the matching grant.
func (r *Registry) ExecuteApproved(ctx context.Context, plan security.OperationPlan, executor security.Actor) protocol.OperationResponse {
	started := time.Now().UTC()
	response := protocol.OperationResponse{
		RequestID: plan.RequestID,
		Action:    plan.Action,
		Status:    "rejected",
		StartedAt: started,
		AuditID:   newID("audit"),
	}
	if response.RequestID == "" {
		response.RequestID = newID("request")
	}
	if executor.Kind == security.ActorTCI || !security.HasPermission(executor, security.PermissionOperationMutate) {
		response.Error = &protocol.Problem{Code: "mutation_forbidden", Message: "executor is not authorized for service mutations"}
		response.FinishedAt = time.Now().UTC()
		return response
	}
	if problem := r.ValidateApprovedPlan(plan); problem != nil {
		response.Error = problem
		response.FinishedAt = time.Now().UTC()
		return response
	}

	parameters, _ := exactPlanParameters(plan)
	request := protocol.OperationRequest{
		RequestID:  plan.RequestID,
		Action:     plan.Action,
		Actor:      plan.Actor.ID,
		Parameters: parameters,
	}
	operation := r.operations[plan.Action]
	response.Risk = operation.definition.Risk
	result, err := operation.handler(ctx, request)
	if result == nil {
		result = map[string]any{}
	}
	result["executor_actor_id"] = executor.ID
	result["plan_actor_id"] = plan.Actor.ID
	result["plan_id"] = plan.ID
	result["plan_digest"] = plan.Digest
	response.Result = result
	response.FinishedAt = time.Now().UTC()
	if err != nil {
		response.Status = "failed"
		response.Error = problemFromOperationError(err)
		return response
	}
	response.Status = "completed"
	return response
}

func (r *Registry) runServiceTransaction(ctx context.Context, action, unit string) (map[string]any, error) {
	target, ok := r.serviceTargets[unit]
	if !ok {
		return nil, newOperationError("service_not_allowlisted", "service is not in the mutation allowlist")
	}
	txCtx, cancel := context.WithTimeout(ctx, r.transactionTimeout)
	defer cancel()

	precondition, err := r.probe.ServiceStatus(txCtx, unit)
	if err != nil {
		return nil, newOperationError("service_precondition_failed", "service precondition could not be observed")
	}
	preActive := stateValue(precondition, "active_state")
	result := map[string]any{
		"action":          action,
		"unit":            unit,
		"precondition":    precondition,
		"recovery_status": "not_required",
	}
	if target.healthURL != "" && preActive == "active" {
		if err := r.health.Check(txCtx, target.healthURL); err == nil {
			result["pre_health"] = "healthy"
		} else {
			result["pre_health"] = "unhealthy"
		}
	}

	if err := r.applyServiceAction(txCtx, action, unit); err != nil {
		postcondition, _ := r.probe.ServiceStatus(txCtx, unit)
		result["postcondition"] = postcondition
		result["recovery_status"] = r.recoverPrecondition(txCtx, unit, preActive)
		return result, newOperationError("service_action_failed", "service action failed before the expected postcondition was reached")
	}

	expected := expectedActiveState(action)
	postcondition, err := r.waitForServicePostcondition(txCtx, unit, target.healthURL, expected)
	result["postcondition"] = postcondition
	if err != nil {
		result["recovery_status"] = r.recoverPrecondition(txCtx, unit, preActive)
		return result, newOperationError("service_postcondition_failed", "service did not reach the required postcondition before timeout")
	}
	result["postcondition_verified"] = true
	if expected == "active" && target.healthURL != "" {
		result["health_verified"] = true
	}
	return result, nil
}

func (r *Registry) applyServiceAction(ctx context.Context, action, unit string) error {
	switch action {
	case "service.start":
		return r.mutator.Start(ctx, unit)
	case "service.stop":
		return r.mutator.Stop(ctx, unit)
	case "service.restart":
		return r.mutator.Restart(ctx, unit)
	default:
		return errors.New("unsupported service mutation")
	}
}

func expectedActiveState(action string) string {
	if action == "service.stop" {
		return "inactive"
	}
	return "active"
}

func (r *Registry) waitForServicePostcondition(ctx context.Context, unit, healthURL, expected string) (map[string]any, error) {
	ticker := time.NewTicker(r.pollInterval)
	defer ticker.Stop()
	var last map[string]any
	for {
		state, err := r.probe.ServiceStatus(ctx, unit)
		if err == nil {
			last = state
			if stateValue(state, "active_state") == expected {
				if expected != "active" || healthURL == "" || r.health.Check(ctx, healthURL) == nil {
					return state, nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return last, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (r *Registry) recoverPrecondition(ctx context.Context, unit, preActive string) string {
	if ctx.Err() != nil {
		return "failed"
	}
	var action string
	switch preActive {
	case "active":
		action = "service.start"
	case "inactive", "failed":
		action = "service.stop"
	default:
		return "not_defined"
	}
	if err := r.applyServiceAction(ctx, action, unit); err != nil {
		return "failed"
	}
	if _, err := r.waitForServicePostcondition(ctx, unit, "", expectedActiveState(action)); err != nil {
		return "failed"
	}
	return "succeeded"
}

func stateValue(state map[string]any, key string) string {
	value, _ := state[key].(string)
	return strings.ToLower(strings.TrimSpace(value))
}
