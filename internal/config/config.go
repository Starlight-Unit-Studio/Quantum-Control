package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultControlListen      = "127.0.0.1:17440"
	defaultBrokerSocket       = "/run/quantum-control/qcored.sock"
	defaultTokenFile          = "/etc/quantum-control/broker.token"
	defaultAuditPath          = "/var/lib/quantum-control/audit/audit.jsonl"
	defaultBrokerGrantPath    = "/var/lib/quantum-control-broker/grants.json"
	defaultBodyLimit          = int64(1 << 20)
	defaultTransactionTimeout = 30 * time.Second
	defaultServicePoll        = 250 * time.Millisecond
)

// Control configures the unprivileged web/API process.
type Control struct {
	Listen           string
	APIToken         string
	ActorFile        string
	AuditPath        string
	PlanTTL          time.Duration
	BrokerSocket     string
	BrokerToken      string
	RequestBodyLimit int64
	HeaderTimeout    time.Duration
	IdleTimeout      time.Duration
	BrokerTimeout    time.Duration
}

// Broker configures qcored, the privileged typed-operation broker.
type Broker struct {
	SocketPath          string
	BrokerToken         string
	ActorFile           string
	GrantPath           string
	GrantTTL            time.Duration
	ServicePolicyFile   string
	TransactionTimeout  time.Duration
	ServicePollInterval time.Duration
	RequestBodyLimit    int64
	HeaderTimeout       time.Duration
	IdleTimeout         time.Duration
}

func LoadControl() (Control, error) {
	brokerToken, err := loadBrokerToken()
	if err != nil {
		return Control{}, err
	}
	bodyLimit, err := envInt64("QUANTUM_CONTROL_REQUEST_BODY_LIMIT", defaultBodyLimit)
	if err != nil {
		return Control{}, err
	}
	headerTimeout, err := envDuration("QUANTUM_CONTROL_HEADER_TIMEOUT", 10*time.Second)
	if err != nil {
		return Control{}, err
	}
	idleTimeout, err := envDuration("QUANTUM_CONTROL_IDLE_TIMEOUT", 90*time.Second)
	if err != nil {
		return Control{}, err
	}
	brokerTimeout, err := envDuration("QUANTUM_CONTROL_BROKER_TIMEOUT", 15*time.Second)
	if err != nil {
		return Control{}, err
	}
	planTTL, err := envDuration("QUANTUM_CONTROL_PLAN_TTL", 5*time.Minute)
	if err != nil {
		return Control{}, err
	}

	cfg := Control{
		Listen:           envOr("QUANTUM_CONTROL_LISTEN", defaultControlListen),
		APIToken:         strings.TrimSpace(os.Getenv("QUANTUM_CONTROL_API_TOKEN")),
		ActorFile:        strings.TrimSpace(os.Getenv("QUANTUM_CONTROL_ACTOR_FILE")),
		AuditPath:        envOr("QUANTUM_CONTROL_AUDIT_PATH", defaultAuditPath),
		PlanTTL:          planTTL,
		BrokerSocket:     envOr("QUANTUM_CONTROL_BROKER_SOCKET", defaultBrokerSocket),
		BrokerToken:      brokerToken,
		RequestBodyLimit: bodyLimit,
		HeaderTimeout:    headerTimeout,
		IdleTimeout:      idleTimeout,
		BrokerTimeout:    brokerTimeout,
	}
	if err := cfg.Validate(); err != nil {
		return Control{}, err
	}
	return cfg, nil
}

func LoadBroker() (Broker, error) {
	brokerToken, err := loadBrokerToken()
	if err != nil {
		return Broker{}, err
	}
	bodyLimit, err := envInt64("QUANTUM_CONTROL_BROKER_REQUEST_BODY_LIMIT", defaultBodyLimit)
	if err != nil {
		return Broker{}, err
	}
	headerTimeout, err := envDuration("QUANTUM_CONTROL_BROKER_HEADER_TIMEOUT", 10*time.Second)
	if err != nil {
		return Broker{}, err
	}
	idleTimeout, err := envDuration("QUANTUM_CONTROL_BROKER_IDLE_TIMEOUT", 30*time.Second)
	if err != nil {
		return Broker{}, err
	}
	grantTTL, err := envDuration("QUANTUM_CONTROL_GRANT_TTL", 2*time.Minute)
	if err != nil {
		return Broker{}, err
	}
	transactionTimeout, err := envDuration("QUANTUM_CONTROL_TRANSACTION_TIMEOUT", defaultTransactionTimeout)
	if err != nil {
		return Broker{}, err
	}
	pollInterval, err := envDuration("QUANTUM_CONTROL_SERVICE_POLL_INTERVAL", defaultServicePoll)
	if err != nil {
		return Broker{}, err
	}

	cfg := Broker{
		SocketPath:          envOr("QUANTUM_CONTROL_BROKER_SOCKET", defaultBrokerSocket),
		BrokerToken:         brokerToken,
		ActorFile:           strings.TrimSpace(os.Getenv("QUANTUM_CONTROL_ACTOR_FILE")),
		GrantPath:           envOr("QUANTUM_CONTROL_GRANT_PATH", defaultBrokerGrantPath),
		GrantTTL:            grantTTL,
		ServicePolicyFile:   strings.TrimSpace(os.Getenv("QUANTUM_CONTROL_SERVICE_POLICY_FILE")),
		TransactionTimeout:  transactionTimeout,
		ServicePollInterval: pollInterval,
		RequestBodyLimit:    bodyLimit,
		HeaderTimeout:       headerTimeout,
		IdleTimeout:         idleTimeout,
	}
	if err := cfg.Validate(); err != nil {
		return Broker{}, err
	}
	return cfg, nil
}

