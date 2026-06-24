package sqlitecli

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Floodnut/ergo-loom/internal/core"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	DBPath     string
	SchemaPath string
}

type RegistryItem struct {
	ID          string
	DisplayName string
	Kind        string
	Enabled     bool
}

type ProviderProfile struct {
	ID               string
	ProviderPluginID string
	DisplayName      string
	IsDefault        bool
}

type ProviderModel struct {
	ID               string
	ProviderPluginID string
	DisplayName      string
	ModelRef         string
	Status           string
	IsDefault        bool
}

type AccessRoute struct {
	ID                 string
	ProviderPluginID   string
	DisplayName        string
	AccessMode         string
	Transport          string
	RequiresLicense    bool
	RequiresAPIKey     bool
	SupportsStreaming  bool
	SupportsTools      bool
	SupportsImport     bool
	SupportsHandoff    bool
	CostModel          string
	Status             string
	ContextBudgetChars int // 0 = policy default
}

type Project struct {
	ID                string
	DisplayName       string
	RootPath          string
	IsDefault         bool
	ContextPolicy     string
	HandoffPolicy     string
	RoutePolicy       string
	ToolApprovalPolicy string
	KbScopePolicy     string
}

type CommandRun struct {
	ID         string
	ProjectID  string
	SessionID  string
	Command    string
	WorkingDir string
	Status     string
	ExitCode   int
	Stdout     string
	Stderr     string
	StartedAt  string
	FinishedAt string
}

type ProjectAccessRoute struct {
	ProjectID string
	Route     AccessRoute
	Enabled   bool
	Priority  int
}

type ModeratorPreference struct {
	Scope                    string
	ProjectID                string
	Mode                     string
	PrimaryProviderGroupID   string
	SecondaryProviderGroupID string
	Source                   string
}

type TokenUsageInput struct {
	ProviderPluginID  string
	ProviderProfileID string
	SessionID         string
	AgentRunID        string
	Model             string
	PromptTokens      int
	CompletionTokens  int
	Status            string
}

type TokenUsageSummary struct {
	ProviderPluginID  string
	ProviderProfileID string
	Model             string
	PromptTokens      int
	CompletionTokens  int
	Requests          int
}

type SessionContextUsage struct {
	MessageCount     int
	EstimatedTokens  int
	PromptTokens     int
	CompletionTokens int
	ProviderChats    int
}

type ProviderChatBinding struct {
	ID                string
	SessionID         string
	ProviderPluginID  string
	ProviderProfileID string
	AccessRouteID     string
	ModelID           string
	ExternalThreadID  string
	Status            string
}

type ProviderChatBindingInput struct {
	SessionID         string
	ProviderPluginID  string
	ProviderProfileID string
	AccessRouteID     string
	ModelID           string
	ExternalThreadID  string
	Status            string
}

type ChatRunInput struct {
	ProjectID       string
	SessionID       string
	BranchID        string
	Role            core.ChatRunRole
	Status          core.ChatRunStatus
	InputEventID    string
	ContextPacketID string
}

type ProviderSegmentInput struct {
	ChatRunID        string
	ProviderID       string
	RouteID          string
	ModelID          string
	ExternalThreadID string
	Status           core.ProviderSegmentStatus
	HandoffReason    string
}

type ContextPacketRecord struct {
	ID             string
	ProjectID      string
	SessionID      string
	BranchID       string
	HeadEventID    string
	UserInput      string
	ContentRef     string
	ReferenceCount int
	CreatedAt      time.Time
}

type SteeringInput struct {
	ChatRunID         string
	ProviderSegmentID string
	EventID           string
	Content           string
	Status            string
}

type SteeringRecord struct {
	ID                string
	ChatRunID         string
	ProviderSegmentID string
	EventID           string
	Content           string
	Status            string
	CreatedAt         time.Time
}

type QueueItemInput struct {
	SessionID      string
	BranchID       string
	Content        string
	Mode           core.QueueItemMode
	RouteID        string
	ModelID        string
	ThinkingEffort string
}

type CandidateOutputInput struct {
	ChatRunID      string
	SessionID      string
	BranchID       string
	TriggerEventID string
	Content        string
	Status         core.CandidateOutputStatus
}

type CandidateMergeResult struct {
	Candidate              core.CandidateOutput `json:"candidate"`
	Message                core.Message         `json:"message"`
	Event                  core.Event           `json:"event"`
	SupersededCandidateIDs []string             `json:"supersededCandidateIds"`
}

type MessageEventInput struct {
	SessionID     string
	MessageID     string
	ActivityIndex int
	Kind          string
	PayloadJSON   string
}

type MessageEvent struct {
	ID            string `json:"id"`
	SessionID     string `json:"sessionId"`
	MessageID     string `json:"messageId"`
	ActivityIndex int    `json:"activityIndex"`
	Kind          string `json:"kind"`
	PayloadJSON   string `json:"payloadJson"`
	CreatedAt     string `json:"createdAt"`
}

type KnowledgeSearchOptions struct {
	Query     string
	Scope     core.KnowledgeScope
	ProjectID string
	Limit     int
}

func New(dbPath string) Store {
	return Store{
		DBPath:     dbPath,
		SchemaPath: filepath.Join("internal", "storage", "sqlitecli", "schema.sql"),
	}
}

func (s Store) Init() error {
	if err := os.MkdirAll(filepath.Dir(s.DBPath), 0o755); err != nil {
		return err
	}

	schema, err := os.ReadFile(s.SchemaPath)
	if err != nil {
		return err
	}

	if err := s.run(string(schema)); err != nil {
		return err
	}
	if err := s.ensureSessionProjectColumn(); err != nil {
		return err
	}
	if err := s.ensureSessionActiveRouteColumns(); err != nil {
		return err
	}
	if err := s.ensureAccessRouteContextBudgetColumn(); err != nil {
		return err
	}
	if err := s.ensureProjectContextPolicyColumn(); err != nil {
		return err
	}
	if err := s.ensureProjectHandoffPolicyColumn(); err != nil {
		return err
	}
	if err := s.ensureProjectRoutePolicyColumn(); err != nil {
		return err
	}
	if err := s.ensureProjectToolApprovalPolicyColumn(); err != nil {
		return err
	}
	if err := s.ensureProjectKbScopePolicyColumn(); err != nil {
		return err
	}
	return s.ensureCandidateTriggerEventColumn()
}

func (s Store) ListSessions() ([]core.Session, error) {
	return s.ListSessionsForProject("")
}

func (s Store) ListSessionsForProject(projectID string) ([]core.Session, error) {
	projectID = strings.TrimSpace(projectID)
	where := ""
	if projectID != "" {
		where = fmt.Sprintf("WHERE COALESCE(project_id, 'default') = %s", quote(projectID))
	}
	out, err := s.queryJSON(`
SELECT id, COALESCE(project_id, 'default') AS project_id, source_tool, source_id, title, created_at, updated_at,
       COALESCE(parent_session_id, '') AS parent_id,
       COALESCE(branch_from_message_id, '') AS branch_from_id,
       COALESCE(active_route_id, '') AS active_route_id,
       COALESCE(active_model_id, '') AS active_model_id
FROM sessions
` + where + `
ORDER BY updated_at DESC, id ASC;
`)
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(out)) == 0 {
		return []core.Session{}, nil
	}

	var rows []sessionRow
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		return nil, err
	}

	sessions := make([]core.Session, 0, len(rows))
	for _, row := range rows {
		session, err := row.toCore()
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}
	return sessions, nil
}

func (s Store) GetSession(id string) (core.Session, []core.Message, error) {
	sessionOut, err := s.queryJSON(fmt.Sprintf(`
SELECT id, COALESCE(project_id, 'default') AS project_id, source_tool, source_id, title, created_at, updated_at,
       COALESCE(parent_session_id, '') AS parent_id,
       COALESCE(branch_from_message_id, '') AS branch_from_id,
       COALESCE(active_route_id, '') AS active_route_id,
       COALESCE(active_model_id, '') AS active_model_id
FROM sessions
WHERE id = %s;
`, quote(id)))
	if err != nil {
		return core.Session{}, nil, err
	}

	var sessionRows []sessionRow
	if err := json.Unmarshal([]byte(emptyArray(sessionOut)), &sessionRows); err != nil {
		return core.Session{}, nil, err
	}
	if len(sessionRows) == 0 {
		return core.Session{}, nil, ErrNotFound
	}

	session, err := sessionRows[0].toCore()
	if err != nil {
		return core.Session{}, nil, err
	}

	messageOut, err := s.queryJSON(fmt.Sprintf(`
SELECT id, session_id, role, content, created_at, COALESCE(source_id, '') AS source_id
FROM messages
WHERE session_id = %s
ORDER BY ordinal ASC;
`, quote(id)))
	if err != nil {
		return core.Session{}, nil, err
	}

	var messageRows []messageRow
	if err := json.Unmarshal([]byte(emptyArray(messageOut)), &messageRows); err != nil {
		return core.Session{}, nil, err
	}

	messages := make([]core.Message, 0, len(messageRows))
	for _, row := range messageRows {
		message, err := row.toCore()
		if err != nil {
			return core.Session{}, nil, err
		}
		messages = append(messages, message)
	}

	return session, messages, nil
}

func (s Store) CreateChatSession(title string) (core.Session, error) {
	return s.CreateChatSessionForProject("", title)
}

func (s Store) CreateChatSessionForProject(projectID string, title string) (core.Session, error) {
	if strings.TrimSpace(title) == "" {
		title = "Untitled chat"
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		projectID = "default"
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	sessionID := "session_" + randomHex(16)
	stmt := fmt.Sprintf(`INSERT INTO sessions (id, project_id, source_tool, source_id, title, created_at, updated_at)
VALUES (%s, %s, 'ergo', %s, %s, %s, %s);
`, quote(sessionID), quote(projectID), quote(sessionID), quote(title), quote(now), quote(now))

	if err := s.run(stmt); err != nil {
		return core.Session{}, err
	}
	session, _, err := s.GetSession(sessionID)
	return session, err
}

func (s Store) ListSessionProviderGroups(sessionID string) ([]string, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, errors.New("session id is required")
	}
	out, err := s.queryJSON(fmt.Sprintf(`
SELECT provider_group_id
FROM session_provider_groups
WHERE session_id = %s
ORDER BY created_at ASC, provider_group_id ASC;
`, quote(sessionID)))
	if err != nil {
		return nil, err
	}
	var rows []struct {
		ProviderGroupID string `json:"provider_group_id"`
	}
	if err := json.Unmarshal([]byte(emptyArray(out)), &rows); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row.ProviderGroupID) != "" {
			ids = append(ids, row.ProviderGroupID)
		}
	}
	return ids, nil
}

func (s Store) SetSessionProviderGroups(sessionID string, providerGroupIDs []string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return errors.New("session id is required")
	}
	if _, _, err := s.GetSession(sessionID); err != nil {
		return err
	}
	seen := map[string]bool{}
	values := make([]string, 0, len(providerGroupIDs))
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, providerGroupID := range providerGroupIDs {
		providerGroupID = strings.TrimSpace(providerGroupID)
		if providerGroupID == "" || seen[providerGroupID] {
			continue
		}
		seen[providerGroupID] = true
		values = append(values, fmt.Sprintf("(%s, %s, %s)", quote(sessionID), quote(providerGroupID), quote(now)))
	}
	stmt := fmt.Sprintf("DELETE FROM session_provider_groups WHERE session_id = %s;\n", quote(sessionID))
	if len(values) > 0 {
		stmt += "INSERT INTO session_provider_groups (session_id, provider_group_id, created_at) VALUES\n"
		stmt += strings.Join(values, ",\n") + ";\n"
	}
	return s.run(stmt)
}

func (s Store) RenameSession(sessionID string, title string) (core.Session, error) {
	sessionID = strings.TrimSpace(sessionID)
	title = strings.TrimSpace(title)
	if sessionID == "" {
		return core.Session{}, errors.New("session id is required")
	}
	if title == "" {
		return core.Session{}, errors.New("chat title is required")
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	stmt := fmt.Sprintf(`UPDATE sessions
SET title = %s, updated_at = %s
WHERE id = %s;
`, quote(title), quote(now), quote(sessionID))
	if err := s.run(stmt); err != nil {
		return core.Session{}, err
	}
	session, _, err := s.GetSession(sessionID)
	return session, err
}

func (s Store) DeleteSession(sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return errors.New("session id is required")
	}
	if _, _, err := s.GetSession(sessionID); err != nil {
		return err
	}
	stmt := fmt.Sprintf(`BEGIN;
DELETE FROM branches WHERE parent_session_id = %s OR session_id = %s;
UPDATE sessions SET parent_session_id = NULL, branch_from_message_id = NULL WHERE parent_session_id = %s;
DELETE FROM message_events WHERE session_id = %s;
DELETE FROM messages WHERE session_id = %s;
DELETE FROM session_provider_groups WHERE session_id = %s;
DELETE FROM provider_chats WHERE session_id = %s;
DELETE FROM token_ledger WHERE session_id = %s;
UPDATE command_runs SET session_id = NULL WHERE session_id = %s;
UPDATE agent_runs SET session_id = NULL WHERE session_id = %s;
DELETE FROM sessions WHERE id = %s;
COMMIT;
`, quote(sessionID), quote(sessionID), quote(sessionID), quote(sessionID), quote(sessionID), quote(sessionID), quote(sessionID), quote(sessionID), quote(sessionID), quote(sessionID), quote(sessionID))
	return s.run(stmt)
}

