package security

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

var actorIDPattern = regexp.MustCompile(`^[a-z][a-z0-9.-]{1,31}:[A-Za-z0-9][A-Za-z0-9_.@:-]{0,95}$`)

var rolePermissions = map[string][]Permission{
	"reader": {
		PermissionControlRead,
		PermissionInventoryRead,
		PermissionOperationCatalog,
	},
	"operator": {
		PermissionControlRead,
		PermissionInventoryRead,
		PermissionOperationCatalog,
		PermissionOperationPlan,
		PermissionOperationExecute,
	},
	"auditor": {
		PermissionControlRead,
		PermissionAuditRead,
	},
	"approver": {
		PermissionControlRead,
		PermissionInventoryRead,
		PermissionOperationCatalog,
		PermissionOperationPlan,
		PermissionOperationExecute,
		PermissionAuditRead,
		PermissionConfirm,
	},
	"tci-proposer": {
		PermissionControlRead,
		PermissionInventoryRead,
		PermissionOperationCatalog,
		PermissionOperationPlan,
		PermissionTCIPropose,
	},
	"service": {
		PermissionControlRead,
		PermissionInventoryRead,
		PermissionOperationCatalog,
		PermissionOperationPlan,
		PermissionOperationExecute,
		PermissionAuditRead,
	},
}

type Authenticator interface {
	AuthenticateBearer(string) (Actor, bool)
}

type MultiAuthenticator []Authenticator

func (m MultiAuthenticator) AuthenticateBearer(token string) (Actor, bool) {
	for _, authenticator := range m {
		if authenticator == nil {
			continue
		}
		if actor, ok := authenticator.AuthenticateBearer(token); ok {
			return actor, true
		}
	}
	return Actor{}, false
}

type RegistryAuthenticator struct {
	credentials []ActorCredential
}

func LoadActorRegistry(path string) (*RegistryAuthenticator, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read actor registry: %w", err)
	}
	if len(data) > 1<<20 {
		return nil, errors.New("actor registry exceeds 1 MiB")
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var registry ActorRegistry
	if err := decoder.Decode(&registry); err != nil {
		return nil, fmt.Errorf("decode actor registry: %w", err)
	}
	if registry.Schema != ActorSchema {
		return nil, fmt.Errorf("unsupported actor registry schema %q", registry.Schema)
	}
	if len(registry.Actors) == 0 {
		return nil, errors.New("actor registry must contain at least one actor")
	}
	seen := make(map[string]struct{}, len(registry.Actors))
	for index := range registry.Actors {
		credential := &registry.Actors[index]
		if err := normalizeActor(&credential.Actor); err != nil {
			return nil, fmt.Errorf("actor %d: %w", index, err)
		}
		if _, exists := seen[credential.Actor.ID]; exists {
			return nil, fmt.Errorf("duplicate actor ID %q", credential.Actor.ID)
		}
		seen[credential.Actor.ID] = struct{}{}
		digest, err := decodeSHA256(credential.TokenSHA256)
		if err != nil || allZero(digest) {
			return nil, fmt.Errorf("actor %q has an invalid token_sha256", credential.Actor.ID)
		}
		credential.TokenSHA256 = hex.EncodeToString(digest)
	}
	return &RegistryAuthenticator{credentials: registry.Actors}, nil
}

func NewStaticAuthenticator(actor Actor, rawToken string) (*RegistryAuthenticator, error) {
	if strings.TrimSpace(rawToken) == "" {
		return nil, errors.New("raw token is required")
	}
	if err := normalizeActor(&actor); err != nil {
		return nil, err
	}
	digest := sha256.Sum256([]byte(rawToken))
	return &RegistryAuthenticator{credentials: []ActorCredential{{
		Actor: actor, TokenSHA256: hex.EncodeToString(digest[:]),
	}}}, nil
}