func (c Control) Validate() error {
	if _, _, err := net.SplitHostPort(c.Listen); err != nil {
		return fmt.Errorf("invalid QUANTUM_CONTROL_LISTEN: %w", err)
	}
	if !filepath.IsAbs(c.BrokerSocket) {
		return errors.New("QUANTUM_CONTROL_BROKER_SOCKET must be absolute")
	}
	if len(c.BrokerToken) < 32 {
		return errors.New("broker token must contain at least 32 characters")
	}
	if c.APIToken != "" && len(c.APIToken) < 32 {
		return errors.New("QUANTUM_CONTROL_API_TOKEN must contain at least 32 characters when configured")
	}
	if c.ActorFile != "" && !filepath.IsAbs(c.ActorFile) {
		return errors.New("QUANTUM_CONTROL_ACTOR_FILE must be absolute when configured")
	}
	if !filepath.IsAbs(c.AuditPath) {
		return errors.New("QUANTUM_CONTROL_AUDIT_PATH must be absolute")
	}
	if c.PlanTTL <= 0 || c.PlanTTL > 15*time.Minute {
		return errors.New("QUANTUM_CONTROL_PLAN_TTL must be greater than zero and at most 15 minutes")
	}
	if c.RequestBodyLimit < 1024 {
		return errors.New("QUANTUM_CONTROL_REQUEST_BODY_LIMIT must be at least 1024 bytes")
	}
	if c.HeaderTimeout <= 0 || c.IdleTimeout <= 0 || c.BrokerTimeout <= 0 {
		return errors.New("control timeouts must be greater than zero")
	}
	if !isLoopbackListen(c.Listen) && c.APIToken == "" && c.ActorFile == "" {
		return errors.New("non-loopback listen address requires QUANTUM_CONTROL_API_TOKEN or QUANTUM_CONTROL_ACTOR_FILE")
	}
	return nil
}

func (c Broker) Validate() error {
	if !filepath.IsAbs(c.SocketPath) {
		return errors.New("QUANTUM_CONTROL_BROKER_SOCKET must be absolute")
	}
	if len(c.BrokerToken) < 32 {
		return errors.New("broker token must contain at least 32 characters")
	}
	if c.ActorFile != "" && !filepath.IsAbs(c.ActorFile) {
		return errors.New("QUANTUM_CONTROL_ACTOR_FILE must be absolute when configured for qcored")
	}
	if !filepath.IsAbs(c.GrantPath) {
		return errors.New("QUANTUM_CONTROL_GRANT_PATH must be absolute")
	}
	if c.GrantTTL <= 0 || c.GrantTTL > 15*time.Minute {
		return errors.New("QUANTUM_CONTROL_GRANT_TTL must be greater than zero and at most 15 minutes")
	}
	if c.ServicePolicyFile != "" && !filepath.IsAbs(c.ServicePolicyFile) {
		return errors.New("QUANTUM_CONTROL_SERVICE_POLICY_FILE must be absolute when configured")
	}
	if c.TransactionTimeout < time.Second || c.TransactionTimeout > 2*time.Minute {
		return errors.New("QUANTUM_CONTROL_TRANSACTION_TIMEOUT must be between 1 second and 2 minutes")
	}
	if c.ServicePollInterval < 10*time.Millisecond || c.ServicePollInterval > 5*time.Second {
		return errors.New("QUANTUM_CONTROL_SERVICE_POLL_INTERVAL must be between 10 milliseconds and 5 seconds")
	}
	if c.RequestBodyLimit < 1024 {
		return errors.New("broker request body limit must be at least 1024 bytes")
	}
	if c.HeaderTimeout <= 0 || c.IdleTimeout <= 0 {
		return errors.New("broker timeouts must be greater than zero")
	}
	return nil
}

func loadBrokerToken() (string, error) {
	if token := strings.TrimSpace(os.Getenv("QUANTUM_CONTROL_BROKER_TOKEN")); token != "" {
		if len(token) < 32 {
			return "", errors.New("broker token must contain at least 32 characters")
		}
		return token, nil
	}
	path := envOr("QUANTUM_CONTROL_BROKER_TOKEN_FILE", defaultTokenFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read broker token file %s: %w", path, err)
	}
	token := strings.TrimSpace(string(data))
	if len(token) < 32 {
		return "", errors.New("broker token must contain at least 32 characters")
	}
	return token, nil
}

func isLoopbackListen(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envInt64(key string, fallback int64) (int64, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	return parsed, nil
}

func envDuration(key string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	return parsed, nil
}
