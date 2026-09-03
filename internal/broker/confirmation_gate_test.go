package broker

import (
	"context"
	"testing"

	"github.com/Starlight-Unit-Studio/Quantum-Control/internal/protocol"
)

func TestConfirmationRequiredOperationCanPlanButCannotExecuteWithArbitraryString(t *testing.T) {
	registry := NewRegistry(&fakeProbe{})
	registry.register(protocol.OperationDefinition{
		Action:               "test.confirmed",
		Summary:              "test-only confirmation boundary",
		Risk:                 protocol.RiskMedium,
		RequiresConfirmation: true,
		Implemented:          true,
	}, func(context.Context, protocol.OperationRequest) (map[string]any, error) {
		return map[string]any{"unexpected": true}, nil
	})

	plan := registry.Plan(protocol.OperationRequest{Action: "test.confirmed"})
	if !plan.Valid || !plan.RequiresConfirmation {
		t.Fatalf("confirmation-required operation could not be reviewed: %#v", plan)
	}

	response := registry.Execute(context.Background(), protocol.OperationRequest{
		Action:       "test.confirmed",
		Confirmation: "attacker-controlled-string",
	})
	if response.Status != "rejected" || response.Error == nil || response.Error.Code != "confirmation_verifier_required" {
		t.Fatalf("arbitrary confirmation string crossed broker gate: %#v", response)
	}
}