func (a *RegistryAuthenticator) AuthenticateBearer(token string) (Actor, bool) {
	if a == nil || strings.TrimSpace(token) == "" {
		return Actor{}, false
	}
	provided := sha256.Sum256([]byte(token))
	for _, credential := range a.credentials {
		expected, err := decodeSHA256(credential.TokenSHA256)
		if err != nil {
			continue
		}
		if subtle.ConstantTimeCompare(provided[:], expected) == 1 {
			return cloneActor(credential.Actor), true
		}
	}
	return Actor{}, false
}

// LocalReadOnlyActor is used only when the public API is loopback-only and no
// credential source is configured. It may plan and execute current read-only
// operations but has no audit-read, confirmation or mutation authority.
func LocalReadOnlyActor() Actor {
	actor := Actor{
		ID:          "service:loopback-readonly",
		Kind:        ActorService,
		DisplayName: "Loopback read-only bootstrap",
		Roles:       []string{"operator"},
	}
	_ = normalizeActor(&actor)
	return actor
}

func LegacyServiceActor() Actor {
	actor := Actor{
		ID:          "service:legacy-api-token",
		Kind:        ActorService,
		DisplayName: "Legacy Quantum Control API token",
		Roles:       []string{"service"},
	}
	_ = normalizeActor(&actor)
	return actor
}

func HasPermission(actor Actor, permission Permission) bool {
	for _, granted := range actor.Permissions {
		if granted == permission {
			return true
		}
	}
	return false
}

func normalizeActor(actor *Actor) error {
	actor.ID = strings.TrimSpace(actor.ID)
	actor.DisplayName = strings.TrimSpace(actor.DisplayName)
	if !actorIDPattern.MatchString(actor.ID) {
		return errors.New("actor ID does not match policy")
	}
	switch actor.Kind {
	case ActorHuman, ActorService, ActorTCI:
	default:
		return fmt.Errorf("unsupported actor kind %q", actor.Kind)
	}
	if actor.DisplayName == "" || len(actor.DisplayName) > 160 {
		return errors.New("actor display_name is required and must not exceed 160 characters")
	}
	if len(actor.Roles) == 0 {
		return errors.New("actor must have at least one role")
	}
	roles := append([]string{}, actor.Roles...)
	sort.Strings(roles)
	roles = uniqueStrings(roles)
	permissions := make([]Permission, 0)
	for _, role := range roles {
		granted, ok := rolePermissions[role]
		if !ok {
			return fmt.Errorf("unknown role %q", role)
		}
		if actor.Kind == ActorTCI && role != "tci-proposer" && role != "reader" {
			return fmt.Errorf("TCI actor may not hold elevated role %q", role)
		}
		permissions = append(permissions, granted...)
	}
	permissions = uniquePermissions(permissions)
	if actor.Kind == ActorTCI && HasPermission(Actor{Permissions: permissions}, PermissionConfirm) {
		return errors.New("TCI actor may not receive confirmation permission")
	}
	actor.Roles = roles
	actor.Permissions = permissions
	return nil
}

func cloneActor(actor Actor) Actor {
	actor.Roles = append([]string{}, actor.Roles...)
	actor.Permissions = append([]Permission{}, actor.Permissions...)
	return actor
}

func decodeSHA256(value string) ([]byte, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	if len(value) != 64 {
		return nil, errors.New("SHA-256 digest must contain 64 hexadecimal characters")
	}
	return hex.DecodeString(value)
}

func allZero(data []byte) bool {
	for _, value := range data {
		if value != 0 {
			return false
		}
	}
	return true
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	result := values[:0]
	for index, value := range values {
		if index == 0 || value != values[index-1] {
			result = append(result, value)
		}
	}
	return result
}

func uniquePermissions(values []Permission) []Permission {
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	if len(values) == 0 {
		return []Permission{}
	}
	result := values[:0]
	for index, value := range values {
		if index == 0 || value != values[index-1] {
			result = append(result, value)
		}
	}
	return result
}
