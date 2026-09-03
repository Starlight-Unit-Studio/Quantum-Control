package config

import (
	"strings"
	"testing"
	"time"
)

const testToken = "01234567890123456789012345678901"

func TestControlRejectsUnauthenticatedRemoteBind(t *testing.T) {
	cfg := validControl()
	cfg.Listen = "0.0.0.0:17440"
	cfg.APIToken = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() accepted unauthenticated remote bind")
	}
}

func TestControlAllowsRemoteBindWithStrongToken(t *testing.T) {
	cfg := validControl()
	cfg.Listen = "0.0.0.0:17440"
	cfg.APIToken = testToken
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() returned error: %v", err)
	}
}

func TestControlRejectsShortAPIToken(t *testing.T) {
	cfg := validControl()
	cfg.APIToken = "short"
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() accepted a short API token")
	}
}

func TestBrokerRequiresAbsoluteSocket(t *testing.T) {
	cfg := validBroker()
	cfg.SocketPath = "qcored.sock"
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() accepted relative socket path")
	}
}

func TestLoadControlRejectsInvalidNumericConfiguration(t *testing.T) {
	t.Setenv("QUANTUM_CONTROL_BROKER_TOKEN", testToken)
	t.Setenv("QUANTUM_CONTROL_REQUEST_BODY_LIMIT", "not-a-number")
	_, err := LoadControl()
	if err == nil || !strings.Contains(err.Error(), "QUANTUM_CONTROL_REQUEST_BODY_LIMIT") {
		t.Fatalf("expected numeric parse error, got %v", err)
	}
}

func TestLoadControlRejectsInvalidDurationConfiguration(t *testing.T) {
	t.Setenv("QUANTUM_CONTROL_BROKER_TOKEN", testToken)
	t.Setenv("QUANTUM_CONTROL_BROKER_TIMEOUT", "not-a-duration")
	_, err := LoadControl()
	if err == nil || !strings.Contains(err.Error(), "QUANTUM_CONTROL_BROKER_TIMEOUT") {
		t.Fatalf("expected duration parse error, got %v", err)
	}
}

func TestLoadBrokerRejectsShortEnvironmentToken(t *testing.T) {
	t.Setenv("QUANTUM_CONTROL_BROKER_TOKEN", "short")
	_, err := LoadBroker()
	if err == nil || !strings.Contains(err.Error(), "at least 32") {
		t.Fatalf("expected short token error, got %v", err)
	}
}

func validControl() Control {
	return Control{
		Listen:           "127.0.0.1:17440",
		BrokerSocket:     "/run/quantum-control/qcored.sock",
		BrokerToken:      testToken,
		RequestBodyLimit: defaultBodyLimit,
		HeaderTimeout:    10 * time.Second,
		IdleTimeout:      90 * time.Second,
		BrokerTimeout:    15 * time.Second,
	}
}

func validBroker() Broker {
	return Broker{
		SocketPath:       "/run/quantum-control/qcored.sock",
		BrokerToken:      testToken,
		RequestBodyLimit: defaultBodyLimit,
		HeaderTimeout:    10 * time.Second,
		IdleTimeout:      30 * time.Second,
	}
}
