package servicecontrol

import (
	"context"
	"reflect"
	"testing"
)

type recordingRunner struct {
	binary string
	args   []string
}

func (r *recordingRunner) Run(_ context.Context, binary string, args ...string) error {
	r.binary = binary
	r.args = append([]string{}, args...)
	return nil
}

func TestNativeRestartUsesFixedArgumentVector(t *testing.T) {
	runner := &recordingRunner{}
	mutator := Native{Binary: "/usr/bin/systemctl", Runner: runner}
	if err := mutator.Restart(context.Background(), "quantum-runtime.service"); err != nil {
		t.Fatal(err)
	}
	if runner.binary != "/usr/bin/systemctl" {
		t.Fatalf("unexpected binary: %q", runner.binary)
	}
	want := []string{"restart", "--", "quantum-runtime.service"}
	if !reflect.DeepEqual(runner.args, want) {
		t.Fatalf("unexpected argv: %#v", runner.args)
	}
}

func TestNativeRejectsCommandLikeUnit(t *testing.T) {
	for _, unit := range []string{"--now", "quantum-runtime.service --no-block", "", " runtime.service"} {
		runner := &recordingRunner{}
		if err := (Native{Binary: "/usr/bin/systemctl", Runner: runner}).Restart(context.Background(), unit); err == nil {
			t.Fatalf("accepted unsafe unit %q", unit)
		}
		if len(runner.args) != 0 {
			t.Fatalf("runner invoked for unsafe unit %q", unit)
		}
	}
}

func TestDefaultPolicyContainsOnlyQuantumRuntime(t *testing.T) {
	policy := DefaultPolicy()
	units := policy.AllowedUnits()
	if !reflect.DeepEqual(units, []string{"quantum-runtime.service"}) {
		t.Fatalf("unexpected compiled mutation allowlist: %#v", units)
	}
	for _, unit := range []string{"quantum-control.service", "ollama.service", "apache2.service"} {
		if policy.Allows(unit) {
			t.Fatalf("default policy unexpectedly allows %q", unit)
		}
	}
}