func (s Store) AddMessage(sessionID string, role string, content string) (core.Message, error) {
	if strings.TrimSpace(role) == "" {
		return core.Message{}, errors.New("role is required")
	}
	if strings.TrimSpace(content) == "" {
		return core.Message{}, errors.New("content is required")
	}

	session, _, err := s.GetSession(sessionID)
	if err != nil {
		return core.Message{}, err
	}

	ordinal, err := s.nextMessageOrdinal(sessionID)
	if err != nil {
		return core.Message{}, err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	messageID := "message_" + randomHex(16)
	stmt := fmt.Sprintf(`BEGIN;
INSERT INTO messages (id, session_id, role, content, created_at, ordinal)
VALUES (%s, %s, %s, %s, %s, %d);
UPDATE sessions SET updated_at = %s WHERE id = %s;
COMMIT;
`, quote(messageID), quote(sessionID), quote(role), quote(content), quote(now), ordinal, quote(now), quote(sessionID))

	if err := s.run(stmt); err != nil {
		return core.Message{}, err
	}
	if _, err := s.appendMessageEventProjection(session, messageID, role); err != nil {
		return core.Message{}, err
	}
	_, messages, err := s.GetSession(sessionID)
	if err != nil {
		return core.Message{}, err
	}
	for _, message := range messages {
		if message.ID == messageID {
			return message, nil
		}
	}
	return core.Message{}, ErrNotFound
}

func (s Store) appendMessageEventProjection(session core.Session, messageID string, role string) (core.Event, error) {
	eventType := messageEventType(role)
	parentIDs := []string{}
	if head, err := s.getHead(session.ProjectID, session.ID, "main"); err == nil && strings.TrimSpace(head.EventID) != "" {
		parentIDs = append(parentIDs, head.EventID)
	} else if err != nil && !errors.Is(err, ErrNotFound) {
		return core.Event{}, err
	}
	event, err := s.AppendEvent(core.EventInput{
		Type:           eventType,
		ProjectID:      session.ProjectID,
		SessionID:      session.ID,
		BranchID:       "main",
		ParentEventIDs: parentIDs,
		PayloadRef:     "message:" + messageID,
	})
	if err != nil {
		return core.Event{}, err
	}
	if _, err := s.MoveHead(session.ProjectID, session.ID, "main", event.ID); err != nil {
		return core.Event{}, err
	}
	return event, nil
}

func messageEventType(role string) core.EventType {
	switch strings.TrimSpace(strings.ToLower(role)) {
	case "user":
		return core.EventMessageUser
	case "assistant":
		return core.EventMessageAssistant
	default:
		return core.EventType("message." + strings.TrimSpace(strings.ToLower(role)))
	}
}

func (s Store) AddMessageEvent(input MessageEventInput) (MessageEvent, error) {
	input.SessionID = strings.TrimSpace(input.SessionID)
	input.Kind = strings.TrimSpace(input.Kind)
	if input.SessionID == "" {
		return MessageEvent{}, errors.New("session id is required")
	}
	if input.Kind == "" {
		return MessageEvent{}, errors.New("message event kind is required")
	}
	if strings.TrimSpace(input.PayloadJSON) == "" {
		input.PayloadJSON = "{}"
	}
	if input.ActivityIndex < 0 {
		input.ActivityIndex = 0
	}

	eventID := "message_event_" + randomHex(16)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	stmt := fmt.Sprintf(`INSERT INTO message_events (id, session_id, message_id, activity_index, kind, payload_json, created_at)
VALUES (%s, %s, %s, %d, %s, %s, %s);
`, quote(eventID), quote(input.SessionID), nullable(strings.TrimSpace(input.MessageID)), input.ActivityIndex, quote(input.Kind), quote(input.PayloadJSON), quote(now))
	if err := s.run(stmt); err != nil {
		return MessageEvent{}, err
	}
	return MessageEvent{
		ID:            eventID,
		SessionID:     input.SessionID,
		MessageID:     strings.TrimSpace(input.MessageID),
		ActivityIndex: input.ActivityIndex,
		Kind:          input.Kind,
		PayloadJSON:   input.PayloadJSON,
		CreatedAt:     now,
	}, nil
}

func (s Store) ListMessageEvents(sessionID string) ([]MessageEvent, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, errors.New("session id is required")
	}
	out, err := s.queryJSON(fmt.Sprintf(`
SELECT id, session_id, COALESCE(message_id, '') AS message_id, activity_index, kind, payload_json, created_at
FROM message_events
WHERE session_id = %s
ORDER BY activity_index ASC, created_at ASC, id ASC;
`, quote(sessionID)))
	if err != nil {
		return nil, err
	}
	var rows []messageEventRow
	if err := json.Unmarshal([]byte(emptyArray(out)), &rows); err != nil {
		return nil, err
	}
	events := make([]MessageEvent, 0, len(rows))
	for _, row := range rows {
		events = append(events, row.toCore())
	}
	return events, nil
}

func (s Store) AppendEvent(input core.EventInput) (core.Event, error) {
	input.Type = core.EventType(strings.TrimSpace(string(input.Type)))
	input.ProjectID = strings.TrimSpace(input.ProjectID)
	input.SessionID = strings.TrimSpace(input.SessionID)
	input.BranchID = strings.TrimSpace(input.BranchID)
	input.PayloadRef = strings.TrimSpace(input.PayloadRef)
	if input.Type == "" {
		return core.Event{}, errors.New("event type is required")
	}
	if input.BranchID == "" {
		input.BranchID = "main"
	}
	if input.ProjectID == "" && input.SessionID != "" {
		if session, _, err := s.GetSession(input.SessionID); err == nil {
			input.ProjectID = session.ProjectID
		}
	}

	eventID := "event_" + randomHex(16)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	createdAt, err := parseTime(now)
	if err != nil {
		return core.Event{}, err
	}
	parentIDs := compactStrings(input.ParentEventIDs)
	event := core.Event{
		ID:             eventID,
		Type:           input.Type,
		ProjectID:      input.ProjectID,
		SessionID:      input.SessionID,
		BranchID:       input.BranchID,
		ParentEventIDs: parentIDs,
		PayloadRef:     input.PayloadRef,
		CreatedAt:      createdAt,
	}
	if err := s.writeContextEventObject(event); err != nil {
		return core.Event{}, err
	}
	stmt := fmt.Sprintf(`BEGIN;
INSERT INTO context_events (id, type, project_id, session_id, branch_id, payload_ref, created_at)
VALUES (%s, %s, %s, %s, %s, %s, %s);
`, quote(eventID), quote(string(input.Type)), nullable(input.ProjectID), nullable(input.SessionID), quote(input.BranchID), quote(input.PayloadRef), quote(now))
	for i, parentID := range parentIDs {
		stmt += fmt.Sprintf(`INSERT INTO context_event_parents (event_id, parent_event_id, ordinal)
VALUES (%s, %s, %d);
`, quote(eventID), quote(parentID), i)
	}
	stmt += "COMMIT;\n"
	if err := s.run(stmt); err != nil {
		return core.Event{}, err
	}
	return s.GetEvent(eventID)
}

func (s Store) writeContextEventObject(event core.Event) error {
	if strings.TrimSpace(s.DBPath) == "" {
		return nil
	}
	path := filepath.Join(filepath.Dir(s.DBPath), "objects", "events", event.ID+".json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(event, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func (s Store) GetEvent(id string) (core.Event, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return core.Event{}, errors.New("event id is required")
	}
	out, err := s.queryJSON(fmt.Sprintf(`
SELECT ce.id,
       ce.type,
       COALESCE(ce.project_id, '') AS project_id,
       COALESCE(ce.session_id, '') AS session_id,
       COALESCE(ce.branch_id, '') AS branch_id,
       COALESCE(ce.payload_ref, '') AS payload_ref,
       ce.created_at,
       COALESCE(GROUP_CONCAT(cep.parent_event_id, char(31)), '') AS parent_event_ids
FROM context_events ce
LEFT JOIN context_event_parents cep ON cep.event_id = ce.id
WHERE ce.id = %s
GROUP BY ce.id;
`, quote(id)))
	if err != nil {
		return core.Event{}, err
	}
	var rows []contextEventRow
	if err := json.Unmarshal([]byte(emptyArray(out)), &rows); err != nil {
		return core.Event{}, err
	}
	if len(rows) == 0 {
		return core.Event{}, ErrNotFound
	}
	return rows[0].toCore()
}

func (s Store) ListAncestors(headEventID string, limit int) ([]core.Event, error) {
	headEventID = strings.TrimSpace(headEventID)
	if headEventID == "" {
		return nil, errors.New("head event id is required")
	}
	if limit <= 0 {
		limit = 100
	}
	out, err := s.queryJSON(fmt.Sprintf(`
WITH RECURSIVE ancestry(id, depth) AS (
  SELECT %s, 0
  UNION ALL
  SELECT cep.parent_event_id, ancestry.depth + 1
  FROM context_event_parents cep
  JOIN ancestry ON cep.event_id = ancestry.id
  WHERE ancestry.depth < %d
)
SELECT ce.id,
       ce.type,
       COALESCE(ce.project_id, '') AS project_id,
       COALESCE(ce.session_id, '') AS session_id,
       COALESCE(ce.branch_id, '') AS branch_id,
       COALESCE(ce.payload_ref, '') AS payload_ref,
       ce.created_at,
       COALESCE(GROUP_CONCAT(cep.parent_event_id, char(31)), '') AS parent_event_ids,
       MIN(ancestry.depth) AS depth
FROM ancestry
JOIN context_events ce ON ce.id = ancestry.id
LEFT JOIN context_event_parents cep ON cep.event_id = ce.id
GROUP BY ce.id
ORDER BY depth DESC, ce.created_at ASC, ce.id ASC;
`, quote(headEventID), limit-1))
	if err != nil {
		return nil, err
	}
	var rows []contextEventRow
	if err := json.Unmarshal([]byte(emptyArray(out)), &rows); err != nil {
		return nil, err
	}
	events := make([]core.Event, 0, len(rows))
	for _, row := range rows {
		event, err := row.toCore()
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

func (s Store) MoveHead(projectID string, sessionID string, branchID string, eventID string) (core.Head, error) {
	projectID = strings.TrimSpace(projectID)
	sessionID = strings.TrimSpace(sessionID)
	branchID = strings.TrimSpace(branchID)
	eventID = strings.TrimSpace(eventID)
	if branchID == "" {
		branchID = "main"
	}
	event, err := s.GetEvent(eventID)
	if err != nil {
		return core.Head{}, err
	}
	if projectID == "" {
		projectID = event.ProjectID
	}
	if sessionID == "" {
		sessionID = event.SessionID
	}
	if projectID == "" || sessionID == "" {
		return core.Head{}, errors.New("project id and session id are required")
	}

	headID := "head_" + randomHex(16)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	stmt := fmt.Sprintf(`
INSERT INTO context_heads (id, project_id, session_id, branch_id, event_id, updated_at)
VALUES (%s, %s, %s, %s, %s, %s)
ON CONFLICT(project_id, session_id, branch_id)
DO UPDATE SET event_id = excluded.event_id, updated_at = excluded.updated_at;
`, quote(headID), quote(projectID), quote(sessionID), quote(branchID), quote(eventID), quote(now))
	if err := s.run(stmt); err != nil {
		return core.Head{}, err
	}
	return s.getHead(projectID, sessionID, branchID)
}

func (s Store) GetHead(projectID string, sessionID string, branchID string) (core.Head, error) {
	projectID = strings.TrimSpace(projectID)
	sessionID = strings.TrimSpace(sessionID)
	branchID = strings.TrimSpace(branchID)
	if branchID == "" {
		branchID = "main"
	}
	if projectID == "" || sessionID == "" {
		return core.Head{}, errors.New("project id and session id are required")
	}
	return s.getHead(projectID, sessionID, branchID)
}

func (s Store) getHead(projectID string, sessionID string, branchID string) (core.Head, error) {
	out, err := s.queryJSON(fmt.Sprintf(`
SELECT id, project_id, session_id, branch_id, event_id, updated_at
FROM context_heads
WHERE project_id = %s AND session_id = %s AND branch_id = %s;
`, quote(projectID), quote(sessionID), quote(branchID)))
	if err != nil {
		return core.Head{}, err
	}
	var rows []contextHeadRow
	if err := json.Unmarshal([]byte(emptyArray(out)), &rows); err != nil {
		return core.Head{}, err
	}
	if len(rows) == 0 {
		return core.Head{}, ErrNotFound
	}
	return rows[0].toCore()
}

func (s Store) CreateGraphBranch(sessionID string, fromEventID string) (core.GraphBranch, error) {
	sessionID = strings.TrimSpace(sessionID)
	fromEventID = strings.TrimSpace(fromEventID)
	if sessionID == "" {
		return core.GraphBranch{}, errors.New("session id is required")
	}
	event, err := s.GetEvent(fromEventID)
	if err != nil {
		return core.GraphBranch{}, err
	}
	projectID := event.ProjectID
	if projectID == "" {
		if session, _, err := s.GetSession(sessionID); err == nil {
			projectID = session.ProjectID
		}
	}
	branchID := "branch_" + randomHex(8)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	stmt := fmt.Sprintf(`BEGIN;
INSERT INTO context_branches (id, project_id, session_id, from_event_id, head_event_id, created_at)
VALUES (%s, %s, %s, %s, %s, %s);
INSERT INTO context_heads (id, project_id, session_id, branch_id, event_id, updated_at)
VALUES (%s, %s, %s, %s, %s, %s);
COMMIT;
`, quote(branchID), nullable(projectID), quote(sessionID), quote(fromEventID), quote(fromEventID), quote(now),
		quote("head_"+randomHex(16)), nullable(projectID), quote(sessionID), quote(branchID), quote(fromEventID), quote(now))
	if err := s.run(stmt); err != nil {
		return core.GraphBranch{}, err
	}
	createdAt, err := parseTime(now)
	if err != nil {
		return core.GraphBranch{}, err
	}
	return core.GraphBranch{
		ID:          branchID,
		ProjectID:   projectID,
		SessionID:   sessionID,
		FromEventID: fromEventID,
		HeadEventID: fromEventID,
		CreatedAt:   createdAt,
	}, nil
}

func (s Store) CreateMerge(projectID string, sessionID string, branchID string, parentEventIDs []string) (core.Event, error) {
	projectID = strings.TrimSpace(projectID)
	sessionID = strings.TrimSpace(sessionID)
	branchID = strings.TrimSpace(branchID)
	if branchID == "" {
		branchID = "main"
	}
	if len(parentEventIDs) < 2 {
		return core.Event{}, errors.New("merge requires at least two parent events")
	}
	event, err := s.AppendEvent(core.EventInput{
		Type:           core.EventMergeCreated,
		ProjectID:      projectID,
		SessionID:      sessionID,
		BranchID:       branchID,
		ParentEventIDs: parentEventIDs,
		PayloadRef:     "merge",
	})
	if err != nil {
		return core.Event{}, err
	}
	if _, err := s.MoveHead(projectID, sessionID, branchID, event.ID); err != nil {
		return core.Event{}, err
	}
	return event, nil
}

func (s Store) StartChatRun(input ChatRunInput) (core.ChatRun, error) {
	input.ProjectID = strings.TrimSpace(input.ProjectID)
	input.SessionID = strings.TrimSpace(input.SessionID)
	input.BranchID = strings.TrimSpace(input.BranchID)
	input.InputEventID = strings.TrimSpace(input.InputEventID)
	input.ContextPacketID = strings.TrimSpace(input.ContextPacketID)
	if input.SessionID == "" {
		return core.ChatRun{}, errors.New("session id is required")
	}
	if input.BranchID == "" {
		input.BranchID = "main"
	}
	if input.Role == "" {
		input.Role = core.ChatRunRoleMain
	}
	if input.Status == "" {
		input.Status = core.ChatRunRunning
	}
	if input.ProjectID == "" {
		if session, _, err := s.GetSession(input.SessionID); err == nil {
			input.ProjectID = session.ProjectID
		}
	}
	runID := "chat_run_" + randomHex(16)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	stmt := fmt.Sprintf(`
INSERT INTO chat_runs (id, project_id, session_id, branch_id, role, status, input_event_id, context_packet_id, created_at, updated_at)
VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s);
`, quote(runID), nullable(input.ProjectID), quote(input.SessionID), quote(input.BranchID), quote(string(input.Role)), quote(string(input.Status)), nullable(input.InputEventID), quote(input.ContextPacketID), quote(now), quote(now))
	if err := s.run(stmt); err != nil {
		return core.ChatRun{}, err
	}
	return s.getChatRun(runID)
}

func (s Store) CompleteChatRun(id string, status core.ChatRunStatus, outputEventID string) (core.ChatRun, error) {
	id = strings.TrimSpace(id)
	outputEventID = strings.TrimSpace(outputEventID)
	if id == "" {
		return core.ChatRun{}, errors.New("chat run id is required")
	}
	if status == "" {
		status = core.ChatRunCompleted
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	stmt := fmt.Sprintf(`UPDATE chat_runs
SET status = %s, output_event_id = %s, updated_at = %s
WHERE id = %s;
`, quote(string(status)), nullable(outputEventID), quote(now), quote(id))
	if err := s.run(stmt); err != nil {
		return core.ChatRun{}, err
	}
	return s.getChatRun(id)
}

func (s Store) UpdateChatRunStatus(id string, status core.ChatRunStatus) (core.ChatRun, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return core.ChatRun{}, errors.New("chat run id is required")
	}
	if status == "" {
		status = core.ChatRunRunning
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	stmt := fmt.Sprintf(`UPDATE chat_runs
SET status = %s, updated_at = %s
WHERE id = %s;
`, quote(string(status)), quote(now), quote(id))
	if err := s.run(stmt); err != nil {
		return core.ChatRun{}, err
	}
	return s.getChatRun(id)
}

func (s Store) GetActiveChatRun(sessionID string) (core.ChatRun, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return core.ChatRun{}, errors.New("session id is required")
	}
	out, err := s.queryJSON(fmt.Sprintf(`
SELECT id, COALESCE(project_id, '') AS project_id, session_id, branch_id, role, status,
       COALESCE(input_event_id, '') AS input_event_id,
       COALESCE(output_event_id, '') AS output_event_id,
       context_packet_id, created_at, updated_at
FROM chat_runs
WHERE session_id = %s AND branch_id = 'main' AND role = %s AND status IN (%s, %s)
ORDER BY created_at DESC, id DESC
LIMIT 1;
`, quote(sessionID), quote(string(core.ChatRunRoleMain)), quote(string(core.ChatRunRunning)), quote(string(core.ChatRunWaitingApproval))))
	if err != nil {
		return core.ChatRun{}, err
	}
	var rows []chatRunRow
	if err := json.Unmarshal([]byte(emptyArray(out)), &rows); err != nil {
		return core.ChatRun{}, err
	}
	if len(rows) == 0 {
		return core.ChatRun{}, ErrNotFound
	}
	return rows[0].toCore()
}

func (s Store) ActiveMainChatRun(sessionID string) (core.ChatRun, error) {
	return s.GetActiveChatRun(sessionID)
}

func (s Store) NextQueuedChatRun(sessionID string) (core.ChatRun, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return core.ChatRun{}, errors.New("session id is required")
	}
	out, err := s.queryJSON(fmt.Sprintf(`
SELECT id, COALESCE(project_id, '') AS project_id, session_id, branch_id, role, status,
       COALESCE(input_event_id, '') AS input_event_id,
       COALESCE(output_event_id, '') AS output_event_id,
       context_packet_id, created_at, updated_at
FROM chat_runs
WHERE session_id = %s AND branch_id = 'main' AND role = %s AND status = %s
ORDER BY created_at ASC, id ASC
LIMIT 1;
`, quote(sessionID), quote(string(core.ChatRunRoleMain)), quote(string(core.ChatRunQueued))))
	if err != nil {
		return core.ChatRun{}, err
	}
	var rows []chatRunRow
	if err := json.Unmarshal([]byte(emptyArray(out)), &rows); err != nil {
		return core.ChatRun{}, err
	}
	if len(rows) == 0 {
		return core.ChatRun{}, ErrNotFound
	}
	return rows[0].toCore()
}

func (s Store) StartProviderSegment(input ProviderSegmentInput) (core.ProviderSegment, error) {
	input.ChatRunID = strings.TrimSpace(input.ChatRunID)
	input.ProviderID = strings.TrimSpace(input.ProviderID)
	input.RouteID = strings.TrimSpace(input.RouteID)
	input.ModelID = strings.TrimSpace(input.ModelID)
	input.ExternalThreadID = strings.TrimSpace(input.ExternalThreadID)
	input.HandoffReason = strings.TrimSpace(input.HandoffReason)
	if input.ChatRunID == "" {
		return core.ProviderSegment{}, errors.New("chat run id is required")
	}
	if input.ProviderID == "" {
		return core.ProviderSegment{}, errors.New("provider id is required")
	}
	if input.Status == "" {
		input.Status = core.ProviderSegmentRunning
	}
	segmentID := "provider_segment_" + randomHex(16)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	stmt := fmt.Sprintf(`
INSERT INTO provider_segments (id, chat_run_id, provider_id, route_id, model_id, external_thread_id, status, handoff_reason, started_at)
VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s);
`, quote(segmentID), quote(input.ChatRunID), quote(input.ProviderID), quote(input.RouteID), quote(input.ModelID), quote(input.ExternalThreadID), quote(string(input.Status)), quote(input.HandoffReason), quote(now))
	if err := s.run(stmt); err != nil {
		return core.ProviderSegment{}, err
	}
	return s.getProviderSegment(segmentID)
}

func (s Store) CompleteProviderSegment(id string, status core.ProviderSegmentStatus, externalThreadID string) (core.ProviderSegment, error) {
	id = strings.TrimSpace(id)
	externalThreadID = strings.TrimSpace(externalThreadID)
	if id == "" {
		return core.ProviderSegment{}, errors.New("provider segment id is required")
	}
	if status == "" {
		status = core.ProviderSegmentCompleted
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	setExternalThread := ""
	if externalThreadID != "" {
		setExternalThread = fmt.Sprintf(", external_thread_id = %s", quote(externalThreadID))
	}
	stmt := fmt.Sprintf(`UPDATE provider_segments
SET status = %s, completed_at = %s%s
WHERE id = %s;
`, quote(string(status)), quote(now), setExternalThread, quote(id))
	if err := s.run(stmt); err != nil {
		return core.ProviderSegment{}, err
	}
	return s.getProviderSegment(id)
}

func (s Store) LastCompletedSegment(sessionID string) (core.ProviderSegment, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return core.ProviderSegment{}, errors.New("session id is required")
	}
	out, err := s.queryJSON(fmt.Sprintf(`
SELECT ps.id, ps.chat_run_id, ps.provider_id, ps.route_id, ps.model_id,
       ps.external_thread_id, ps.status, ps.handoff_reason,
       ps.started_at, COALESCE(ps.completed_at, '') AS completed_at
FROM provider_segments ps
JOIN chat_runs cr ON ps.chat_run_id = cr.id
WHERE cr.session_id = %s AND cr.role = 'main' AND ps.status = 'completed'
ORDER BY ps.completed_at DESC, ps.id DESC
LIMIT 1;
`, quote(sessionID)))
	if err != nil {
		return core.ProviderSegment{}, err
	}
	var rows []providerSegmentRow
	if err := json.Unmarshal([]byte(emptyArray(out)), &rows); err != nil {
		return core.ProviderSegment{}, err
	}
	if len(rows) == 0 {
		return core.ProviderSegment{}, ErrNotFound
	}
	return rows[0].toCore()
}

func (s Store) MessagesSince(sessionID string, since time.Time) ([]core.Message, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, errors.New("session id is required")
	}
	out, err := s.queryJSON(fmt.Sprintf(`
SELECT id, session_id, COALESCE(source_id, '') AS source_id, role, content, created_at, ordinal
FROM messages
WHERE session_id = %s AND created_at >= %s
ORDER BY ordinal ASC;
`, quote(sessionID), quote(since.UTC().Format(time.RFC3339Nano))))
	if err != nil {
		return nil, err
	}
	var rows []messageRow
	if err := json.Unmarshal([]byte(emptyArray(out)), &rows); err != nil {
		return nil, err
	}
	msgs := make([]core.Message, 0, len(rows))
	for _, row := range rows {
		m, err := row.toCore()
		if err != nil {
			continue
		}
		msgs = append(msgs, m)
	}
	return msgs, nil
}

