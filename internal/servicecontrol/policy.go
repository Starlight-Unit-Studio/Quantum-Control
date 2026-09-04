package servicecontrol

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
)

const PolicySchema = "quantum.control/service-mutation-policy/v1alpha1"

type ServiceSpec struct {
	Unit      string
	HealthURL string
}

var compiledServices = map[string]ServiceSpec{
	"quantum-runtime.service": {
		Unit:      "quantum-runtime.service",
		HealthURL: "http://127.0.0.1:11450/healthz",
	},
}

type policyFile struct {
	Schema       string   `json:"schema"`
	AllowedUnits []string `json:"allowed_units"`
}

// Policy can only narrow the compiled service set. A deployment file can
// disable services, but it cannot introduce a new privileged target.
type Policy struct {
	allowed map[string]ServiceSpec
}

func DefaultPolicy() Policy {
	allowed := make(map[string]ServiceSpec, len(compiledServices))
	for unit, spec := range compiledServices {
		allowed[unit] = spec
	}
	return Policy{allowed: allowed}
}

func LoadPolicy(path string) (Policy, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return DefaultPolicy(), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Policy{}, fmt.Errorf("read service mutation policy: %w", err)
	}
	if len(data) > 1<<20 {
		return Policy{}, errors.New("service mutation policy exceeds 1 MiB")
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var document policyFile
	if err := decoder.Decode(&document); err != nil {
		return Policy{}, fmt.Errorf("decode service mutation policy: %w", err)
	}
	if document.Schema != PolicySchema {
		return Policy{}, fmt.Errorf("unsupported service mutation policy schema %q", document.Schema)
	}
	allowed := make(map[string]ServiceSpec, len(document.AllowedUnits))
	for _, raw := range document.AllowedUnits {
		unit := strings.TrimSpace(raw)
		if unit == "" {
			return Policy{}, errors.New("service mutation policy contains an empty unit")
		}
		spec, ok := compiledServices[unit]
		if !ok {
			return Policy{}, fmt.Errorf("service mutation policy may not broaden compiled allowlist with %q", unit)
		}
		allowed[unit] = spec
	}
	return Policy{allowed: allowed}, nil
}

func (p Policy) Allows(unit string) bool {
	_, ok := p.allowed[unit]
	return ok
}

func (p Policy) Spec(unit string) (ServiceSpec, bool) {
	spec, ok := p.allowed[unit]
	return spec, ok
}

func (p Policy) AllowedUnits() []string {
	units := make([]string, 0, len(p.allowed))
	for unit := range p.allowed {
		units = append(units, unit)
	}
	sort.Strings(units)
	return units
}
