package security

import "time"

const ActorSchema = "quantum.control/actors/v1alpha1"
const PlanSchema = "quantum.control/operation-plan/v1alpha1"
const GrantSchema = "quantum.control/confirmation-grant/v1alpha1"
const AuditSchema = "quantum.control/audit-record/v1alpha1"

type ActorKind string

const ActorHuman ActorKind = "human"
const ActorService ActorKind = "service"
const ActorTCI ActorKind = "tci"

type Permission string

const PermissionControlRead Permission = "control.read"
const PermissionInventoryRead Permission = "inventory.read"
const PermissionOperationCatalog Permission = "operations.catalog.read"
const PermissionOperationPlan Permission = "operations.plan"
const PermissionOperationExecute Permission = "operations.execute.readonly"
const PermissionOperationMutate Permission = "operations.execute.mutate"
const PermissionAuditRead Permission = "audit.read"
const PermissionConfirm Permission = "operations.confirm"
const PermissionTCIPropose Permission = "operations.propose"

type Actor struct {
	ID          string       `json:"id"`
	Kind        ActorKind    `json:"kind"`
	DisplayName string       `json:"display_name"`
	Roles       []string     `json:"roles"`
	Permissions []Permission `json:"permissions"`
}

type ActorCredential struct {
	Actor       Actor  `json:"actor"`
	TokenSHA256 string `json:"token_sha256"`
}

type ActorRegistry struct {
	Schema string            `json:"schema"`
	Actors []ActorCredential `json:"actors"`
}

type Parameter struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type OperationPlan struct {
	Schema               string      `json:"schema"`
	ID                   string      `json:"id"`
	Digest               string      `json:"digest"`
	RequestID            string      `json:"request_id"`
	SessionID            string      `json:"session_id"`
	Actor                Actor       `json:"actor"`
	Action               string      `json:"action"`
	Parameters           []Parameter `json:"parameters"`
	Risk                 string      `json:"risk"`
	RequiresConfirmation bool        `json:"requires_confirmation"`
	Valid                bool        `json:"valid"`
	CreatedAt            time.Time   `json:"created_at"`
	ExpiresAt            time.Time   `json:"expires_at"`
	ErrorCode            string      `json:"error_code,omitempty"`
}

type ConfirmationGrant struct {
	Schema         string    `json:"schema"`
	ID             string    `json:"id"`
	PlanID         string    `json:"plan_id"`
	PlanDigest     string    `json:"plan_digest"`
	SubjectActorID string    `json:"subject_actor_id"`
	SessionID      string    `json:"session_id"`
	Approver       Actor     `json:"approver"`
	Action         string    `json:"action"`
	IssuedAt       time.Time `json:"issued_at"`
	ExpiresAt      time.Time `json:"expires_at"`
	UsedAt         time.Time `json:"used_at,omitempty"`
}

type GrantResponse struct {
	Grant ConfirmationGrant `json:"grant"`
	Token string            `json:"token"`
}

type AuditRecord struct {
	Schema         string            `json:"schema"`
	Sequence       uint64            `json:"sequence"`
	ID             string            `json:"id"`
	Timestamp      time.Time         `json:"timestamp"`
	Event          string            `json:"event"`
	Actor          Actor             `json:"actor"`
	RequestID      string            `json:"request_id,omitempty"`
	SessionID      string            `json:"session_id,omitempty"`
	PlanID         string            `json:"plan_id,omitempty"`
	PlanDigest     string            `json:"plan_digest,omitempty"`
	Action         string            `json:"action,omitempty"`
	Risk           string            `json:"risk,omitempty"`
	Status         string            `json:"status"`
	RollbackStatus string            `json:"rollback_status,omitempty"`
	Parameters     map[string]string `json:"parameters,omitempty"`
	ErrorCode      string            `json:"error_code,omitempty"`
	PreviousHash   string            `json:"previous_hash"`
	EntryHash      string            `json:"entry_hash"`
}

type AuditEvent struct {
	Event          string
	Actor          Actor
	RequestID      string
	SessionID      string
	PlanID         string
	PlanDigest     string
	Action         string
	Risk           string
	Status         string
	RollbackStatus string
	Parameters     map[string]string
	ErrorCode      string
}