func (s Store) SaveSummary(payload core.SummaryPayload) (string, error) {
	id := "summary_" + randomHex(12)
	contentRef := filepath.ToSlash(filepath.Join("objects", "summaries", id+".json"))
	path := filepath.Join(filepath.Dir(s.DBPath), filepath.FromSlash(contentRef))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return id, nil
}

func (s Store) GetSummary(id string) (core.SummaryPayload, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return core.SummaryPayload{}, errors.New("summary id is required")
	}
	contentRef := filepath.ToSlash(filepath.Join("objects", "summaries", id+".json"))
	path := filepath.Join(filepath.Dir(s.DBPath), filepath.FromSlash(contentRef))
	data, err := os.ReadFile(path)
	if err != nil {
		return core.SummaryPayload{}, err
	}
	var payload core.SummaryPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return core.SummaryPayload{}, err
	}
	return payload, nil
}

func (s Store) ActiveProviderSegment(chatRunID string) (core.ProviderSegment, error) {
	chatRunID = strings.TrimSpace(chatRunID)
	if chatRunID == "" {
		return core.ProviderSegment{}, errors.New("chat run id is required")
	}
	out, err := s.queryJSON(fmt.Sprintf(`
SELECT id, chat_run_id, provider_id, route_id, model_id, external_thread_id, status,
       handoff_reason, started_at, COALESCE(completed_at, '') AS completed_at
FROM provider_segments
WHERE chat_run_id = %s AND status = %s
ORDER BY started_at DESC, id DESC
LIMIT 1;
`, quote(chatRunID), quote(string(core.ProviderSegmentRunning))))
	if err != nil {
		return core.ProviderSegment{}, err
	}
	var rows []providerSegmentRow
	if err := json.Unmarshal([]byte(emptyArray(out)), &rows); err != nil {
		return core.ProviderSegment{}, err
	}
	if len(rows) == 0 {
		return core.ProviderSegment{}, ErrNotFound
	}
	return rows[0].toCore()
}

func (s Store) SaveContextPacket(packet core.ContextPacket) (ContextPacketRecord, error) {
	packet.ID = strings.TrimSpace(packet.ID)
	packet.ProjectID = strings.TrimSpace(packet.ProjectID)
	packet.SessionID = strings.TrimSpace(packet.SessionID)
	packet.BranchID = strings.TrimSpace(packet.BranchID)
	packet.HeadEventID = strings.TrimSpace(packet.HeadEventID)
	if packet.ID == "" {
		packet.ID = "context_packet_" + randomHex(16)
	}
	if packet.SessionID == "" {
		return ContextPacketRecord{}, errors.New("session id is required")
	}
	if packet.BranchID == "" {
		packet.BranchID = "main"
	}
	if packet.ProjectID == "" {
		if session, _, err := s.GetSession(packet.SessionID); err == nil {
			packet.ProjectID = session.ProjectID
		}
	}
	if packet.CreatedAt.IsZero() {
		packet.CreatedAt = time.Now().UTC()
	}
	contentRef := filepath.ToSlash(filepath.Join("objects", "context-packets", packet.ID+".json"))
	if err := s.writeContextPacketObject(contentRef, packet); err != nil {
		return ContextPacketRecord{}, err
	}
	stmt := fmt.Sprintf(`
INSERT INTO context_packets (id, project_id, session_id, branch_id, head_event_id, user_input, content_ref, reference_count, created_at)
VALUES (%s, %s, %s, %s, %s, %s, %s, %d, %s);
`, quote(packet.ID), nullable(packet.ProjectID), quote(packet.SessionID), quote(packet.BranchID), nullable(packet.HeadEventID), quote(packet.UserInput), quote(contentRef), len(packet.References), quote(packet.CreatedAt.Format(time.RFC3339Nano)))
	if err := s.run(stmt); err != nil {
		return ContextPacketRecord{}, err
	}
	return s.getContextPacketRecord(packet.ID)
}

