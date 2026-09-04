package servicecontrol

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

var unitPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.@:-]{0,127}$`)

// Mutator exposes only the three typed service lifecycle actions supported by
// the first Quantum Control mutation milestone. It never accepts command text.
type Mutator interface {
	Start(context.Context, string) error
	Stop(context.Context, string) error
	Restart(context.Context, string) error
}

// Runner exists so the fixed systemctl argument vectors can be verified in
// tests without invoking the host service manager.
type Runner interface {
	Run(context.Context, string, ...string) error
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, binary string, args ...string) error {
	command := exec.CommandContext(ctx, binary, args...)
	if output, err := command.CombinedOutput(); err != nil {
		message := strings.TrimSpace(string(output))
		if len(message) > 256 {
			message = message[:256]
		}
		if message == "" {
			return fmt.Errorf("systemctl action failed: %w", err)
		}
		return fmt.Errorf("systemctl action failed: %s", message)
	}
	return nil
}

// Native executes systemctl directly with a fixed argv vector. Binary and
// Runner are injectable only for tests; production uses exec.LookPath and
// exec.CommandContext.
type Native struct {
	Binary string
	Runner Runner
}

func (n Native) Start(ctx context.Context, unit string) error {
	return n.run(ctx, "start", unit)
}

func (n Native) Stop(ctx context.Context, unit string) error {
	return n.run(ctx, "stop", unit)
}

func (n Native) Restart(ctx context.Context, unit string) error {
	return n.run(ctx, "restart", unit)
}

func (n Native) run(ctx context.Context, verb, unit string) error {
	if err := validateUnit(unit); err != nil {
		return err
	}
	switch verb {
	case "start", "stop", "restart":
	default:
		return errors.New("unsupported service action")
	}
	binary := strings.TrimSpace(n.Binary)
	if binary == "" {
		path, err := exec.LookPath("systemctl")
		if err != nil {
			return errors.New("systemctl is not available")
		}
		binary = path
	}
	runner := n.Runner
	if runner == nil {
		runner = execRunner{}
	}
	// The separator is fixed and the unit is validated independently. No shell
	// parser or request-derived executable/option can enter this vector.
	return runner.Run(ctx, binary, verb, "--", unit)
}

func validateUnit(unit string) error {
	if !unitPattern.MatchString(unit) {
		return errors.New("systemd unit does not match policy")
	}
	return nil
}
