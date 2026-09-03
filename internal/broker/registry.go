package broker

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Starlight-Unit-Studio/Quantum-Control/internal/protocol"
	"github.com/Starlight-Unit-Studio/Quantum-Control/internal/systemprobe"
)

var serviceUnitPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.@:-]{0,127}$`)

type operationHandler func(context.Context, protocol.OperationRequest) (map[string]any, error)

type registeredOperation struct {
	definition protocol.OperationDefinition
	handler    operationHandler
}

// Registry owns the explicit operation allowlist. No request can introduce an
// executable name, argument vector or shell fragment.
type Registry struct {
	operations map[string]registeredOperation
}

// NewRegistry creates the alpha read-only operation catalog.
func NewRegistry(probe systemprobe.Probe) *Registry {
	registry := &Registry{operations: make(map[string]registeredOperation)}
	registry.register(protocol.OperationDefinition{
		Action:      "system.snapshot",
		Summary:     "Read a bounded local system summary",
		Risk:        protocol.RiskReadOnly,
		Implemented: true,
	}, func(ctx context.Context, _ protocol.OperationRequest) (map[string]any, error) {
		return probe.Snapshot(ctx)
	})
	registry.register(protocol.OperationDefinition{
		Action:      "service.status",
		Summary:     "Read systemd unit state",
		Risk:        protocol.RiskReadOnly,
		Implemented: true,
		Parameters: []protocol.ParameterDefinition{{
			Name:        "unit",
			Description: "Exact systemd unit name",
			Required:    true,
			Pattern:     serviceUnitPattern.String(),
		}},
	}, func(ctx context.Context, request protocol.OperationRequest) (map[string]any, error) {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		return probe.ServiceStatus(ctx, request.Parameters["unit"])
	})
	return registry
}

func (r *Registry) register(definition protocol.OperationDefinition, handler operationHandler) {
	r.operations[definition.Action] = registeredOperation{definition: definition, handler: handler}
}

// Catalog returns a stable action-sorted copy of the allowlist.
func (r *Registry) Catalog() []protocol.OperationDefinition {
	definitions := make([]protocol.OperationDefinition, 0, len(r.operations))
	for _, operation := range r.operations {
		definitions = append(definitions, operation.definition)
	}
	sort.Slice(definitions, func(i, j int) bool {
		return definitions[i].Action < definitions[j].Action
	})
	return definitions
}

// Plan validates a request without executing it.
func (r *Registry) Plan(request protocol.OperationRequest) protocol.OperationPlan {
	operation, problem := r.validate(request)
	if problem != nil {
		return protocol.OperationPlan{
			Request: request,
			Valid:   false,
			Error:   problem,
		}
	}
	return protocol.OperationPlan{
		Request:              request,
		Definition:           operation.definition,
		Valid:                true,
		RequiresConfirmation: operation.definition.RequiresConfirmation,
	}
}

// Execute validates and runs one registered operation.
func (r *Registry) Execute(ctx context.Context, request protocol.OperationRequest) protocol.OperationResponse {
	started := time.Now().UTC()
	response := protocol.OperationResponse{
		RequestID: request.RequestID,
		Action:    request.Action,
		Status:    "rejected",
		StartedAt: started,
		AuditID:   newID("audit"),
	}
	if response.RequestID == "" {
		response.RequestID = newID("request")
	}

	operation, problem := r.validate(request)
	if problem != nil {
		response.Error = problem
		response.FinishedAt = time.Now().UTC()
		return response
	}
	response.Risk = operation.definition.Risk

	result, err := operation.handler(ctx, request)
	response.FinishedAt = time.Now().UTC()
	if err != nil {
		response.Status = "failed"
		response.Error = &protocol.Problem{Code: "operation_failed", Message: err.Error()}
		return response
	}
	response.Status = "completed"
	response.Result = result
	return response
}

func (r *Registry) validate(request protocol.OperationRequest) (registeredOperation, *protocol.Problem) {
	action := strings.TrimSpace(request.Action)
	if action == "" {
		return registeredOperation{}, &protocol.Problem{Code: "invalid_action", Message: "action is required"}
	}
	operation, ok := r.operations[action]
	if !ok {
		return registeredOperation{}, &protocol.Problem{Code: "unknown_action", Message: "action is not allowlisted"}
	}
	if !operation.definition.Implemented {
		return registeredOperation{}, &protocol.Problem{Code: "not_implemented", Message: "action is not implemented"}
	}
	if operation.definition.RequiresConfirmation && strings.TrimSpace(request.Confirmation) == "" {
		return registeredOperation{}, &protocol.Problem{Code: "confirmation_required", Message: "operation requires explicit confirmation"}
	}

	allowed := make(map[string]protocol.ParameterDefinition, len(operation.definition.Parameters))
	for _, parameter := range operation.definition.Parameters {
		allowed[parameter.Name] = parameter
		value, exists := request.Parameters[parameter.Name]
		if parameter.Required && (!exists || strings.TrimSpace(value) == "") {
			return registeredOperation{}, &protocol.Problem{
				Code:    "invalid_parameters",
				Message: fmt.Sprintf("parameter %q is required", parameter.Name),
			}
		}
		if exists {
			if problem := validateParameter(parameter, value); problem != nil {
				return registeredOperation{}, problem
			}
		}
	}
	for name := range request.Parameters {
		if _, ok := allowed[name]; !ok {
			return registeredOperation{}, &protocol.Problem{
				Code:    "invalid_parameters",
				Message: fmt.Sprintf("parameter %q is not accepted", name),
			}
		}
	}
	return operation, nil
}

func validateParameter(definition protocol.ParameterDefinition, value string) *protocol.Problem {
	if definition.Pattern != "" {
		pattern, err := regexp.Compile(definition.Pattern)
		if err != nil {
			return &protocol.Problem{Code: "policy_error", Message: "operation parameter policy is invalid"}
		}
		if !pattern.MatchString(value) {
			return &protocol.Problem{
				Code:    "invalid_parameters",
				Message: fmt.Sprintf("parameter %q does not match policy", definition.Name),
			}
		}
	}
	if len(definition.AllowedValues) > 0 {
		for _, allowed := range definition.AllowedValues {
			if value == allowed {
				return nil
			}
		}
		return &protocol.Problem{
			Code:    "invalid_parameters",
			Message: fmt.Sprintf("parameter %q is not an allowed value", definition.Name),
		}
	}
	return nil
}

func newID(prefix string) string {
	var value [12]byte
	if _, err := rand.Read(value[:]); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(value[:])
}
