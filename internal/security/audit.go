package security

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const maxAuditLineBytes = 1 << 20

type AuditStore struct {
	mu       sync.Mutex
	path     string
	now      func() time.Time
	records  []AuditRecord
	lastHash string
	sequence uint64
}

func OpenAuditStore(path string) (*AuditStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("audit store path is required")
	}
	store := &AuditStore{path: path, now: time.Now, records: []AuditRecord{}}
	if err := store.loadAndVerify(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *AuditStore) Append(event AuditEvent) (AuditRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	record := AuditRecord{
		Schema:       AuditSchema,
		Sequence:     s.sequence + 1,
		ID:           newSecurityID("audit"),
		Timestamp:    s.now().UTC(),
		Event:        sanitizeLabel(event.Event, 96),
		Actor:        cloneActor(event.Actor),
		RequestID:    sanitizeLabel(event.RequestID, 160),
		SessionID:    sanitizeLabel(event.SessionID, 160),
		PlanID:       sanitizeLabel(event.PlanID, 160),
		PlanDigest:   sanitizeDigest(event.PlanDigest),
		Action:       sanitizeLabel(event.Action, 160),
		Risk:         sanitizeLabel(event.Risk, 64),
		Status:       sanitizeLabel(event.Status, 64),
		Parameters:   RedactParameters(event.Parameters),
		ErrorCode:    sanitizeLabel(event.ErrorCode, 96),
		PreviousHash: s.lastHash,
	}
	if record.Event == "" || record.Actor.ID == "" || record.Status == "" {
		return AuditRecord{}, errors.New("audit event, actor and status are required")
	}
	hash, err := auditHash(record)
	if err != nil {
		return AuditRecord{}, err
	}
	record.EntryHash = hash
	line, err := json.Marshal(record)
	if err != nil {
		return AuditRecord{}, fmt.Errorf("encode audit record: %w", err)
	}
	if len(line) > maxAuditLineBytes {
		return AuditRecord{}, errors.New("audit record exceeds line limit")
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return AuditRecord{}, fmt.Errorf("create audit directory: %w", err)
	}
	file, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return AuditRecord{}, fmt.Errorf("open audit store: %w", err)
	}
	line = append(line, '\n')
	if _, err := file.Write(line); err != nil {
		file.Close()
		return AuditRecord{}, fmt.Errorf("append audit record: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return AuditRecord{}, fmt.Errorf("sync audit record: %w", err)
	}
	if err := file.Close(); err != nil {
		return AuditRecord{}, fmt.Errorf("close audit store: %w", err)
	}
	s.sequence = record.Sequence
	s.lastHash = record.EntryHash
	s.records = append(s.records, cloneAuditRecord(record))
	return cloneAuditRecord(record), nil
}

func (s *AuditStore) Query(limit int, actorID, action string) []AuditRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	actorID = strings.TrimSpace(actorID)
	action = strings.TrimSpace(action)
	result := make([]AuditRecord, 0, limit)
	for index := len(s.records) - 1; index >= 0 && len(result) < limit; index-- {
		record := s.records[index]
		if actorID != "" && record.Actor.ID != actorID {
			continue
		}
		if action != "" && record.Action != action {
			continue
		}
		result = append(result, cloneAuditRecord(record))
	}
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return result
}

func (s *AuditStore) Integrity() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return map[string]any{
		"schema":    AuditSchema,
		"records":   s.sequence,
		"head_hash": s.lastHash,
		"verified":  true,
	}
}

func (s *AuditStore) loadAndVerify() error {
	file, err := os.Open(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open audit store: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64<<10), maxAuditLineBytes)
	var previous string
	var sequence uint64
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			return fmt.Errorf("audit integrity failure at line %d: empty record", lineNumber)
		}
		var record AuditRecord
		decoder := json.NewDecoder(strings.NewReader(string(line)))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&record); err != nil {
			return fmt.Errorf("audit integrity failure at line %d: invalid JSON", lineNumber)
		}
		if record.Schema != AuditSchema || record.Sequence != sequence+1 || record.PreviousHash != previous {
			return fmt.Errorf("audit integrity failure at line %d: chain metadata mismatch", lineNumber)
		}
		expected, err := auditHash(record)
		if err != nil || !strings.EqualFold(expected, record.EntryHash) {
			return fmt.Errorf("audit integrity failure at line %d: entry hash mismatch", lineNumber)
		}
		if record.Actor.ID == "" || record.Event == "" || record.Status == "" {
			return fmt.Errorf("audit integrity failure at line %d: required field missing", lineNumber)
		}
		sequence = record.Sequence
		previous = record.EntryHash
		s.records = append(s.records, cloneAuditRecord(record))
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read audit store: %w", err)
	}
	s.sequence = sequence
	s.lastHash = previous
	return nil
}

