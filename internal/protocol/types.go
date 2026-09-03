package protocol

import "time"

// Risk expresses the authorization and review class of an operation.
type Risk string

const (
	RiskReadOnly    Risk = "read-only"
	RiskLow         Risk = "low"
	RiskHigh        Risk = "high"
	RiskDestructive Risk = "destructive"
)

// ParameterDefinition describes one typed operation input.
type ParameterDefinition struct {
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Required      bool     `json:"required"`
	Pattern       string   `json:"pattern,omitempty"`
	AllowedValues []string `json:"allowed_values,omitempty"`
}

// OperationDefinition is safe metadata published by qcored.
type OperationDefinition struct {
	Action               string                `json:"action"`
	Summary              string                `json:"summary"`
	Risk                 Risk                  `json:"risk"`
	RequiresConfirmation bool                  `json:"requires_confirmation"`
	Implemented          bool                  `json:"implemented"`
	Parameters           []ParameterDefinition `json:"parameters,omitempty"`
}

// OperationRequest is the only input accepted by the privileged broker. It
// contains a typed action and string parameters, never a command line.
type OperationRequest struct {
	RequestID    string            `json:"request_id"`
	Action       string            `json:"action"`
	Actor        string            `json:"actor"`
	Parameters   map[string]string `json:"parameters,omitempty"`
	Confirmation string            `json:"confirmation,omitempty"`
}

// OperationResponse contains the audited result of a broker operation.
type OperationResponse struct {
	RequestID  string         `json:"request_id"`
	Action     string         `json:"action"`
	Status     string         `json:"status"`
	Risk       Risk           `json:"risk"`
	StartedAt  time.Time      `json:"started_at"`
	FinishedAt time.Time      `json:"finished_at"`
	AuditID    string         `json:"audit_id"`
	Result     map[string]any `json:"result,omitempty"`
	Error      *Problem       `json:"error,omitempty"`
}

// Problem is the stable error shape used by both services.
type Problem struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// OperationPlan is returned before a future mutating operation is authorized.
type OperationPlan struct {
	Request              OperationRequest    `json:"request"`
	Definition           OperationDefinition `json:"definition"`
	Valid                bool                `json:"valid"`
	RequiresConfirmation bool                `json:"requires_confirmation"`
	Warnings             []string            `json:"warnings,omitempty"`
	Error                *Problem            `json:"error,omitempty"`
}
