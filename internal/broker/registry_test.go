package broker

import (
	"context"
	"testing"

	"github.com/Starlight-Unit-Studio/Quantum-Control/internal/protocol"
)

type fakeProbe struct {
	lastUnit string
}

func (p *fakeProbe) Snapshot(context.Context) (map[string]any, error) {
	return map[string]any{"hostname": "test-node"}, nil
}

func (p *fakeProbe) ServiceStatus(_ context.Context, unit string) (map[string]any, error) {
	p.lastUnit = unit
	return map[string]any{"unit": unit, "active_state": "active"}, nil
}

func TestRegistryRejectsUnknownAction(t *testing.T) {
	registry := NewRegistry(&fakeProbe{})
	response := registry.Execute(context.Background(), protocol.OperationRequest{Action: "shell.exec"})
	if response.Status != "rejected" || response.Error == nil || response.Error.Code != "unknown_action" {
		t.Fatalf("unexpected response: %#v", response)
	}
}

func TestRegistryRejectsCommandLikeUnit(t *testing.T) {
	registry := NewRegistry(&fakeProbe{})
	response := registry.Execute(context.Background(), protocol.OperationRequest{
		Action:     "service.status",
		Parameters: map[string]string{"unit": "--help"},
	})
	if response.Status != "rejected" || response.Error == nil || response.Error.Code != "invalid_parameters" {
		t.Fatalf("unexpected response: %#v", response)
	}
}

func TestRegistryExecutesAllowlistedStatus(t *testing.T) {
	probe := &fakeProbe{}
	registry := NewRegistry(probe)
	response := registry.Execute(context.Background(), protocol.OperationRequest{
		RequestID:  "request-1",
		Action:     "service.status",
		Actor:      "test",
		Parameters: map[string]string{"unit": "quantum-runtime.service"},
	})
	if response.Status != "completed" || probe.lastUnit != "quantum-runtime.service" {
		t.Fatalf("unexpected response/probe: %#v %q", response, probe.lastUnit)
	}
}

func TestPlanRejectsUnexpectedParameter(t *testing.T) {
	registry := NewRegistry(&fakeProbe{})
	plan := registry.Plan(protocol.OperationRequest{
		Action:     "system.snapshot",
		Parameters: map[string]string{"command": "id"},
	})
	if plan.Valid || plan.Error == nil {
		t.Fatalf("unexpected plan: %#v", plan)
	}
}