func auditHash(record AuditRecord) (string, error) {
	parameters := make([]Parameter, 0, len(record.Parameters))
	for name, value := range record.Parameters {
		parameters = append(parameters, Parameter{Name: name, Value: value})
	}
	sort.Slice(parameters, func(i, j int) bool { return parameters[i].Name < parameters[j].Name })
	canonical := struct {
		Schema       string       `json:"schema"`
		Sequence     uint64       `json:"sequence"`
		ID           string       `json:"id"`
		Timestamp    string       `json:"timestamp"`
		Event        string       `json:"event"`
		ActorID      string       `json:"actor_id"`
		ActorKind    ActorKind    `json:"actor_kind"`
		Roles        []string     `json:"roles"`
		Permissions  []Permission `json:"permissions"`
		RequestID    string       `json:"request_id,omitempty"`
		SessionID    string       `json:"session_id,omitempty"`
		PlanID       string       `json:"plan_id,omitempty"`
		PlanDigest   string       `json:"plan_digest,omitempty"`
		Action       string       `json:"action,omitempty"`
		Risk         string       `json:"risk,omitempty"`
		Status       string       `json:"status"`
		Parameters   []Parameter  `json:"parameters,omitempty"`
		ErrorCode    string       `json:"error_code,omitempty"`
		PreviousHash string       `json:"previous_hash"`
	}{
		Schema: record.Schema, Sequence: record.Sequence, ID: record.ID,
		Timestamp: record.Timestamp.UTC().Format(time.RFC3339Nano), Event: record.Event,
		ActorID: record.Actor.ID, ActorKind: record.Actor.Kind,
		Roles: append([]string{}, record.Actor.Roles...), Permissions: append([]Permission{}, record.Actor.Permissions...),
		RequestID: record.RequestID, SessionID: record.SessionID, PlanID: record.PlanID,
		PlanDigest: record.PlanDigest, Action: record.Action, Risk: record.Risk,
		Status: record.Status, Parameters: parameters, ErrorCode: record.ErrorCode,
		PreviousHash: record.PreviousHash,
	}
	sort.Strings(canonical.Roles)
	sort.Slice(canonical.Permissions, func(i, j int) bool { return canonical.Permissions[i] < canonical.Permissions[j] })
	data, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("canonicalize audit record: %w", err)
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func RedactParameters(parameters map[string]string) map[string]string {
	if len(parameters) == 0 {
		return map[string]string{}
	}
	result := make(map[string]string, len(parameters))
	for key, value := range parameters {
		if secretKey(key) {
			result[key] = "[REDACTED]"
			continue
		}
		if len(value) > 512 {
			value = value[:512] + "[TRUNCATED]"
		}
		result[key] = value
	}
	return result
}

func secretKey(key string) bool {
	value := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(key), "-", "_"), ".", "_"))
	for _, marker := range []string{"password", "passwd", "passphrase", "token", "secret", "credential", "api_key", "apikey", "private_key"} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func sanitizeLabel(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) > limit {
		value = value[:limit]
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return ""
		}
	}
	return value
}

func sanitizeDigest(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) != 64 {
		return ""
	}
	if _, err := hex.DecodeString(value); err != nil {
		return ""
	}
	return value
}

func cloneAuditRecord(record AuditRecord) AuditRecord {
	record.Actor = cloneActor(record.Actor)
	if record.Parameters != nil {
		copyMap := make(map[string]string, len(record.Parameters))
		for key, value := range record.Parameters {
			copyMap[key] = value
		}
		record.Parameters = copyMap
	}
	return record
}
