package security

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const grantStoreSchema = "quantum.control/confirmation-store/v1alpha1"

type storedGrant struct {
	Grant       ConfirmationGrant `json:"grant"`
	TokenSHA256 string            `json:"token_sha256"`
}

type grantState struct {
	Schema string        `json:"schema"`
	Grants []storedGrant `json:"grants"`
}

type GrantStore struct {
	mu   sync.Mutex
	path string
	ttl  time.Duration
	now  func() time.Time
	data grantState
}

func OpenGrantStore(path string, ttl time.Duration) (*GrantStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("grant store path is required")
	}
	if ttl <= 0 || ttl > 15*time.Minute {
		return nil, errors.New("confirmation grant TTL must be greater than zero and at most 15 minutes")
	}
	store := &GrantStore{
		path: path,
		ttl:  ttl,
		now:  time.Now,
		data: grantState{Schema: grantStoreSchema, Grants: []storedGrant{}},
	}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *GrantStore) Issue(plan OperationPlan, approver Actor) (GrantResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now().UTC()
	if plan.Schema != PlanSchema || !plan.Valid || !plan.RequiresConfirmation {
		return GrantResponse{}, errors.New("plan does not require confirmation")
	}
	if !VerifyPlanDigest(plan) {
		return GrantResponse{}, errors.New("plan digest verification failed")
	}
	if !plan.ExpiresAt.After(now) {
		return GrantResponse{}, errors.New("plan has expired")
	}
	if strings.TrimSpace(plan.SessionID) == "" {
		return GrantResponse{}, errors.New("plan session is required for confirmation")
	}
	if approver.Kind != ActorHuman || !HasPermission(approver, PermissionConfirm) {
		return GrantResponse{}, errors.New("only an authenticated human approver may issue confirmation grants")
	}
	if plan.Actor.ID == approver.ID {
		return GrantResponse{}, errors.New("v1alpha1 requires a distinct human approver")
	}
	for _, existing := range s.data.Grants {
		if existing.Grant.PlanID == plan.ID && strings.EqualFold(existing.Grant.PlanDigest, plan.Digest) {
			return GrantResponse{}, errors.New("a confirmation grant already exists for this plan")
		}
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return GrantResponse{}, fmt.Errorf("generate confirmation token: %w", err)
	}
	token := hex.EncodeToString(raw)
	tokenDigest := sha256.Sum256([]byte(token))
	expires := now.Add(s.ttl)
	if plan.ExpiresAt.Before(expires) {
		expires = plan.ExpiresAt
	}
	grant := ConfirmationGrant{
		Schema:         GrantSchema,
		ID:             newSecurityID("grant"),
		PlanID:         plan.ID,
		PlanDigest:     plan.Digest,
		SubjectActorID: plan.Actor.ID,
		SessionID:      plan.SessionID,
		Approver:       cloneActor(approver),
		Action:         plan.Action,
		IssuedAt:       now,
		ExpiresAt:      expires,
	}
	s.data.Grants = append(s.data.Grants, storedGrant{Grant: grant, TokenSHA256: hex.EncodeToString(tokenDigest[:])})
	if err := s.persistLocked(); err != nil {
		s.data.Grants = s.data.Grants[:len(s.data.Grants)-1]
		return GrantResponse{}, err
	}
	return GrantResponse{Grant: grant, Token: token}, nil
}

func (s *GrantStore) Consume(token string, plan OperationPlan, subjectActorID, action string) (ConfirmationGrant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if strings.TrimSpace(token) == "" {
		return ConfirmationGrant{}, errors.New("confirmation token is required")
	}
	if plan.Schema != PlanSchema || !plan.Valid || !plan.RequiresConfirmation {
		return ConfirmationGrant{}, errors.New("operation plan is not confirmation-executable")
	}
	if !VerifyPlanDigest(plan) {
		return ConfirmationGrant{}, errors.New("plan digest verification failed")
	}
	now := s.now().UTC()
	if !plan.ExpiresAt.After(now) {
		return ConfirmationGrant{}, errors.New("operation plan has expired")
	}
	provided := sha256.Sum256([]byte(token))
	for index := range s.data.Grants {
		stored := &s.data.Grants[index]
		expected, err := decodeSHA256(stored.TokenSHA256)
		if err != nil || subtle.ConstantTimeCompare(provided[:], expected) != 1 {
			continue
		}
		grant := stored.Grant
		if !grant.UsedAt.IsZero() {
			return ConfirmationGrant{}, errors.New("confirmation grant has already been consumed")
		}
		if !grant.ExpiresAt.After(now) {
			return ConfirmationGrant{}, errors.New("confirmation grant has expired")
		}
		if grant.PlanID != plan.ID ||
			!strings.EqualFold(grant.PlanDigest, plan.Digest) ||
			grant.SubjectActorID != subjectActorID ||
			grant.SessionID != plan.SessionID ||
			grant.Action != action {
			return ConfirmationGrant{}, errors.New("confirmation grant is not bound to this operation")
		}
		stored.Grant.UsedAt = now
		if err := s.persistLocked(); err != nil {
			stored.Grant.UsedAt = time.Time{}
			return ConfirmationGrant{}, err
		}
		return cloneGrant(stored.Grant), nil
	}
	return ConfirmationGrant{}, errors.New("confirmation grant not found")
}

func (s *GrantStore) load() error {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read confirmation store: %w", err)
	}
	if len(data) > 8<<20 {
		return errors.New("confirmation store exceeds 8 MiB")
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var state grantState
	if err := decoder.Decode(&state); err != nil {
		return fmt.Errorf("decode confirmation store: %w", err)
	}
	if state.Schema != grantStoreSchema {
		return fmt.Errorf("unsupported confirmation store schema %q", state.Schema)
	}
	seen := make(map[string]struct{}, len(state.Grants))
	for _, stored := range state.Grants {
		if stored.Grant.Schema != GrantSchema ||
			stored.Grant.ID == "" ||
			stored.Grant.PlanID == "" ||
			stored.Grant.PlanDigest == "" ||
			stored.Grant.SubjectActorID == "" ||
			stored.Grant.SessionID == "" ||
			stored.Grant.Action == "" {
			return errors.New("confirmation store contains an invalid grant")
		}
		if _, exists := seen[stored.Grant.ID]; exists {
			return errors.New("confirmation store contains a duplicate grant ID")
		}
		seen[stored.Grant.ID] = struct{}{}
		if _, err := decodeSHA256(stored.TokenSHA256); err != nil {
			return errors.New("confirmation store contains an invalid token digest")
		}
	}
	if state.Grants == nil {
		state.Grants = []storedGrant{}
	}
	s.data = state
	return nil
}

func (s *GrantStore) persistLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return fmt.Errorf("create confirmation store directory: %w", err)
	}
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("encode confirmation store: %w", err)
	}
	data = append(data, '\n')
	temp, err := os.CreateTemp(filepath.Dir(s.path), ".grants-*")
	if err != nil {
		return fmt.Errorf("create confirmation store temp file: %w", err)
	}
	name := temp.Name()
	defer os.Remove(name)
	if err := temp.Chmod(0600); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, s.path); err != nil {
		return fmt.Errorf("replace confirmation store: %w", err)
	}
	return nil
}

func cloneGrant(grant ConfirmationGrant) ConfirmationGrant {
	grant.Approver = cloneActor(grant.Approver)
	return grant
}