func (s Store) writeContextPacketObject(contentRef string, packet core.ContextPacket) error {
	if strings.TrimSpace(s.DBPath) == "" {
		return nil
	}
	path := filepath.Join(filepath.Dir(s.DBPath), filepath.FromSlash(contentRef))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(packet, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func (s Store) getContextPacketRecord(id string) (ContextPacketRecord, error) {
	out, err := s.queryJSON(fmt.Sprintf(`
SELECT id, COALESCE(project_id, '') AS project_id, session_id, branch_id,
       COALESCE(head_event_id, '') AS head_event_id, user_input, content_ref,
       reference_count, created_at
FROM context_packets
WHERE id = %s;
`, quote(id)))
	if err != nil {
		return ContextPacketRecord{}, err
	}
	var rows []contextPacketRow
	if err := json.Unmarshal([]byte(emptyArray(out)), &rows); err != nil {
		return ContextPacketRecord{}, err
	}
	if len(rows) == 0 {
		return ContextPacketRecord{}, ErrNotFound
	}
	return rows[0].toCore()
}

func (s Store) RecordSteering(input SteeringInput) (SteeringRecord, error) {
	input.ChatRunID = strings.TrimSpace(input.ChatRunID)
	input.ProviderSegmentID = strings.TrimSpace(input.ProviderSegmentID)
	input.EventID = strings.TrimSpace(input.EventID)
	input.Content = strings.TrimSpace(input.Content)
	input.Status = strings.TrimSpace(input.Status)
	if input.ChatRunID == "" {
		return SteeringRecord{}, errors.New("chat run id is required")
	}
	if input.Content == "" {
		return SteeringRecord{}, errors.New("steering content is required")
	}
	if input.Status == "" {
		input.Status = "recorded"
	}
	id := "steering_" + randomHex(16)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	stmt := fmt.Sprintf(`
INSERT INTO steering_events (id, chat_run_id, provider_segment_id, event_id, content, status, created_at)
VALUES (%s, %s, %s, %s, %s, %s, %s);
`, quote(id), quote(input.ChatRunID), nullable(input.ProviderSegmentID), nullable(input.EventID), quote(input.Content), quote(input.Status), quote(now))
	if err := s.run(stmt); err != nil {
		return SteeringRecord{}, err
	}
	return s.getSteeringRecord(id)
}

func (s Store) AddQueueItem(input QueueItemInput) (core.QueueItem, error) {
	input.SessionID = strings.TrimSpace(input.SessionID)
	input.BranchID = strings.TrimSpace(input.BranchID)
	input.Content = strings.TrimSpace(input.Content)
	input.RouteID = strings.TrimSpace(input.RouteID)
	input.ModelID = strings.TrimSpace(input.ModelID)
	input.ThinkingEffort = strings.TrimSpace(input.ThinkingEffort)
	if input.SessionID == "" {
		return core.QueueItem{}, errors.New("session id is required")
	}
	if input.Content == "" {
		return core.QueueItem{}, errors.New("queue item content is required")
	}
	if input.BranchID == "" {
		input.BranchID = "main"
	}
	if input.Mode == "" {
		input.Mode = core.QueueItemNormal
	}
	orderIndex, err := s.nextQueueOrder(input.SessionID, input.BranchID)
	if err != nil {
		return core.QueueItem{}, err
	}
	id := "queue_" + randomHex(16)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	stmt := fmt.Sprintf(`
INSERT INTO chat_queue_items (id, session_id, branch_id, content, mode, status, order_index, route_id, model_id, thinking_effort, created_at, updated_at)
VALUES (%s, %s, %s, %s, %s, %s, %d, %s, %s, %s, %s, %s);
`, quote(id), quote(input.SessionID), quote(input.BranchID), quote(input.Content), quote(string(input.Mode)), quote(string(core.QueueItemPending)), orderIndex, quote(input.RouteID), quote(input.ModelID), quote(input.ThinkingEffort), quote(now), quote(now))
	if err := s.run(stmt); err != nil {
		return core.QueueItem{}, err
	}
	return s.getQueueItem(id)
}

func (s Store) ListPendingQueueItems(sessionID string) ([]core.QueueItem, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, errors.New("session id is required")
	}
	out, err := s.queryJSON(fmt.Sprintf(`
SELECT id, session_id, branch_id, content, mode, status, order_index, route_id, model_id, thinking_effort, created_at, updated_at
FROM chat_queue_items
WHERE session_id = %s AND status = %s
ORDER BY order_index ASC, created_at ASC, id ASC;
`, quote(sessionID), quote(string(core.QueueItemPending))))
	if err != nil {
		return nil, err
	}
	var rows []queueItemRow
	if err := json.Unmarshal([]byte(emptyArray(out)), &rows); err != nil {
		return nil, err
	}
	items := make([]core.QueueItem, 0, len(rows))
	for _, row := range rows {
		item, err := row.toCore()
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (s Store) UpdateQueueItemStatus(id string, status core.QueueItemStatus) (core.QueueItem, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return core.QueueItem{}, errors.New("queue item id is required")
	}
	if status == "" {
		status = core.QueueItemConsumed
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	stmt := fmt.Sprintf(`UPDATE chat_queue_items
SET status = %s, updated_at = %s
WHERE id = %s;
`, quote(string(status)), quote(now), quote(id))
	if err := s.run(stmt); err != nil {
		return core.QueueItem{}, err
	}
	return s.getQueueItem(id)
}

func (s Store) ReorderQueueItems(sessionID string, itemIDs []string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return errors.New("session id is required")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	var sql strings.Builder
	sql.WriteString("BEGIN;\n")
	for index, id := range compactStrings(itemIDs) {
		fmt.Fprintf(&sql, `UPDATE chat_queue_items
SET order_index = %d, updated_at = %s
WHERE id = %s AND session_id = %s AND status = %s;
`, index, quote(now), quote(id), quote(sessionID), quote(string(core.QueueItemPending)))
	}
	sql.WriteString("COMMIT;\n")
	return s.run(sql.String())
}

func (s Store) nextQueueOrder(sessionID string, branchID string) (int, error) {
	out, err := s.queryJSON(fmt.Sprintf(`
SELECT COALESCE(MAX(order_index), -1) + 1 AS next_ordinal
FROM chat_queue_items
WHERE session_id = %s AND branch_id = %s AND status = %s;
`, quote(sessionID), quote(branchID), quote(string(core.QueueItemPending))))
	if err != nil {
		return 0, err
	}
	var rows []nextOrdinalRow
	if err := json.Unmarshal([]byte(emptyArray(out)), &rows); err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	return rows[0].NextOrdinal, nil
}

func (s Store) getQueueItem(id string) (core.QueueItem, error) {
	out, err := s.queryJSON(fmt.Sprintf(`
SELECT id, session_id, branch_id, content, mode, status, order_index, route_id, model_id, thinking_effort, created_at, updated_at
FROM chat_queue_items
WHERE id = %s;
`, quote(id)))
	if err != nil {
		return core.QueueItem{}, err
	}
	var rows []queueItemRow
	if err := json.Unmarshal([]byte(emptyArray(out)), &rows); err != nil {
		return core.QueueItem{}, err
	}
	if len(rows) == 0 {
		return core.QueueItem{}, ErrNotFound
	}
	return rows[0].toCore()
}

// ConsumeNextQueueItem atomically marks the next pending queue item as consumed,
// but only when no active chat run exists for the session. Returns ErrNotFound
// if there is nothing to consume or an active run is present.
func (s Store) ConsumeNextQueueItem(sessionID string) (core.QueueItem, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return core.QueueItem{}, errors.New("session id is required")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	stmt := fmt.Sprintf(`
UPDATE chat_queue_items
SET status = %s, updated_at = %s
WHERE id = (
  SELECT q.id FROM chat_queue_items q
  WHERE q.session_id = %s AND q.status = %s
  AND NOT EXISTS (
    SELECT 1 FROM chat_runs cr
    WHERE cr.session_id = %s AND cr.status IN (%s, %s)
  )
  ORDER BY q.order_index ASC, q.created_at ASC, q.id ASC
  LIMIT 1
)
RETURNING id;
`,
		quote(string(core.QueueItemConsumed)), quote(now),
		quote(sessionID), quote(string(core.QueueItemPending)),
		quote(sessionID),
		quote(string(core.ChatRunRunning)), quote(string(core.ChatRunWaitingApproval)),
	)
	out, err := s.queryJSON(stmt)
	if err != nil {
		return core.QueueItem{}, err
	}
	var rows []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(emptyArray(out)), &rows); err != nil {
		return core.QueueItem{}, err
	}
	if len(rows) == 0 {
		return core.QueueItem{}, ErrNotFound
	}
	return s.getQueueItem(rows[0].ID)
}

func (s Store) AddCandidateOutput(input CandidateOutputInput) (core.CandidateOutput, error) {
	input.ChatRunID = strings.TrimSpace(input.ChatRunID)
	input.SessionID = strings.TrimSpace(input.SessionID)
	input.BranchID = strings.TrimSpace(input.BranchID)
	input.TriggerEventID = strings.TrimSpace(input.TriggerEventID)
	input.Content = strings.TrimSpace(input.Content)
	if input.ChatRunID == "" {
		return core.CandidateOutput{}, errors.New("chat run id is required")
	}
	if input.SessionID == "" {
		return core.CandidateOutput{}, errors.New("session id is required")
	}
	if input.Content == "" && input.Status != core.CandidateOutputPending {
		return core.CandidateOutput{}, errors.New("candidate content is required")
	}
	if input.BranchID == "" {
		input.BranchID = "main"
	}
	if input.Status == "" {
		input.Status = core.CandidateOutputReady
	}
	id := "candidate_" + randomHex(16)
	contentRef := filepath.ToSlash(filepath.Join("objects", "candidates", id+".json"))
	if err := s.writeCandidateObject(contentRef, map[string]any{
		"id":             id,
		"chatRunId":      input.ChatRunID,
		"sessionId":      input.SessionID,
		"branchId":       input.BranchID,
		"triggerEventId": input.TriggerEventID,
		"content":        input.Content,
		"status":         input.Status,
		"recordedAt":     time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		return core.CandidateOutput{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	stmt := fmt.Sprintf(`
INSERT INTO candidate_outputs (id, chat_run_id, session_id, branch_id, trigger_event_id, content_ref, status, created_at, updated_at)
VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s);
`, quote(id), quote(input.ChatRunID), quote(input.SessionID), quote(input.BranchID), quote(input.TriggerEventID), quote(contentRef), quote(string(input.Status)), quote(now), quote(now))
	if err := s.run(stmt); err != nil {
		return core.CandidateOutput{}, err
	}
	return s.getCandidateOutput(id)
}

func (s Store) ListPendingCandidateOutputs(sessionID string) ([]core.CandidateOutput, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, errors.New("session id is required")
	}
	out, err := s.queryJSON(fmt.Sprintf(`
SELECT id, chat_run_id, session_id, branch_id, COALESCE(trigger_event_id, '') AS trigger_event_id, content_ref, status, created_at, updated_at
FROM candidate_outputs
WHERE session_id = %s AND status NOT IN (%s, %s, %s, %s)
ORDER BY created_at ASC;
`, quote(sessionID), quote(string(core.CandidateOutputAccepted)), quote(string(core.CandidateOutputRejected)), quote(string(core.CandidateOutputMerged)), quote(string(core.CandidateOutputSuperseded))))
	if err != nil {
		return nil, err
	}
	var rows []candidateOutputRow
	if err := json.Unmarshal([]byte(emptyArray(out)), &rows); err != nil {
		return nil, err
	}
	items := make([]core.CandidateOutput, 0, len(rows))
	for _, row := range rows {
		item, err := row.toCore()
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (s Store) UpdateCandidateOutput(id string, content string, status core.CandidateOutputStatus) (core.CandidateOutput, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return core.CandidateOutput{}, errors.New("candidate output id is required")
	}
	if status == "" {
		status = core.CandidateOutputReady
	}
	existing, err := s.getCandidateOutput(id)
	if err != nil {
		return core.CandidateOutput{}, err
	}
	if err := s.writeCandidateObject(existing.ContentRef, map[string]any{
		"id":             id,
		"chatRunId":      existing.ChatRunID,
		"sessionId":      existing.SessionID,
		"branchId":       existing.BranchID,
		"triggerEventId": existing.TriggerEventID,
		"content":        content,
		"status":         string(status),
		"recordedAt":     time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		return core.CandidateOutput{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	stmt := fmt.Sprintf(`UPDATE candidate_outputs
SET status = %s, updated_at = %s
WHERE id = %s;
`, quote(string(status)), quote(now), quote(id))
	if err := s.run(stmt); err != nil {
		return core.CandidateOutput{}, err
	}
	return s.getCandidateOutput(id)
}

func (s Store) UpdateQueueItemMode(id string, mode core.QueueItemMode) (core.QueueItem, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return core.QueueItem{}, errors.New("queue item id is required")
	}
	if mode == "" {
		mode = core.QueueItemNormal
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	stmt := fmt.Sprintf(`UPDATE chat_queue_items
SET mode = %s, updated_at = %s
WHERE id = %s;
`, quote(string(mode)), quote(now), quote(id))
	if err := s.run(stmt); err != nil {
		return core.QueueItem{}, err
	}
	return s.getQueueItem(id)
}

func (s Store) UpdateCandidateOutputStatus(id string, status core.CandidateOutputStatus) (core.CandidateOutput, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return core.CandidateOutput{}, errors.New("candidate output id is required")
	}
	if status == "" {
		status = core.CandidateOutputReady
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	stmt := fmt.Sprintf(`UPDATE candidate_outputs
SET status = %s, updated_at = %s
WHERE id = %s;
`, quote(string(status)), quote(now), quote(id))
	if err := s.run(stmt); err != nil {
		return core.CandidateOutput{}, err
	}
	return s.getCandidateOutput(id)
}

func (s Store) MergeCandidateOutput(id string) (CandidateMergeResult, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return CandidateMergeResult{}, errors.New("candidate output id is required")
	}
	candidate, err := s.getCandidateOutput(id)
	if err != nil {
		return CandidateMergeResult{}, err
	}
	if candidate.Status != core.CandidateOutputReady && candidate.Status != core.CandidateOutputAccepted {
		return CandidateMergeResult{}, fmt.Errorf("candidate must be ready before merge; current status is %s", candidate.Status)
	}
	content, err := s.readCandidateContent(candidate.ContentRef)
	if err != nil {
		return CandidateMergeResult{}, err
	}
	if strings.TrimSpace(content) == "" {
		return CandidateMergeResult{}, errors.New("candidate content is empty")
	}

	message, err := s.AddMessage(candidate.SessionID, "assistant", content)
	if err != nil {
		return CandidateMergeResult{}, err
	}
	session, _, err := s.GetSession(candidate.SessionID)
	if err != nil {
		return CandidateMergeResult{}, err
	}
	head, err := s.GetHead(session.ProjectID, session.ID, candidate.BranchID)
	if err != nil {
		return CandidateMergeResult{}, err
	}
	event, err := s.AppendEvent(core.EventInput{
		Type:           core.EventCandidateMerged,
		ProjectID:      session.ProjectID,
		SessionID:      session.ID,
		BranchID:       candidate.BranchID,
		ParentEventIDs: []string{head.EventID},
		PayloadRef:     fmt.Sprintf("candidate:%s|message:%s|chat_run:%s", candidate.ID, message.ID, candidate.ChatRunID),
	})
	if err != nil {
		return CandidateMergeResult{}, err
	}
	if _, err := s.MoveHead(session.ProjectID, session.ID, candidate.BranchID, event.ID); err != nil {
		return CandidateMergeResult{}, err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	sql := fmt.Sprintf(`BEGIN;
UPDATE candidate_outputs
SET status = %s, updated_at = %s
WHERE id = %s;
`, quote(string(core.CandidateOutputMerged)), quote(now), quote(candidate.ID))
	if candidate.TriggerEventID != "" {
		sql += fmt.Sprintf(`UPDATE candidate_outputs
SET status = %s, updated_at = %s
WHERE session_id = %s
  AND trigger_event_id = %s
  AND status = %s
  AND id != %s;
`, quote(string(core.CandidateOutputSuperseded)), quote(now), quote(candidate.SessionID), quote(candidate.TriggerEventID), quote(string(core.CandidateOutputReady)), quote(candidate.ID))
	}
	sql += "COMMIT;\n"
	if err := s.run(sql); err != nil {
		return CandidateMergeResult{}, err
	}

	merged, err := s.getCandidateOutput(candidate.ID)
	if err != nil {
		return CandidateMergeResult{}, err
	}
	supersededIDs, err := s.listSupersededCandidateIDs(candidate.SessionID, candidate.TriggerEventID, candidate.ID)
	if err != nil {
		return CandidateMergeResult{}, err
	}
	return CandidateMergeResult{
		Candidate:              merged,
		Message:                message,
		Event:                  event,
		SupersededCandidateIDs: supersededIDs,
	}, nil
}

func (s Store) writeCandidateObject(contentRef string, payload any) error {
	if strings.TrimSpace(s.DBPath) == "" {
		return nil
	}
	path := filepath.Join(filepath.Dir(s.DBPath), filepath.FromSlash(contentRef))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func (s Store) readCandidateContent(contentRef string) (string, error) {
	contentRef = strings.TrimSpace(contentRef)
	if contentRef == "" {
		return "", errors.New("candidate content ref is required")
	}
	path := filepath.Join(filepath.Dir(s.DBPath), filepath.FromSlash(contentRef))
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var payload struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", err
	}
	return payload.Content, nil
}

func (s Store) getCandidateOutput(id string) (core.CandidateOutput, error) {
	out, err := s.queryJSON(fmt.Sprintf(`
SELECT id, chat_run_id, session_id, branch_id, COALESCE(trigger_event_id, '') AS trigger_event_id, content_ref, status, created_at, updated_at
FROM candidate_outputs
WHERE id = %s;
`, quote(id)))
	if err != nil {
		return core.CandidateOutput{}, err
	}
	var rows []candidateOutputRow
	if err := json.Unmarshal([]byte(emptyArray(out)), &rows); err != nil {
		return core.CandidateOutput{}, err
	}
	if len(rows) == 0 {
		return core.CandidateOutput{}, ErrNotFound
	}
	return rows[0].toCore()
}

func (s Store) listSupersededCandidateIDs(sessionID string, triggerEventID string, excludeID string) ([]string, error) {
	sessionID = strings.TrimSpace(sessionID)
	triggerEventID = strings.TrimSpace(triggerEventID)
	excludeID = strings.TrimSpace(excludeID)
	if sessionID == "" || triggerEventID == "" {
		return nil, nil
	}
	out, err := s.queryJSON(fmt.Sprintf(`
SELECT id
FROM candidate_outputs
WHERE session_id = %s
  AND trigger_event_id = %s
  AND status = %s
  AND id != %s
ORDER BY updated_at ASC, id ASC;
`, quote(sessionID), quote(triggerEventID), quote(string(core.CandidateOutputSuperseded)), quote(excludeID)))
	if err != nil {
		return nil, err
	}
	var rows []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(emptyArray(out)), &rows); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	return ids, nil
}

func (s Store) getSteeringRecord(id string) (SteeringRecord, error) {
	out, err := s.queryJSON(fmt.Sprintf(`
SELECT id, chat_run_id, COALESCE(provider_segment_id, '') AS provider_segment_id,
       COALESCE(event_id, '') AS event_id, content, status, created_at
FROM steering_events
WHERE id = %s;
`, quote(id)))
	if err != nil {
		return SteeringRecord{}, err
	}
	var rows []steeringRow
	if err := json.Unmarshal([]byte(emptyArray(out)), &rows); err != nil {
		return SteeringRecord{}, err
	}
	if len(rows) == 0 {
		return SteeringRecord{}, ErrNotFound
	}
	return rows[0].toCore()
}

func (s Store) getChatRun(id string) (core.ChatRun, error) {
	out, err := s.queryJSON(fmt.Sprintf(`
SELECT id, COALESCE(project_id, '') AS project_id, session_id, branch_id, role, status,
       COALESCE(input_event_id, '') AS input_event_id,
       COALESCE(output_event_id, '') AS output_event_id,
       context_packet_id, created_at, updated_at
FROM chat_runs
WHERE id = %s;
`, quote(id)))
	if err != nil {
		return core.ChatRun{}, err
	}
	var rows []chatRunRow
	if err := json.Unmarshal([]byte(emptyArray(out)), &rows); err != nil {
		return core.ChatRun{}, err
	}
	if len(rows) == 0 {
		return core.ChatRun{}, ErrNotFound
	}
	return rows[0].toCore()
}

func (s Store) getProviderSegment(id string) (core.ProviderSegment, error) {
	out, err := s.queryJSON(fmt.Sprintf(`
SELECT id, chat_run_id, provider_id, route_id, model_id, external_thread_id, status,
       handoff_reason, started_at, COALESCE(completed_at, '') AS completed_at
FROM provider_segments
WHERE id = %s;
`, quote(id)))
	if err != nil {
		return core.ProviderSegment{}, err
	}
	var rows []providerSegmentRow
	if err := json.Unmarshal([]byte(emptyArray(out)), &rows); err != nil {
		return core.ProviderSegment{}, err
	}
	if len(rows) == 0 {
		return core.ProviderSegment{}, ErrNotFound
	}
	return rows[0].toCore()
}

func (s Store) CreateBranch(sessionID string, fromMessageID string) (core.Session, error) {
	session, messages, err := s.GetSession(sessionID)
	if err != nil {
		return core.Session{}, err
	}

	fromOrdinal := -1
	for i, message := range messages {
		if message.ID == fromMessageID {
			fromOrdinal = i
			break
		}
	}
	if fromOrdinal == -1 {
		return core.Session{}, fmt.Errorf("message %q: %w", fromMessageID, ErrNotFound)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	branchSessionID := "session_" + randomHex(16)
	branchID := "branch_" + randomHex(16)
	title := session.Title + " branch"

	var sql bytes.Buffer
	fmt.Fprintf(&sql, "BEGIN;\n")
	fmt.Fprintf(&sql, `INSERT INTO sessions (id, project_id, source_tool, source_id, title, created_at, updated_at, parent_session_id, branch_from_message_id)
VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s);
`, quote(branchSessionID), nullable(session.ProjectID), quote(string(session.SourceTool)), quote(branchSessionID), quote(title), quote(now), quote(now), quote(session.ID), quote(fromMessageID))

	for i, message := range messages[:fromOrdinal+1] {
		fmt.Fprintf(&sql, `INSERT INTO messages (id, session_id, source_id, role, content, created_at, ordinal)
VALUES (%s, %s, %s, %s, %s, %s, %d);
`, quote("message_"+randomHex(16)), quote(branchSessionID), nullable(message.SourceID), quote(message.Role), quote(message.Content), quote(message.CreatedAt.Format(time.RFC3339Nano)), i)
	}

	fmt.Fprintf(&sql, `INSERT INTO branches (id, parent_session_id, session_id, from_message_id)
VALUES (%s, %s, %s, %s);
`, quote(branchID), quote(session.ID), quote(branchSessionID), quote(fromMessageID))
	fmt.Fprintf(&sql, "COMMIT;\n")

	if err := s.run(sql.String()); err != nil {
		return core.Session{}, err
	}

	branch, _, err := s.GetSession(branchSessionID)
	return branch, err
}

func (s Store) ListProviderPlugins() ([]RegistryItem, error) {
	return s.listRegistry(`
SELECT id, display_name, kind, enabled
FROM provider_plugins
ORDER BY id ASC;
`)
}

func (s Store) ListAgentPlugins() ([]RegistryItem, error) {
	return s.listRegistry(`
SELECT id, display_name, CASE uses_ai WHEN 1 THEN 'ai' ELSE 'local' END AS kind, enabled
FROM agent_plugins
ORDER BY id ASC;
`)
}

func (s Store) ListCapabilities() ([]RegistryItem, error) {
	return s.listRegistry(`
SELECT id, display_name, kind, enabled
FROM capabilities
ORDER BY id ASC;
`)
}

func (s Store) ListTools() ([]RegistryItem, error) {
	return s.listRegistry(`
SELECT id, display_name, kind, enabled
FROM tool_registry
ORDER BY id ASC;
`)
}

func (s Store) ListProviderProfiles() ([]ProviderProfile, error) {
	out, err := s.queryJSON(`
SELECT id, provider_plugin_id, display_name, is_default
FROM provider_profiles
ORDER BY provider_plugin_id ASC, display_name ASC;
`)
	if err != nil {
		return nil, err
	}
	var rows []providerProfileRow
	if err := json.Unmarshal([]byte(emptyArray(out)), &rows); err != nil {
		return nil, err
	}
	profiles := make([]ProviderProfile, 0, len(rows))
	for _, row := range rows {
		profiles = append(profiles, ProviderProfile{
			ID:               row.ID,
			ProviderPluginID: row.ProviderPluginID,
			DisplayName:      row.DisplayName,
			IsDefault:        row.IsDefault != 0,
		})
	}
	return profiles, nil
}

func (s Store) UpsertProviderProfile(providerID string, displayName string) (ProviderProfile, error) {
	providerID = strings.TrimSpace(providerID)
	displayName = strings.TrimSpace(displayName)
	if providerID == "" {
		return ProviderProfile{}, errors.New("provider id is required")
	}
	if displayName == "" {
		displayName = providerID + " account"
	}

	profileID := "profile_" + providerID + "_local"
	stmt := fmt.Sprintf(`
UPDATE provider_profiles
SET is_default = 0
WHERE provider_plugin_id = %s;

INSERT INTO provider_profiles (id, provider_plugin_id, display_name, credential_ref, is_default)
VALUES (%s, %s, %s, %s, 1)
ON CONFLICT(id) DO UPDATE SET
  display_name = excluded.display_name,
  credential_ref = excluded.credential_ref,
  is_default = 1;
`, quote(providerID), quote(profileID), quote(providerID), quote(displayName), quote("local-account:"+providerID))

	if err := s.run(stmt); err != nil {
		return ProviderProfile{}, err
	}
	profiles, err := s.ListProviderProfiles()
	if err != nil {
		return ProviderProfile{}, err
	}
	for _, profile := range profiles {
		if profile.ID == profileID {
			return profile, nil
		}
	}
	return ProviderProfile{}, ErrNotFound
}

func (s Store) ListProviderModels() ([]ProviderModel, error) {
	out, err := s.queryJSON(`
SELECT id, provider_plugin_id, display_name, model_ref, status, is_default
FROM provider_models
ORDER BY provider_plugin_id ASC, is_default DESC, display_name ASC;
`)
	if err != nil {
		return nil, err
	}
	var rows []providerModelRow
	if err := json.Unmarshal([]byte(emptyArray(out)), &rows); err != nil {
		return nil, err
	}
	models := make([]ProviderModel, 0, len(rows))
	for _, row := range rows {
		models = append(models, ProviderModel{
			ID:               row.ID,
			ProviderPluginID: row.ProviderPluginID,
			DisplayName:      row.DisplayName,
			ModelRef:         row.ModelRef,
			Status:           row.Status,
			IsDefault:        row.IsDefault != 0,
		})
	}
	return models, nil
}

func (s Store) ListAccessRoutes() ([]AccessRoute, error) {
	out, err := s.queryJSON(`
SELECT id, provider_plugin_id, display_name, access_mode, transport,
       requires_license, requires_api_key, supports_streaming, supports_tools,
       supports_import, supports_handoff, cost_model, status,
       COALESCE(context_budget_chars, 0) AS context_budget_chars
FROM access_routes
ORDER BY provider_plugin_id ASC, access_mode ASC, id ASC;
`)
	if err != nil {
		return nil, err
	}
	var rows []accessRouteRow
	if err := json.Unmarshal([]byte(emptyArray(out)), &rows); err != nil {
		return nil, err
	}
	routes := make([]AccessRoute, 0, len(rows))
	for _, row := range rows {
		routes = append(routes, AccessRoute{
			ID:                 row.ID,
			ProviderPluginID:   row.ProviderPluginID,
			DisplayName:        row.DisplayName,
			AccessMode:         row.AccessMode,
			Transport:          row.Transport,
			RequiresLicense:    row.RequiresLicense != 0,
			RequiresAPIKey:     row.RequiresAPIKey != 0,
			SupportsStreaming:  row.SupportsStreaming != 0,
			SupportsTools:      row.SupportsTools != 0,
			SupportsImport:     row.SupportsImport != 0,
			SupportsHandoff:    row.SupportsHandoff != 0,
			CostModel:          row.CostModel,
			ContextBudgetChars: row.ContextBudgetChars,
			Status:             row.Status,
		})
	}
	return routes, nil
}

func (s Store) DefaultProject() (Project, error) {
	out, err := s.queryJSON(`
SELECT id, display_name, COALESCE(root_path, '') AS root_path, is_default, COALESCE(context_policy, 'flat-trim') AS context_policy, COALESCE(handoff_policy, 'route-change') AS handoff_policy, COALESCE(route_policy, 'manual') AS route_policy, COALESCE(tool_approval_policy, 'safe-only') AS tool_approval_policy, COALESCE(kb_scope_policy, 'project-only') AS kb_scope_policy
FROM projects
ORDER BY is_default DESC, created_at ASC
LIMIT 1;
`)
	if err != nil {
		return Project{}, err
	}
	var rows []projectRow
	if err := json.Unmarshal([]byte(emptyArray(out)), &rows); err != nil {
		return Project{}, err
	}
	if len(rows) == 0 {
		return Project{}, ErrNotFound
	}
	return rows[0].toCore(), nil
}

func (s Store) ListProjects() ([]Project, error) {
	out, err := s.queryJSON(`
SELECT id, display_name, COALESCE(root_path, '') AS root_path, is_default, COALESCE(context_policy, 'flat-trim') AS context_policy, COALESCE(handoff_policy, 'route-change') AS handoff_policy, COALESCE(route_policy, 'manual') AS route_policy, COALESCE(tool_approval_policy, 'safe-only') AS tool_approval_policy, COALESCE(kb_scope_policy, 'project-only') AS kb_scope_policy
FROM projects
ORDER BY is_default DESC, display_name ASC;
`)
	if err != nil {
		return nil, err
	}
	var rows []projectRow
	if err := json.Unmarshal([]byte(emptyArray(out)), &rows); err != nil {
		return nil, err
	}
	projects := make([]Project, 0, len(rows))
	for _, row := range rows {
		projects = append(projects, row.toCore())
	}
	return projects, nil
}

func (s Store) CreateProject(displayName string, rootPath string) (Project, error) {
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		return Project{}, errors.New("project name is required")
	}
	projectID := "project_" + randomHex(12)
	stmt := fmt.Sprintf(`INSERT INTO projects (id, display_name, root_path, is_default)
VALUES (%s, %s, %s, 0);
`, quote(projectID), quote(displayName), nullable(strings.TrimSpace(rootPath)))
	if err := s.run(stmt); err != nil {
		return Project{}, err
	}
	out, err := s.queryJSON(fmt.Sprintf(`
SELECT id, display_name, COALESCE(root_path, '') AS root_path, is_default, COALESCE(context_policy, 'flat-trim') AS context_policy, COALESCE(handoff_policy, 'route-change') AS handoff_policy, COALESCE(route_policy, 'manual') AS route_policy, COALESCE(tool_approval_policy, 'safe-only') AS tool_approval_policy, COALESCE(kb_scope_policy, 'project-only') AS kb_scope_policy
FROM projects
WHERE id = %s;
`, quote(projectID)))
	if err != nil {
		return Project{}, err
	}
	var rows []projectRow
	if err := json.Unmarshal([]byte(emptyArray(out)), &rows); err != nil {
		return Project{}, err
	}
	if len(rows) == 0 {
		return Project{}, ErrNotFound
	}
	return rows[0].toCore(), nil
}

func (s Store) RenameProject(projectID string, displayName string) (Project, error) {
	projectID = strings.TrimSpace(projectID)
	displayName = strings.TrimSpace(displayName)
	if projectID == "" {
		return Project{}, errors.New("project id is required")
	}
	if displayName == "" {
		return Project{}, errors.New("project name is required")
	}
	stmt := fmt.Sprintf(`UPDATE projects SET display_name = %s WHERE id = %s;
`, quote(displayName), quote(projectID))
	if err := s.run(stmt); err != nil {
		return Project{}, err
	}
	out, err := s.queryJSON(fmt.Sprintf(`
SELECT id, display_name, COALESCE(root_path, '') AS root_path, is_default, COALESCE(context_policy, 'flat-trim') AS context_policy, COALESCE(handoff_policy, 'route-change') AS handoff_policy, COALESCE(route_policy, 'manual') AS route_policy, COALESCE(tool_approval_policy, 'safe-only') AS tool_approval_policy, COALESCE(kb_scope_policy, 'project-only') AS kb_scope_policy
FROM projects
WHERE id = %s;
`, quote(projectID)))
	if err != nil {
		return Project{}, err
	}
	var rows []projectRow
	if err := json.Unmarshal([]byte(emptyArray(out)), &rows); err != nil {
		return Project{}, err
	}
	if len(rows) == 0 {
		return Project{}, ErrNotFound
	}
	return rows[0].toCore(), nil
}

// ProjectSettingsUpdate holds the policy fields that can be changed via
// PATCH /api/projects/{id}/settings. Empty string means "no change".
type ProjectSettingsUpdate struct {
	ContextPolicy      string
	HandoffPolicy      string
	RoutePolicy        string
	ToolApprovalPolicy string
	KbScopePolicy      string
}

func (s Store) UpdateProjectSettings(projectID string, u ProjectSettingsUpdate) (Project, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return Project{}, errors.New("project id is required")
	}
	setClauses := []string{}
	if u.ContextPolicy != "" {
		setClauses = append(setClauses, fmt.Sprintf("context_policy = %s", quote(u.ContextPolicy)))
	}
	if u.HandoffPolicy != "" {
		setClauses = append(setClauses, fmt.Sprintf("handoff_policy = %s", quote(u.HandoffPolicy)))
	}
	if u.RoutePolicy != "" {
		setClauses = append(setClauses, fmt.Sprintf("route_policy = %s", quote(u.RoutePolicy)))
	}
	if u.ToolApprovalPolicy != "" {
		setClauses = append(setClauses, fmt.Sprintf("tool_approval_policy = %s", quote(u.ToolApprovalPolicy)))
	}
	if u.KbScopePolicy != "" {
		setClauses = append(setClauses, fmt.Sprintf("kb_scope_policy = %s", quote(u.KbScopePolicy)))
	}
	if len(setClauses) > 0 {
		stmt := fmt.Sprintf("UPDATE projects SET %s WHERE id = %s;",
			strings.Join(setClauses, ", "), quote(projectID))
		if err := s.run(stmt); err != nil {
			return Project{}, err
		}
	}
	out, err := s.queryJSON(fmt.Sprintf(`
SELECT id, display_name, COALESCE(root_path, '') AS root_path, is_default, COALESCE(context_policy, 'flat-trim') AS context_policy, COALESCE(handoff_policy, 'route-change') AS handoff_policy, COALESCE(route_policy, 'manual') AS route_policy, COALESCE(tool_approval_policy, 'safe-only') AS tool_approval_policy, COALESCE(kb_scope_policy, 'project-only') AS kb_scope_policy
FROM projects
WHERE id = %s;
`, quote(projectID)))
	if err != nil {
		return Project{}, err
	}
	var rows []projectRow
	if err := json.Unmarshal([]byte(emptyArray(out)), &rows); err != nil {
		return Project{}, err
	}
	if len(rows) == 0 {
		return Project{}, ErrNotFound
	}
	return rows[0].toCore(), nil
}

func (s Store) GetProject(projectID string) (Project, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return Project{}, errors.New("project id is required")
	}
	out, err := s.queryJSON(fmt.Sprintf(`
SELECT id, display_name, COALESCE(root_path, '') AS root_path, is_default, COALESCE(context_policy, 'flat-trim') AS context_policy, COALESCE(handoff_policy, 'route-change') AS handoff_policy, COALESCE(route_policy, 'manual') AS route_policy, COALESCE(tool_approval_policy, 'safe-only') AS tool_approval_policy, COALESCE(kb_scope_policy, 'project-only') AS kb_scope_policy
FROM projects WHERE id = %s;
`, quote(projectID)))
	if err != nil {
		return Project{}, err
	}
	var rows []projectRow
	if err := json.Unmarshal([]byte(emptyArray(out)), &rows); err != nil {
		return Project{}, err
	}
	if len(rows) == 0 {
		return Project{}, ErrNotFound
	}
	return rows[0].toCore(), nil
}

func (s Store) DeleteProject(projectID string) error {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return errors.New("project id is required")
	}
	out, err := s.queryJSON(fmt.Sprintf(`
SELECT id, display_name, COALESCE(root_path, '') AS root_path, is_default, COALESCE(context_policy, 'flat-trim') AS context_policy, COALESCE(handoff_policy, 'route-change') AS handoff_policy, COALESCE(route_policy, 'manual') AS route_policy, COALESCE(tool_approval_policy, 'safe-only') AS tool_approval_policy, COALESCE(kb_scope_policy, 'project-only') AS kb_scope_policy
FROM projects
WHERE id = %s;
`, quote(projectID)))
	if err != nil {
		return err
	}
	var rows []projectRow
	if err := json.Unmarshal([]byte(emptyArray(out)), &rows); err != nil {
		return err
	}
	if len(rows) == 0 {
		return ErrNotFound
	}
	if rows[0].IsDefault != 0 {
		return errors.New("default project cannot be deleted")
	}

	sessionSubquery := fmt.Sprintf("SELECT id FROM sessions WHERE COALESCE(project_id, 'default') = %s", quote(projectID))
	stmt := fmt.Sprintf(`BEGIN;
DELETE FROM branches WHERE parent_session_id IN (%s) OR session_id IN (%s);
UPDATE sessions SET parent_session_id = NULL, branch_from_message_id = NULL WHERE parent_session_id IN (%s);
DELETE FROM message_events WHERE session_id IN (%s);
DELETE FROM messages WHERE session_id IN (%s);
DELETE FROM session_provider_groups WHERE session_id IN (%s);
DELETE FROM provider_chats WHERE session_id IN (%s);
DELETE FROM token_ledger WHERE session_id IN (%s);
UPDATE command_runs SET session_id = NULL WHERE session_id IN (%s);
UPDATE agent_runs SET session_id = NULL WHERE session_id IN (%s);
DELETE FROM sessions WHERE COALESCE(project_id, 'default') = %s;
DELETE FROM project_access_routes WHERE project_id = %s;
DELETE FROM moderator_preferences WHERE scope = 'project' AND project_id = %s;
DELETE FROM projects WHERE id = %s;
COMMIT;
`, sessionSubquery, sessionSubquery, sessionSubquery, sessionSubquery, sessionSubquery, sessionSubquery, sessionSubquery, sessionSubquery, sessionSubquery, sessionSubquery, quote(projectID), quote(projectID), quote(projectID), quote(projectID))
	return s.run(stmt)
}

func (s Store) SearchSessions(query string) ([]core.Session, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return s.ListSessions()
	}
	like := "%" + query + "%"
	out, err := s.queryJSON(fmt.Sprintf(`
SELECT DISTINCT s.id, s.source_tool, s.source_id, s.title, s.created_at, s.updated_at,
       COALESCE(s.parent_session_id, '') AS parent_id,
       COALESCE(s.branch_from_message_id, '') AS branch_from_id
FROM sessions s
LEFT JOIN messages m ON m.session_id = s.id
WHERE s.title LIKE %s OR s.source_tool LIKE %s OR m.content LIKE %s
ORDER BY s.updated_at DESC, s.id ASC;
`, quote(like), quote(like), quote(like)))
	if err != nil {
		return nil, err
	}
	var rows []sessionRow
	if err := json.Unmarshal([]byte(emptyArray(out)), &rows); err != nil {
		return nil, err
	}
	sessions := make([]core.Session, 0, len(rows))
	for _, row := range rows {
		session, err := row.toCore()
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}
	return sessions, nil
}

func (s Store) AddKnowledgeItem(item core.KnowledgeItem) (core.KnowledgeItem, error) {
	item.Scope = core.KnowledgeScope(strings.TrimSpace(string(item.Scope)))
	item.ProjectID = strings.TrimSpace(item.ProjectID)
	item.Kind = strings.TrimSpace(item.Kind)
	item.Title = strings.TrimSpace(item.Title)
	item.SourceEventID = strings.TrimSpace(item.SourceEventID)
	if item.Scope == "" {
		if item.ProjectID != "" {
			item.Scope = core.KnowledgeScopeProject
		} else {
			item.Scope = core.KnowledgeScopeGlobal
		}
	}
	if item.Scope == core.KnowledgeScopeProject && item.ProjectID == "" {
		return core.KnowledgeItem{}, errors.New("project knowledge requires project id")
	}
	if item.Kind == "" {
		item.Kind = "fact"
	}
	if item.Title == "" {
		return core.KnowledgeItem{}, errors.New("knowledge title is required")
	}
	if item.ID == "" {
		item.ID = "knowledge_" + randomHex(16)
	}
	now := time.Now().UTC()
	if item.CreatedAt.IsZero() {
		item.CreatedAt = now
	}
	item.UpdatedAt = now
	if item.ContentRef == "" {
		item.ContentRef = filepath.ToSlash(filepath.Join("objects", "knowledge", item.ID+".json"))
	}
	if err := s.writeKnowledgeObject(item); err != nil {
		return core.KnowledgeItem{}, err
	}
	stmt := fmt.Sprintf(`
INSERT INTO knowledge_items (id, scope, project_id, kind, title, source_event_id, content_ref, created_at, updated_at)
VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s);
`, quote(item.ID), quote(string(item.Scope)), nullable(item.ProjectID), quote(item.Kind), quote(item.Title), nullable(item.SourceEventID), quote(item.ContentRef), quote(item.CreatedAt.Format(time.RFC3339Nano)), quote(item.UpdatedAt.Format(time.RFC3339Nano)))
	if err := s.run(stmt); err != nil {
		return core.KnowledgeItem{}, err
	}
	return item, nil
}

func (s Store) SearchKnowledge(options KnowledgeSearchOptions) ([]core.KnowledgeItem, error) {
	limit := options.Limit
	if limit <= 0 {
		limit = 20
	}
	clauses := []string{"1 = 1"}
	if strings.TrimSpace(options.Query) != "" {
		like := "%" + strings.TrimSpace(options.Query) + "%"
		clauses = append(clauses, fmt.Sprintf("(title LIKE %s OR kind LIKE %s OR content_ref LIKE %s)", quote(like), quote(like), quote(like)))
	}
	if options.Scope != "" {
		clauses = append(clauses, fmt.Sprintf("scope = %s", quote(string(options.Scope))))
	}
	if strings.TrimSpace(options.ProjectID) != "" {
		clauses = append(clauses, fmt.Sprintf("COALESCE(project_id, '') = %s", quote(strings.TrimSpace(options.ProjectID))))
	}
	out, err := s.queryJSON(fmt.Sprintf(`
SELECT id, scope, COALESCE(project_id, '') AS project_id, kind, title,
       COALESCE(source_event_id, '') AS source_event_id, content_ref, created_at, updated_at
FROM knowledge_items
WHERE %s
ORDER BY updated_at DESC, id ASC
LIMIT %d;
`, strings.Join(clauses, " AND "), limit))
	if err != nil {
		return nil, err
	}
	var rows []knowledgeItemRow
	if err := json.Unmarshal([]byte(emptyArray(out)), &rows); err != nil {
		return nil, err
	}
	items := make([]core.KnowledgeItem, 0, len(rows))
	for _, row := range rows {
		item, err := row.toCore()
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (s Store) writeKnowledgeObject(item core.KnowledgeItem) error {
	if strings.TrimSpace(s.DBPath) == "" {
		return nil
	}
	path := filepath.Join(filepath.Dir(s.DBPath), filepath.FromSlash(item.ContentRef))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func (s Store) AddCommandRun(input CommandRun) (CommandRun, error) {
	if strings.TrimSpace(input.Command) == "" {
		return CommandRun{}, errors.New("command is required")
	}
	if strings.TrimSpace(input.WorkingDir) == "" {
		return CommandRun{}, errors.New("working directory is required")
	}
	if strings.TrimSpace(input.Status) == "" {
		input.Status = "completed"
	}
	runID := "cmd_" + randomHex(16)
	startedAt := input.StartedAt
	if strings.TrimSpace(startedAt) == "" {
		startedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	finishedAt := input.FinishedAt
	if strings.TrimSpace(finishedAt) == "" {
		finishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	stmt := fmt.Sprintf(`INSERT INTO command_runs (
  id, project_id, session_id, command, working_dir, status, exit_code, stdout, stderr, started_at, finished_at
) VALUES (%s, %s, %s, %s, %s, %s, %d, %s, %s, %s, %s);
`, quote(runID), nullable(input.ProjectID), nullable(input.SessionID), quote(input.Command), quote(input.WorkingDir), quote(input.Status), input.ExitCode, quote(input.Stdout), quote(input.Stderr), quote(startedAt), quote(finishedAt))
	if err := s.run(stmt); err != nil {
		return CommandRun{}, err
	}
	input.ID = runID
	input.StartedAt = startedAt
	input.FinishedAt = finishedAt
	return input, nil
}

func (s Store) ListCommandRuns(limit int) ([]CommandRun, error) {
	if limit <= 0 {
		limit = 10
	}
	out, err := s.queryJSON(fmt.Sprintf(`
SELECT id, COALESCE(project_id, '') AS project_id, COALESCE(session_id, '') AS session_id,
       command, working_dir, status, COALESCE(exit_code, 0) AS exit_code,
       stdout, stderr, started_at, COALESCE(finished_at, '') AS finished_at
FROM command_runs
ORDER BY started_at DESC
LIMIT %d;
`, limit))
	if err != nil {
		return nil, err
	}
	var rows []commandRunRow
	if err := json.Unmarshal([]byte(emptyArray(out)), &rows); err != nil {
		return nil, err
	}
	runs := make([]CommandRun, 0, len(rows))
	for _, row := range rows {
		runs = append(runs, CommandRun{
			ID:         row.ID,
			ProjectID:  row.ProjectID,
			SessionID:  row.SessionID,
			Command:    row.Command,
			WorkingDir: row.WorkingDir,
			Status:     row.Status,
			ExitCode:   row.ExitCode,
			Stdout:     row.Stdout,
			Stderr:     row.Stderr,
			StartedAt:  row.StartedAt,
			FinishedAt: row.FinishedAt,
		})
	}
	return runs, nil
}

func (s Store) ListProjectAccessRoutes(projectID string) ([]ProjectAccessRoute, error) {
	out, err := s.queryJSON(fmt.Sprintf(`
SELECT par.project_id,
       par.enabled,
       par.priority,
       ar.id,
       ar.provider_plugin_id,
       ar.display_name,
       ar.access_mode,
       ar.transport,
       ar.requires_license,
       ar.requires_api_key,
       ar.supports_streaming,
       ar.supports_tools,
       ar.supports_import,
       ar.supports_handoff,
       ar.cost_model,
       ar.status,
       COALESCE(ar.context_budget_chars, 0) AS context_budget_chars
FROM project_access_routes par
JOIN access_routes ar ON ar.id = par.access_route_id
WHERE par.project_id = %s
ORDER BY par.priority ASC, ar.provider_plugin_id ASC, ar.id ASC;
`, quote(projectID)))
	if err != nil {
		return nil, err
	}
	var rows []projectAccessRouteRow
	if err := json.Unmarshal([]byte(emptyArray(out)), &rows); err != nil {
		return nil, err
	}
	routes := make([]ProjectAccessRoute, 0, len(rows))
	for _, row := range rows {
		routes = append(routes, ProjectAccessRoute{
			ProjectID: row.ProjectID,
			Route:     row.toAccessRoute(),
			Enabled:   row.Enabled != 0,
			Priority:  row.Priority,
		})
	}
	return routes, nil
}

func (s Store) AddProjectAccessRoute(projectID string, routeID string) error {
	if strings.TrimSpace(projectID) == "" {
		return errors.New("project id is required")
	}
	if strings.TrimSpace(routeID) == "" {
		return errors.New("access route id is required")
	}

	stmt := fmt.Sprintf(`
INSERT INTO project_access_routes (project_id, access_route_id, enabled, priority)
VALUES (%s, %s, 1, COALESCE((SELECT MAX(priority) + 10 FROM project_access_routes WHERE project_id = %s), 10))
ON CONFLICT(project_id, access_route_id) DO UPDATE SET enabled = 1;
`, quote(projectID), quote(routeID), quote(projectID))
	return s.run(stmt)
}

func (s Store) RemoveProjectAccessRoute(projectID string, routeID string) error {
	if strings.TrimSpace(projectID) == "" {
		return errors.New("project id is required")
	}
	if strings.TrimSpace(routeID) == "" {
		return errors.New("access route id is required")
	}

	stmt := fmt.Sprintf(`
DELETE FROM project_access_routes
WHERE project_id = %s AND access_route_id = %s;
`, quote(projectID), quote(routeID))
	return s.run(stmt)
}

func (s Store) ModeratorPreference(projectID string) (ModeratorPreference, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID != "" {
		if pref, err := s.moderatorPreferenceForScope("project", projectID); err == nil {
			pref.Source = "project"
			return pref, nil
		} else if !errors.Is(err, ErrNotFound) {
			return ModeratorPreference{}, err
		}
	}
	if pref, err := s.moderatorPreferenceForScope("global", ""); err == nil {
		pref.Source = "global"
		return pref, nil
	} else if !errors.Is(err, ErrNotFound) {
		return ModeratorPreference{}, err
	}
	return ModeratorPreference{Scope: "default", Mode: "auto", Source: "default"}, nil
}

func (s Store) UpsertProjectModeratorPreference(projectID string, mode string, primary string, secondary string) (ModeratorPreference, error) {
	projectID = strings.TrimSpace(projectID)
	mode = strings.TrimSpace(mode)
	if projectID == "" {
		return ModeratorPreference{}, errors.New("project id is required")
	}
	if mode == "" {
		mode = "auto"
	}
	if mode != "auto" && mode != "manual" {
		return ModeratorPreference{}, errors.New("moderator mode must be auto or manual")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	stmt := fmt.Sprintf(`
INSERT INTO moderator_preferences (
  scope, project_id, mode, primary_provider_group_id, secondary_provider_group_id, updated_at
) VALUES ('project', %s, %s, %s, %s, %s)
ON CONFLICT(scope, project_id) DO UPDATE SET
  mode = excluded.mode,
  primary_provider_group_id = excluded.primary_provider_group_id,
  secondary_provider_group_id = excluded.secondary_provider_group_id,
  updated_at = excluded.updated_at;
`, quote(projectID), quote(mode), nullable(primary), nullable(secondary), quote(now))
	if err := s.run(stmt); err != nil {
		return ModeratorPreference{}, err
	}
	return s.ModeratorPreference(projectID)
}

func (s Store) moderatorPreferenceForScope(scope string, projectID string) (ModeratorPreference, error) {
	out, err := s.queryJSON(fmt.Sprintf(`
SELECT scope,
       project_id,
       mode,
       COALESCE(primary_provider_group_id, '') AS primary_provider_group_id,
       COALESCE(secondary_provider_group_id, '') AS secondary_provider_group_id
FROM moderator_preferences
WHERE scope = %s AND project_id = %s;
`, quote(scope), quote(projectID)))
	if err != nil {
		return ModeratorPreference{}, err
	}
	var rows []moderatorPreferenceRow
	if err := json.Unmarshal([]byte(emptyArray(out)), &rows); err != nil {
		return ModeratorPreference{}, err
	}
	if len(rows) == 0 {
		return ModeratorPreference{}, ErrNotFound
	}
	return rows[0].toCore(), nil
}

func (s Store) AddTokenUsage(input TokenUsageInput) error {
	if input.ProviderPluginID == "" {
		return errors.New("provider plugin id is required")
	}
	if input.Model == "" {
		return errors.New("model is required")
	}
	if input.Status == "" {
		input.Status = "success"
	}

	usageID := "usage_" + randomHex(16)
	stmt := fmt.Sprintf(`INSERT INTO token_ledger (
  id, provider_plugin_id, provider_profile_id, agent_run_id, session_id,
  model, prompt_tokens, completion_tokens, status
) VALUES (%s, %s, %s, %s, %s, %s, %d, %d, %s);
`, quote(usageID), quote(input.ProviderPluginID), nullable(input.ProviderProfileID), nullable(input.AgentRunID), nullable(input.SessionID), quote(input.Model), input.PromptTokens, input.CompletionTokens, quote(input.Status))

	return s.run(stmt)
}

func (s Store) TokenUsageSummary() ([]TokenUsageSummary, error) {
	out, err := s.queryJSON(`
SELECT provider_plugin_id,
       COALESCE(provider_profile_id, '') AS provider_profile_id,
       model,
       SUM(prompt_tokens) AS prompt_tokens,
       SUM(completion_tokens) AS completion_tokens,
       COUNT(*) AS requests
FROM token_ledger
GROUP BY provider_plugin_id, provider_profile_id, model
ORDER BY provider_plugin_id ASC, provider_profile_id ASC, model ASC;
`)
	if err != nil {
		return nil, err
	}
	var rows []tokenUsageSummaryRow
	if err := json.Unmarshal([]byte(emptyArray(out)), &rows); err != nil {
		return nil, err
	}
	summaries := make([]TokenUsageSummary, 0, len(rows))
	for _, row := range rows {
		summaries = append(summaries, TokenUsageSummary{
			ProviderPluginID:  row.ProviderPluginID,
			ProviderProfileID: row.ProviderProfileID,
			Model:             row.Model,
			PromptTokens:      row.PromptTokens,
			CompletionTokens:  row.CompletionTokens,
			Requests:          row.Requests,
		})
	}
	return summaries, nil
}

func (s Store) SessionContextUsage(sessionID string) (SessionContextUsage, error) {
	out, err := s.queryJSON(fmt.Sprintf(`
SELECT
  (SELECT COUNT(*) FROM messages WHERE session_id = %s) AS message_count,
  (SELECT COALESCE(SUM((length(content) + 3) / 4), 0) FROM messages WHERE session_id = %s) AS estimated_tokens,
  (SELECT COALESCE(SUM(prompt_tokens), 0) FROM token_ledger WHERE session_id = %s) AS prompt_tokens,
  (SELECT COALESCE(SUM(completion_tokens), 0) FROM token_ledger WHERE session_id = %s) AS completion_tokens,
  (SELECT COUNT(*) FROM provider_chats WHERE session_id = %s AND status = 'active') AS provider_chats;
`, quote(sessionID), quote(sessionID), quote(sessionID), quote(sessionID), quote(sessionID)))
	if err != nil {
		return SessionContextUsage{}, err
	}
	var rows []sessionContextUsageRow
	if err := json.Unmarshal([]byte(emptyArray(out)), &rows); err != nil {
		return SessionContextUsage{}, err
	}
	if len(rows) == 0 {
		return SessionContextUsage{}, nil
	}
	return SessionContextUsage{
		MessageCount:     rows[0].MessageCount,
		EstimatedTokens:  rows[0].EstimatedTokens,
		PromptTokens:     rows[0].PromptTokens,
		CompletionTokens: rows[0].CompletionTokens,
		ProviderChats:    rows[0].ProviderChats,
	}, nil
}

func (s Store) GetProviderChatBinding(input ProviderChatBindingInput) (ProviderChatBinding, error) {
	out, err := s.queryJSON(fmt.Sprintf(`
SELECT id, session_id, provider_plugin_id,
       COALESCE(provider_profile_id, '') AS provider_profile_id,
       COALESCE(access_route_id, '') AS access_route_id,
       COALESCE(model_id, '') AS model_id,
       external_thread_id, status
FROM provider_chats
WHERE session_id = %s
  AND provider_plugin_id = %s
  AND COALESCE(provider_profile_id, '') = %s
  AND COALESCE(access_route_id, '') = %s
  AND COALESCE(model_id, '') = %s
  AND status = 'active'
LIMIT 1;
`, quote(input.SessionID), quote(input.ProviderPluginID), quote(input.ProviderProfileID), quote(input.AccessRouteID), quote(input.ModelID)))
	if err != nil {
		return ProviderChatBinding{}, err
	}
	var rows []providerChatBindingRow
	if err := json.Unmarshal([]byte(emptyArray(out)), &rows); err != nil {
		return ProviderChatBinding{}, err
	}
	if len(rows) == 0 {
		return ProviderChatBinding{}, ErrNotFound
	}
	return rows[0].toCore(), nil
}

func (s Store) ListProviderChatBindings(sessionID string) ([]ProviderChatBinding, error) {
	out, err := s.queryJSON(fmt.Sprintf(`
SELECT id, session_id, provider_plugin_id,
       COALESCE(provider_profile_id, '') AS provider_profile_id,
       COALESCE(access_route_id, '') AS access_route_id,
       COALESCE(model_id, '') AS model_id,
       external_thread_id, status
FROM provider_chats
WHERE session_id = %s
ORDER BY updated_at DESC, created_at DESC;
`, quote(sessionID)))
	if err != nil {
		return nil, err
	}
	var rows []providerChatBindingRow
	if err := json.Unmarshal([]byte(emptyArray(out)), &rows); err != nil {
		return nil, err
	}
	bindings := make([]ProviderChatBinding, 0, len(rows))
	for _, row := range rows {
		bindings = append(bindings, row.toCore())
	}
	return bindings, nil
}

func (s Store) UpsertProviderChatBinding(input ProviderChatBindingInput) (ProviderChatBinding, error) {
	if strings.TrimSpace(input.SessionID) == "" {
		return ProviderChatBinding{}, errors.New("session id is required")
	}
	if strings.TrimSpace(input.ProviderPluginID) == "" {
		return ProviderChatBinding{}, errors.New("provider plugin id is required")
	}
	if strings.TrimSpace(input.ExternalThreadID) == "" {
		return ProviderChatBinding{}, errors.New("external thread id is required")
	}
	if strings.TrimSpace(input.Status) == "" {
		input.Status = "active"
	}
	if existing, err := s.GetProviderChatBinding(input); err == nil {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		stmt := fmt.Sprintf(`
UPDATE provider_chats
SET external_thread_id = %s,
    status = %s,
    updated_at = %s
WHERE id = %s;
`, quote(input.ExternalThreadID), quote(input.Status), quote(now), quote(existing.ID))
		if err := s.run(stmt); err != nil {
			return ProviderChatBinding{}, err
		}
		return s.GetProviderChatBinding(input)
	} else if !errors.Is(err, ErrNotFound) {
		return ProviderChatBinding{}, err
	}
	bindingID := "provider_chat_" + randomHex(16)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	stmt := fmt.Sprintf(`
INSERT INTO provider_chats (
  id, session_id, provider_plugin_id, provider_profile_id, access_route_id,
  model_id, external_thread_id, status, created_at, updated_at
) VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s);
`, quote(bindingID), quote(input.SessionID), quote(input.ProviderPluginID), nullable(input.ProviderProfileID), nullable(input.AccessRouteID), nullable(input.ModelID), quote(input.ExternalThreadID), quote(input.Status), quote(now), quote(now))
	if err := s.run(stmt); err != nil {
		return ProviderChatBinding{}, err
	}
	return s.GetProviderChatBinding(input)
}

func (s Store) listRegistry(query string) ([]RegistryItem, error) {
	out, err := s.queryJSON(query)
	if err != nil {
		return nil, err
	}
	var rows []registryItemRow
	if err := json.Unmarshal([]byte(emptyArray(out)), &rows); err != nil {
		return nil, err
	}
	items := make([]RegistryItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, RegistryItem{
			ID:          row.ID,
			DisplayName: row.DisplayName,
			Kind:        row.Kind,
			Enabled:     row.Enabled != 0,
		})
	}
	return items, nil
}

func (s Store) nextMessageOrdinal(sessionID string) (int, error) {
	out, err := s.queryJSON(fmt.Sprintf(`
SELECT COALESCE(MAX(ordinal) + 1, 0) AS next_ordinal
FROM messages
WHERE session_id = %s;
`, quote(sessionID)))
	if err != nil {
		return 0, err
	}
	var rows []nextOrdinalRow
	if err := json.Unmarshal([]byte(emptyArray(out)), &rows); err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	return rows[0].NextOrdinal, nil
}

func (s Store) run(sql string) error {
	cmd := exec.Command("sqlite3", s.DBPath)
	cmd.Stdin = strings.NewReader(sql)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (s Store) queryJSON(sql string) (string, error) {
	cmd := exec.Command("sqlite3", "-json", s.DBPath, sql)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return string(out), nil
}

func (s Store) ensureAccessRouteContextBudgetColumn() error {
	out, err := s.queryJSON(`PRAGMA table_info(access_routes);`)
	if err != nil {
		return err
	}
	if strings.Contains(out, `"name":"context_budget_chars"`) {
		return nil
	}
	return s.run(`
ALTER TABLE access_routes ADD COLUMN context_budget_chars INTEGER NOT NULL DEFAULT 0;
UPDATE access_routes SET context_budget_chars = 80000 WHERE id = 'claude-code-cli';
UPDATE access_routes SET context_budget_chars = 60000 WHERE id = 'codex-subscription-cli';
UPDATE access_routes SET context_budget_chars = 4000  WHERE id = 'ollama-local';
`)
}

func (s Store) ensureSessionActiveRouteColumns() error {
	out, err := s.queryJSON(`PRAGMA table_info(sessions);`)
	if err != nil {
		return err
	}
	if !strings.Contains(out, `"name":"active_route_id"`) {
		if err := s.run(`ALTER TABLE sessions ADD COLUMN active_route_id TEXT NOT NULL DEFAULT '';`); err != nil {
			return err
		}
	}
	if !strings.Contains(out, `"name":"active_model_id"`) {
		if err := s.run(`ALTER TABLE sessions ADD COLUMN active_model_id TEXT NOT NULL DEFAULT '';`); err != nil {
			return err
		}
	}
	return nil
}

func (s Store) SetSessionActiveRoute(sessionID, routeID, modelID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return errors.New("session id is required")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return s.run(fmt.Sprintf(`
UPDATE sessions SET active_route_id = %s, active_model_id = %s, updated_at = %s
WHERE id = %s;
`, quote(routeID), quote(modelID), quote(now), quote(sessionID)))
}

func (s Store) ensureProjectHandoffPolicyColumn() error {
	out, err := s.queryJSON(`PRAGMA table_info(projects);`)
	if err != nil {
		return err
	}
	if strings.Contains(out, `"name":"handoff_policy"`) {
		return nil
	}
	return s.run(`ALTER TABLE projects ADD COLUMN handoff_policy TEXT NOT NULL DEFAULT 'route-change';`)
}

func (s Store) ensureProjectContextPolicyColumn() error {
	out, err := s.queryJSON(`PRAGMA table_info(projects);`)
	if err != nil {
		return err
	}
	if strings.Contains(out, `"name":"context_policy"`) {
		return nil
	}
	return s.run(`ALTER TABLE projects ADD COLUMN context_policy TEXT NOT NULL DEFAULT 'flat-trim';`)
}

func (s Store) ensureProjectRoutePolicyColumn() error {
	out, err := s.queryJSON(`PRAGMA table_info(projects);`)
	if err != nil {
		return err
	}
	if strings.Contains(out, `"name":"route_policy"`) {
		return nil
	}
	return s.run(`ALTER TABLE projects ADD COLUMN route_policy TEXT NOT NULL DEFAULT 'manual';`)
}

func (s Store) ensureProjectToolApprovalPolicyColumn() error {
	out, err := s.queryJSON(`PRAGMA table_info(projects);`)
	if err != nil {
		return err
	}
	if strings.Contains(out, `"name":"tool_approval_policy"`) {
		return nil
	}
	return s.run(`ALTER TABLE projects ADD COLUMN tool_approval_policy TEXT NOT NULL DEFAULT 'safe-only';`)
}

func (s Store) ensureProjectKbScopePolicyColumn() error {
	out, err := s.queryJSON(`PRAGMA table_info(projects);`)
	if err != nil {
		return err
	}
	if strings.Contains(out, `"name":"kb_scope_policy"`) {
		return nil
	}
	return s.run(`ALTER TABLE projects ADD COLUMN kb_scope_policy TEXT NOT NULL DEFAULT 'project-only';`)
}

func (s Store) ensureCandidateTriggerEventColumn() error {
	out, err := s.queryJSON(`PRAGMA table_info(candidate_outputs);`)
	if err != nil {
		return err
	}
	if !strings.Contains(out, `"name":"trigger_event_id"`) {
		if err := s.run(`ALTER TABLE candidate_outputs ADD COLUMN trigger_event_id TEXT NOT NULL DEFAULT '';`); err != nil {
			return err
		}
	}
	return s.run(`CREATE INDEX IF NOT EXISTS idx_candidate_outputs_trigger ON candidate_outputs(session_id, trigger_event_id, status);`)
}

func (s Store) ensureSessionProjectColumn() error {
	out, err := s.queryJSON(`PRAGMA table_info(sessions);`)
	if err != nil {
		return err
	}
	if strings.Contains(out, `"name":"project_id"`) {
		return nil
	}
	return s.run(`
ALTER TABLE sessions ADD COLUMN project_id TEXT REFERENCES projects(id);
UPDATE sessions SET project_id = 'default' WHERE project_id IS NULL OR project_id = '';
`)
}

type sessionRow struct {
	ID            string `json:"id"`
	ProjectID     string `json:"project_id"`
	SourceTool    string `json:"source_tool"`
	SourceID      string `json:"source_id"`
	Title         string `json:"title"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
	ParentID      string `json:"parent_id"`
	BranchFromID  string `json:"branch_from_id"`
	ActiveRouteID string `json:"active_route_id"`
	ActiveModelID string `json:"active_model_id"`
}

func (r sessionRow) toCore() (core.Session, error) {
	createdAt, err := parseTime(r.CreatedAt)
	if err != nil {
		return core.Session{}, err
	}
	updatedAt, err := parseTime(r.UpdatedAt)
	if err != nil {
		return core.Session{}, err
	}
	return core.Session{
		ID:            r.ID,
		ProjectID:     r.ProjectID,
		SourceTool:    core.SourceTool(r.SourceTool),
		SourceID:      r.SourceID,
		Title:         r.Title,
		CreatedAt:     createdAt,
		UpdatedAt:     updatedAt,
		ParentID:      r.ParentID,
		BranchFromID:  r.BranchFromID,
		ActiveRouteID: r.ActiveRouteID,
		ActiveModelID: r.ActiveModelID,
	}, nil
}

type messageRow struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	Role      string `json:"role"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
	SourceID  string `json:"source_id"`
}

type messageEventRow struct {
	ID            string `json:"id"`
	SessionID     string `json:"session_id"`
	MessageID     string `json:"message_id"`
	ActivityIndex int    `json:"activity_index"`
	Kind          string `json:"kind"`
	PayloadJSON   string `json:"payload_json"`
	CreatedAt     string `json:"created_at"`
}

type contextEventRow struct {
	ID             string `json:"id"`
	Type           string `json:"type"`
	ProjectID      string `json:"project_id"`
	SessionID      string `json:"session_id"`
	BranchID       string `json:"branch_id"`
	PayloadRef     string `json:"payload_ref"`
	ParentEventIDs string `json:"parent_event_ids"`
	CreatedAt      string `json:"created_at"`
}

type contextHeadRow struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	SessionID string `json:"session_id"`
	BranchID  string `json:"branch_id"`
	EventID   string `json:"event_id"`
	UpdatedAt string `json:"updated_at"`
}

type knowledgeItemRow struct {
	ID            string `json:"id"`
	Scope         string `json:"scope"`
	ProjectID     string `json:"project_id"`
	Kind          string `json:"kind"`
	Title         string `json:"title"`
	SourceEventID string `json:"source_event_id"`
	ContentRef    string `json:"content_ref"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

type chatRunRow struct {
	ID              string `json:"id"`
	ProjectID       string `json:"project_id"`
	SessionID       string `json:"session_id"`
	BranchID        string `json:"branch_id"`
	Role            string `json:"role"`
	Status          string `json:"status"`
	InputEventID    string `json:"input_event_id"`
	OutputEventID   string `json:"output_event_id"`
	ContextPacketID string `json:"context_packet_id"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

type providerSegmentRow struct {
	ID               string `json:"id"`
	ChatRunID        string `json:"chat_run_id"`
	ProviderID       string `json:"provider_id"`
	RouteID          string `json:"route_id"`
	ModelID          string `json:"model_id"`
	ExternalThreadID string `json:"external_thread_id"`
	Status           string `json:"status"`
	HandoffReason    string `json:"handoff_reason"`
	StartedAt        string `json:"started_at"`
	CompletedAt      string `json:"completed_at"`
}

type contextPacketRow struct {
	ID             string `json:"id"`
	ProjectID      string `json:"project_id"`
	SessionID      string `json:"session_id"`
	BranchID       string `json:"branch_id"`
	HeadEventID    string `json:"head_event_id"`
	UserInput      string `json:"user_input"`
	ContentRef     string `json:"content_ref"`
	ReferenceCount int    `json:"reference_count"`
	CreatedAt      string `json:"created_at"`
}

type steeringRow struct {
	ID                string `json:"id"`
	ChatRunID         string `json:"chat_run_id"`
	ProviderSegmentID string `json:"provider_segment_id"`
	EventID           string `json:"event_id"`
	Content           string `json:"content"`
	Status            string `json:"status"`
	CreatedAt         string `json:"created_at"`
}

type queueItemRow struct {
	ID             string `json:"id"`
	SessionID      string `json:"session_id"`
	BranchID       string `json:"branch_id"`
	Content        string `json:"content"`
	Mode           string `json:"mode"`
	Status         string `json:"status"`
	OrderIndex     int    `json:"order_index"`
	RouteID        string `json:"route_id"`
	ModelID        string `json:"model_id"`
	ThinkingEffort string `json:"thinking_effort"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

type candidateOutputRow struct {
	ID             string `json:"id"`
	ChatRunID      string `json:"chat_run_id"`
	SessionID      string `json:"session_id"`
	BranchID       string `json:"branch_id"`
	TriggerEventID string `json:"trigger_event_id"`
	ContentRef     string `json:"content_ref"`
	Status         string `json:"status"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

type registryItemRow struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Kind        string `json:"kind"`
	Enabled     int    `json:"enabled"`
}

type providerProfileRow struct {
	ID               string `json:"id"`
	ProviderPluginID string `json:"provider_plugin_id"`
	DisplayName      string `json:"display_name"`
	IsDefault        int    `json:"is_default"`
}

type providerModelRow struct {
	ID               string `json:"id"`
	ProviderPluginID string `json:"provider_plugin_id"`
	DisplayName      string `json:"display_name"`
	ModelRef         string `json:"model_ref"`
	Status           string `json:"status"`
	IsDefault        int    `json:"is_default"`
}

type accessRouteRow struct {
	ID                 string `json:"id"`
	ProviderPluginID   string `json:"provider_plugin_id"`
	DisplayName        string `json:"display_name"`
	AccessMode         string `json:"access_mode"`
	Transport          string `json:"transport"`
	RequiresLicense    int    `json:"requires_license"`
	RequiresAPIKey     int    `json:"requires_api_key"`
	SupportsStreaming  int    `json:"supports_streaming"`
	SupportsTools      int    `json:"supports_tools"`
	SupportsImport     int    `json:"supports_import"`
	SupportsHandoff    int    `json:"supports_handoff"`
	CostModel          string `json:"cost_model"`
	Status             string `json:"status"`
	ContextBudgetChars int    `json:"context_budget_chars"`
}

type projectRow struct {
	ID                 string `json:"id"`
	DisplayName        string `json:"display_name"`
	RootPath           string `json:"root_path"`
	IsDefault          int    `json:"is_default"`
	ContextPolicy      string `json:"context_policy"`
	HandoffPolicy      string `json:"handoff_policy"`
	RoutePolicy        string `json:"route_policy"`
	ToolApprovalPolicy string `json:"tool_approval_policy"`
	KbScopePolicy      string `json:"kb_scope_policy"`
}

type commandRunRow struct {
	ID         string `json:"id"`
	ProjectID  string `json:"project_id"`
	SessionID  string `json:"session_id"`
	Command    string `json:"command"`
	WorkingDir string `json:"working_dir"`
	Status     string `json:"status"`
	ExitCode   int    `json:"exit_code"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at"`
}

func (r projectRow) toCore() Project {
	return Project{
		ID:                 r.ID,
		DisplayName:        r.DisplayName,
		RootPath:           r.RootPath,
		IsDefault:          r.IsDefault != 0,
		ContextPolicy:      r.ContextPolicy,
		HandoffPolicy:      r.HandoffPolicy,
		RoutePolicy:        r.RoutePolicy,
		ToolApprovalPolicy: r.ToolApprovalPolicy,
		KbScopePolicy:      r.KbScopePolicy,
	}
}

type projectAccessRouteRow struct {
	ProjectID          string `json:"project_id"`
	Enabled            int    `json:"enabled"`
	Priority           int    `json:"priority"`
	ID                 string `json:"id"`
	ProviderPluginID   string `json:"provider_plugin_id"`
	DisplayName        string `json:"display_name"`
	AccessMode         string `json:"access_mode"`
	Transport          string `json:"transport"`
	RequiresLicense    int    `json:"requires_license"`
	RequiresAPIKey     int    `json:"requires_api_key"`
	SupportsStreaming  int    `json:"supports_streaming"`
	SupportsTools      int    `json:"supports_tools"`
	SupportsImport     int    `json:"supports_import"`
	SupportsHandoff    int    `json:"supports_handoff"`
	CostModel          string `json:"cost_model"`
	Status             string `json:"status"`
	ContextBudgetChars int    `json:"context_budget_chars"`
}

type moderatorPreferenceRow struct {
	Scope                    string `json:"scope"`
	ProjectID                string `json:"project_id"`
	Mode                     string `json:"mode"`
	PrimaryProviderGroupID   string `json:"primary_provider_group_id"`
	SecondaryProviderGroupID string `json:"secondary_provider_group_id"`
}

func (r moderatorPreferenceRow) toCore() ModeratorPreference {
	return ModeratorPreference{
		Scope:                    r.Scope,
		ProjectID:                r.ProjectID,
		Mode:                     r.Mode,
		PrimaryProviderGroupID:   r.PrimaryProviderGroupID,
		SecondaryProviderGroupID: r.SecondaryProviderGroupID,
	}
}

func (r projectAccessRouteRow) toAccessRoute() AccessRoute {
	return AccessRoute{
		ID:                 r.ID,
		ProviderPluginID:   r.ProviderPluginID,
		DisplayName:        r.DisplayName,
		AccessMode:         r.AccessMode,
		Transport:          r.Transport,
		RequiresLicense:    r.RequiresLicense != 0,
		RequiresAPIKey:     r.RequiresAPIKey != 0,
		SupportsStreaming:  r.SupportsStreaming != 0,
		SupportsTools:      r.SupportsTools != 0,
		SupportsImport:     r.SupportsImport != 0,
		SupportsHandoff:    r.SupportsHandoff != 0,
		CostModel:          r.CostModel,
		Status:             r.Status,
		ContextBudgetChars: r.ContextBudgetChars,
	}
}

type tokenUsageSummaryRow struct {
	ProviderPluginID  string `json:"provider_plugin_id"`
	ProviderProfileID string `json:"provider_profile_id"`
	Model             string `json:"model"`
	PromptTokens      int    `json:"prompt_tokens"`
	CompletionTokens  int    `json:"completion_tokens"`
	Requests          int    `json:"requests"`
}

type sessionContextUsageRow struct {
	MessageCount     int `json:"message_count"`
	EstimatedTokens  int `json:"estimated_tokens"`
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	ProviderChats    int `json:"provider_chats"`
}

type providerChatBindingRow struct {
	ID                string `json:"id"`
	SessionID         string `json:"session_id"`
	ProviderPluginID  string `json:"provider_plugin_id"`
	ProviderProfileID string `json:"provider_profile_id"`
	AccessRouteID     string `json:"access_route_id"`
	ModelID           string `json:"model_id"`
	ExternalThreadID  string `json:"external_thread_id"`
	Status            string `json:"status"`
}

func (r providerChatBindingRow) toCore() ProviderChatBinding {
	return ProviderChatBinding{
		ID:                r.ID,
		SessionID:         r.SessionID,
		ProviderPluginID:  r.ProviderPluginID,
		ProviderProfileID: r.ProviderProfileID,
		AccessRouteID:     r.AccessRouteID,
		ModelID:           r.ModelID,
		ExternalThreadID:  r.ExternalThreadID,
		Status:            r.Status,
	}
}

type nextOrdinalRow struct {
	NextOrdinal int `json:"next_ordinal"`
}

func (r messageRow) toCore() (core.Message, error) {
	createdAt, err := parseTime(r.CreatedAt)
	if err != nil {
		return core.Message{}, err
	}
	return core.Message{
		ID:        r.ID,
		SessionID: r.SessionID,
		Role:      r.Role,
		Content:   r.Content,
		CreatedAt: createdAt,
		SourceID:  r.SourceID,
	}, nil
}

func (r messageEventRow) toCore() MessageEvent {
	return MessageEvent{
		ID:            r.ID,
		SessionID:     r.SessionID,
		MessageID:     r.MessageID,
		ActivityIndex: r.ActivityIndex,
		Kind:          r.Kind,
		PayloadJSON:   r.PayloadJSON,
		CreatedAt:     r.CreatedAt,
	}
}

func (r contextEventRow) toCore() (core.Event, error) {
	createdAt, err := parseTime(r.CreatedAt)
	if err != nil {
		return core.Event{}, err
	}
	parentIDs := []string{}
	for _, id := range strings.Split(r.ParentEventIDs, "\x1f") {
		id = strings.TrimSpace(id)
		if id != "" {
			parentIDs = append(parentIDs, id)
		}
	}
	return core.Event{
		ID:             r.ID,
		Type:           core.EventType(r.Type),
		ProjectID:      r.ProjectID,
		SessionID:      r.SessionID,
		BranchID:       r.BranchID,
		ParentEventIDs: parentIDs,
		PayloadRef:     r.PayloadRef,
		CreatedAt:      createdAt,
	}, nil
}

func (r contextHeadRow) toCore() (core.Head, error) {
	updatedAt, err := parseTime(r.UpdatedAt)
	if err != nil {
		return core.Head{}, err
	}
	return core.Head{
		ID:        r.ID,
		ProjectID: r.ProjectID,
		SessionID: r.SessionID,
		BranchID:  r.BranchID,
		EventID:   r.EventID,
		UpdatedAt: updatedAt,
	}, nil
}

func (r knowledgeItemRow) toCore() (core.KnowledgeItem, error) {
	createdAt, err := parseTime(r.CreatedAt)
	if err != nil {
		return core.KnowledgeItem{}, err
	}
	updatedAt, err := parseTime(r.UpdatedAt)
	if err != nil {
		return core.KnowledgeItem{}, err
	}
	return core.KnowledgeItem{
		ID:            r.ID,
		Scope:         core.KnowledgeScope(r.Scope),
		ProjectID:     r.ProjectID,
		Kind:          r.Kind,
		Title:         r.Title,
		SourceEventID: r.SourceEventID,
		ContentRef:    r.ContentRef,
		CreatedAt:     createdAt,
		UpdatedAt:     updatedAt,
	}, nil
}

func (r chatRunRow) toCore() (core.ChatRun, error) {
	createdAt, err := parseTime(r.CreatedAt)
	if err != nil {
		return core.ChatRun{}, err
	}
	updatedAt, err := parseTime(r.UpdatedAt)
	if err != nil {
		return core.ChatRun{}, err
	}
	return core.ChatRun{
		ID:              r.ID,
		ProjectID:       r.ProjectID,
		SessionID:       r.SessionID,
		BranchID:        r.BranchID,
		Role:            core.ChatRunRole(r.Role),
		Status:          core.ChatRunStatus(r.Status),
		InputEventID:    r.InputEventID,
		OutputEventID:   r.OutputEventID,
		ContextPacketID: r.ContextPacketID,
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
	}, nil
}

func (r providerSegmentRow) toCore() (core.ProviderSegment, error) {
	startedAt, err := parseTime(r.StartedAt)
	if err != nil {
		return core.ProviderSegment{}, err
	}
	var completedAt *time.Time
	if strings.TrimSpace(r.CompletedAt) != "" {
		parsed, err := parseTime(r.CompletedAt)
		if err != nil {
			return core.ProviderSegment{}, err
		}
		completedAt = &parsed
	}
	return core.ProviderSegment{
		ID:               r.ID,
		ChatRunID:        r.ChatRunID,
		ProviderID:       r.ProviderID,
		RouteID:          r.RouteID,
		ModelID:          r.ModelID,
		ExternalThreadID: r.ExternalThreadID,
		Status:           core.ProviderSegmentStatus(r.Status),
		HandoffReason:    r.HandoffReason,
		StartedAt:        startedAt,
		CompletedAt:      completedAt,
	}, nil
}

func (r contextPacketRow) toCore() (ContextPacketRecord, error) {
	createdAt, err := parseTime(r.CreatedAt)
	if err != nil {
		return ContextPacketRecord{}, err
	}
	return ContextPacketRecord{
		ID:             r.ID,
		ProjectID:      r.ProjectID,
		SessionID:      r.SessionID,
		BranchID:       r.BranchID,
		HeadEventID:    r.HeadEventID,
		UserInput:      r.UserInput,
		ContentRef:     r.ContentRef,
		ReferenceCount: r.ReferenceCount,
		CreatedAt:      createdAt,
	}, nil
}

func (r steeringRow) toCore() (SteeringRecord, error) {
	createdAt, err := parseTime(r.CreatedAt)
	if err != nil {
		return SteeringRecord{}, err
	}
	return SteeringRecord{
		ID:                r.ID,
		ChatRunID:         r.ChatRunID,
		ProviderSegmentID: r.ProviderSegmentID,
		EventID:           r.EventID,
		Content:           r.Content,
		Status:            r.Status,
		CreatedAt:         createdAt,
	}, nil
}

func (r queueItemRow) toCore() (core.QueueItem, error) {
	createdAt, err := parseTime(r.CreatedAt)
	if err != nil {
		return core.QueueItem{}, err
	}
	updatedAt, err := parseTime(r.UpdatedAt)
	if err != nil {
		return core.QueueItem{}, err
	}
	return core.QueueItem{
		ID:             r.ID,
		SessionID:      r.SessionID,
		BranchID:       r.BranchID,
		Content:        r.Content,
		Mode:           core.QueueItemMode(r.Mode),
		Status:         core.QueueItemStatus(r.Status),
		OrderIndex:     r.OrderIndex,
		RouteID:        r.RouteID,
		ModelID:        r.ModelID,
		ThinkingEffort: r.ThinkingEffort,
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
	}, nil
}

func (r candidateOutputRow) toCore() (core.CandidateOutput, error) {
	createdAt, err := parseTime(r.CreatedAt)
	if err != nil {
		return core.CandidateOutput{}, err
	}
	updatedAt, err := parseTime(r.UpdatedAt)
	if err != nil {
		return core.CandidateOutput{}, err
	}
	return core.CandidateOutput{
		ID:             r.ID,
		ChatRunID:      r.ChatRunID,
		SessionID:      r.SessionID,
		BranchID:       r.BranchID,
		TriggerEventID: r.TriggerEventID,
		ContentRef:     r.ContentRef,
		Status:         core.CandidateOutputStatus(r.Status),
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
	}, nil
}

func parseTime(value string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02T15:04:05.000Z", value)
}

func quote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func nullable(value string) string {
	if value == "" {
		return "NULL"
	}
	return quote(value)
}

func emptyArray(value string) string {
	if strings.TrimSpace(value) == "" {
		return "[]"
	}
	return value
}

func compactStrings(values []string) []string {
	seen := map[string]bool{}
	items := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		items = append(items, value)
	}
	return items
}

func randomHex(bytesLen int) string {
	buf := make([]byte, bytesLen)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buf)
}
