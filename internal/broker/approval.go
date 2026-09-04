package broker

import (
	"context"
	"errors"
	"strings"

	"github.com/Starlight-Unit-Studio/Quantum-Control/internal/protocol"
	"github.com/Starlight-Unit-Studio/Quantum-Control/internal/security"
)

const maxForwardedActorTokenBytes = 4096

// ApprovalAPI is the mutation extension implemented by the real broker client.
// Keeping it separate from API preserves the existing read-only broker contract
// for integrations and test doubles.
type ApprovalAPI interface {
	Confirm(context.Context, security.OperationPlan, string) (security.GrantResponse, error)
	ExecuteApproved(context.Context, security.OperationPlan, string, string) (protocol.OperationResponse, error)
}

type confirmationEnvelope struct {
	Plan       security.OperationPlan `json:"plan"`
	ActorToken string                 `json:"actor_token"`
}

type approvedExecutionEnvelope struct {
	Plan              security.OperationPlan `json:"plan"`
	ConfirmationToken string                 `json:"confirmation_token"`
	ActorToken        string                 `json:"actor_token"`
}

// SecurityBoundary is root-owned qcored state. The unprivileged public service
// cannot create or consume grants directly.
type SecurityBoundary struct {
	Actors security.Authenticator
	Grants *security.GrantStore
}

func (b SecurityBoundary) authenticateActor(token string) (security.Actor, error) {
	if b.Actors == nil {
		return security.Actor{}, errors.New("broker actor authentication is not configured")
	}
	token = strings.TrimSpace(token)
	if token == "" || len(token) > maxForwardedActorTokenBytes {
		return security.Actor{}, errors.New("valid actor credential required")
	}
	actor, ok := b.Actors.AuthenticateBearer(token)
	if !ok {
		return security.Actor{}, errors.New("valid actor credential required")
	}
	return actor, nil
}
