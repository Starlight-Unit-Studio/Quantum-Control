package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Starlight-Unit-Studio/Quantum-Control/internal/protocol"
)

type PlanBuilder struct {
	TTL time.Duration
	Now func() time.Time
}

func (b PlanBuilder) Build(actor Actor, sessionID string, brokerPlan protocol.OperationPlan) (OperationPlan, error) {
	if b.TTL <= 0 {
		b.TTL = 5 * time.Minute
	}
	if b.Now == nil {
		b.Now = time.Now
	}
	if strings.TrimSpace(actor.ID) == "" {
		return OperationPlan{}, errors.New("plan actor is required")
	}
	created := b.Now().UTC()
	parameters := make([]Parameter, 0, len(brokerPlan.Request.Parameters))
	for name, value := range brokerPlan.Request.Parameters {
		parameters = append(parameters, Parameter{Name: name, Value: value})
	}
	sort.Slice(parameters, func(i, j int) bool { return parameters[i].Name < parameters[j].Name })
	plan := OperationPlan{
		Schema:               PlanSchema,
		ID:                   newSecurityID("plan"),
		RequestID:            brokerPlan.Request.RequestID,
		SessionID:            strings.TrimSpace(sessionID),
		Actor:                cloneActor(actor),
		Action:               strings.TrimSpace(brokerPlan.Request.Action),
		Parameters:           parameters,
		Risk:                 string(brokerPlan.Definition.Risk),
		RequiresConfirmation: brokerPlan.RequiresConfirmation,
		Valid:                brokerPlan.Valid,
		CreatedAt:            created,
		ExpiresAt:            created.Add(b.TTL),
	}
	if brokerPlan.Error != nil {
		plan.ErrorCode = brokerPlan.Error.Code
	}
	digest, err := PlanDigest(plan)
	if err != nil {
		return OperationPlan{}, err
	}
	plan.Digest = digest
	return plan, nil
}

func PlanDigest(plan OperationPlan) (string, error) {
	canonical := struct {
		Schema               string      `json:"schema"`
		ID                   string      `json:"id"`
		RequestID            string      `json:"request_id"`
		SessionID            string      `json:"session_id"`
		ActorID              string      `json:"actor_id"`
		ActorKind            ActorKind   `json:"actor_kind"`
		Action               string      `json:"action"`
		Parameters           []Parameter `json:"parameters"`
		Risk                 string      `json:"risk"`
		RequiresConfirmation bool        `json:"requires_confirmation"`
		Valid                bool        `json:"valid"`
		CreatedAt            string      `json:"created_at"`
		ExpiresAt            string      `json:"expires_at"`
		ErrorCode            string      `json:"error_code,omitempty"`
	}{
		Schema:               plan.Schema,
		ID:                   plan.ID,
		RequestID:            plan.RequestID,
		SessionID:            plan.SessionID,
		ActorID:              plan.Actor.ID,
		ActorKind:            plan.Actor.Kind,
		Action:               plan.Action,
		Parameters:           append([]Parameter{}, plan.Parameters...),
		Risk:                 plan.Risk,
		RequiresConfirmation: plan.RequiresConfirmation,
		Valid:                plan.Valid,
		CreatedAt:            plan.CreatedAt.UTC().Format(time.RFC3339Nano),
		ExpiresAt:            plan.ExpiresAt.UTC().Format(time.RFC3339Nano),
		ErrorCode:            plan.ErrorCode,
	}
	sort.Slice(canonical.Parameters, func(i, j int) bool { return canonical.Parameters[i].Name < canonical.Parameters[j].Name })
	data, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("canonicalize plan: %w", err)
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func VerifyPlanDigest(plan OperationPlan) bool {
	expected, err := PlanDigest(plan)
	if err != nil || len(expected) != len(plan.Digest) {
		return false
	}
	return strings.EqualFold(expected, plan.Digest)
}

func PlanParametersMap(plan OperationPlan) map[string]string {
	result := make(map[string]string, len(plan.Parameters))
	for _, parameter := range plan.Parameters {
		result[parameter.Name] = parameter.Value
	}
	return result
}

type PlanCache struct {
	mu    sync.Mutex
	plans map[string]OperationPlan
	now   func() time.Time
}

func NewPlanCache() *PlanCache {
	return &PlanCache{plans: make(map[string]OperationPlan), now: time.Now}
}

func (c *PlanCache) Put(plan OperationPlan) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.purgeExpiredLocked()
	c.plans[plan.ID] = clonePlan(plan)
}

func (c *PlanCache) Get(id string) (OperationPlan, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.purgeExpiredLocked()
	plan, ok := c.plans[id]
	if !ok {
		return OperationPlan{}, false
	}
	return clonePlan(plan), true
}

func (c *PlanCache) purgeExpiredLocked() {
	now := c.now().UTC()
	for id, plan := range c.plans {
		if !plan.ExpiresAt.After(now) {
			delete(c.plans, id)
		}
	}
}

func clonePlan(plan OperationPlan) OperationPlan {
	plan.Actor = cloneActor(plan.Actor)
	plan.Parameters = append([]Parameter{}, plan.Parameters...)
	return plan
}

func newSecurityID(prefix string) string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(value[:])
}
