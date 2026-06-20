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

	"github.com/jkj-dev/ergo-loom/internal/core"
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
	ID                string
	ProviderPluginID  string
	DisplayName       string
	AccessMode        string
	Transport         string
	RequiresLicense   bool
	RequiresAPIKey    bool
	SupportsStreaming bool
	SupportsTools     bool
	SupportsImport    bool
	SupportsHandoff   bool
	CostModel         string
	Status            string
}

type Project struct {
	ID          string
	DisplayName string
	RootPath    string
	IsDefault   bool
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

	return s.run(string(schema))
}

func (s Store) ListSessions() ([]core.Session, error) {
	out, err := s.queryJSON(`
SELECT id, source_tool, source_id, title, created_at, updated_at,
       COALESCE(parent_session_id, '') AS parent_id,
       COALESCE(branch_from_message_id, '') AS branch_from_id
FROM sessions
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
SELECT id, source_tool, source_id, title, created_at, updated_at,
       COALESCE(parent_session_id, '') AS parent_id,
       COALESCE(branch_from_message_id, '') AS branch_from_id
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
	if strings.TrimSpace(title) == "" {
		title = "Untitled chat"
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	sessionID := "session_" + randomHex(16)
	stmt := fmt.Sprintf(`INSERT INTO sessions (id, source_tool, source_id, title, created_at, updated_at)
VALUES (%s, 'ergo', %s, %s, %s, %s);
`, quote(sessionID), quote(sessionID), quote(title), quote(now), quote(now))

	if err := s.run(stmt); err != nil {
		return core.Session{}, err
	}
	session, _, err := s.GetSession(sessionID)
	return session, err
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

func (s Store) AddMessage(sessionID string, role string, content string) (core.Message, error) {
	if strings.TrimSpace(role) == "" {
		return core.Message{}, errors.New("role is required")
	}
	if strings.TrimSpace(content) == "" {
		return core.Message{}, errors.New("content is required")
	}

	if _, _, err := s.GetSession(sessionID); err != nil {
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
	fmt.Fprintf(&sql, `INSERT INTO sessions (id, source_tool, source_id, title, created_at, updated_at, parent_session_id, branch_from_message_id)
VALUES (%s, %s, %s, %s, %s, %s, %s, %s);
`, quote(branchSessionID), quote(string(session.SourceTool)), quote(branchSessionID), quote(title), quote(now), quote(now), quote(session.ID), quote(fromMessageID))

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
       supports_import, supports_handoff, cost_model, status
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
			ID:                row.ID,
			ProviderPluginID:  row.ProviderPluginID,
			DisplayName:       row.DisplayName,
			AccessMode:        row.AccessMode,
			Transport:         row.Transport,
			RequiresLicense:   row.RequiresLicense != 0,
			RequiresAPIKey:    row.RequiresAPIKey != 0,
			SupportsStreaming: row.SupportsStreaming != 0,
			SupportsTools:     row.SupportsTools != 0,
			SupportsImport:    row.SupportsImport != 0,
			SupportsHandoff:   row.SupportsHandoff != 0,
			CostModel:         row.CostModel,
			Status:            row.Status,
		})
	}
	return routes, nil
}

func (s Store) DefaultProject() (Project, error) {
	out, err := s.queryJSON(`
SELECT id, display_name, COALESCE(root_path, '') AS root_path, is_default
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
SELECT id, display_name, COALESCE(root_path, '') AS root_path, is_default
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
SELECT id, display_name, COALESCE(root_path, '') AS root_path, is_default
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
SELECT id, display_name, COALESCE(root_path, '') AS root_path, is_default
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
       ar.status
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

type sessionRow struct {
	ID           string `json:"id"`
	SourceTool   string `json:"source_tool"`
	SourceID     string `json:"source_id"`
	Title        string `json:"title"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
	ParentID     string `json:"parent_id"`
	BranchFromID string `json:"branch_from_id"`
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
		ID:           r.ID,
		SourceTool:   core.SourceTool(r.SourceTool),
		SourceID:     r.SourceID,
		Title:        r.Title,
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
		ParentID:     r.ParentID,
		BranchFromID: r.BranchFromID,
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
	ID                string `json:"id"`
	ProviderPluginID  string `json:"provider_plugin_id"`
	DisplayName       string `json:"display_name"`
	AccessMode        string `json:"access_mode"`
	Transport         string `json:"transport"`
	RequiresLicense   int    `json:"requires_license"`
	RequiresAPIKey    int    `json:"requires_api_key"`
	SupportsStreaming int    `json:"supports_streaming"`
	SupportsTools     int    `json:"supports_tools"`
	SupportsImport    int    `json:"supports_import"`
	SupportsHandoff   int    `json:"supports_handoff"`
	CostModel         string `json:"cost_model"`
	Status            string `json:"status"`
}

type projectRow struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	RootPath    string `json:"root_path"`
	IsDefault   int    `json:"is_default"`
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
		ID:          r.ID,
		DisplayName: r.DisplayName,
		RootPath:    r.RootPath,
		IsDefault:   r.IsDefault != 0,
	}
}

type projectAccessRouteRow struct {
	ProjectID         string `json:"project_id"`
	Enabled           int    `json:"enabled"`
	Priority          int    `json:"priority"`
	ID                string `json:"id"`
	ProviderPluginID  string `json:"provider_plugin_id"`
	DisplayName       string `json:"display_name"`
	AccessMode        string `json:"access_mode"`
	Transport         string `json:"transport"`
	RequiresLicense   int    `json:"requires_license"`
	RequiresAPIKey    int    `json:"requires_api_key"`
	SupportsStreaming int    `json:"supports_streaming"`
	SupportsTools     int    `json:"supports_tools"`
	SupportsImport    int    `json:"supports_import"`
	SupportsHandoff   int    `json:"supports_handoff"`
	CostModel         string `json:"cost_model"`
	Status            string `json:"status"`
}

func (r projectAccessRouteRow) toAccessRoute() AccessRoute {
	return AccessRoute{
		ID:                r.ID,
		ProviderPluginID:  r.ProviderPluginID,
		DisplayName:       r.DisplayName,
		AccessMode:        r.AccessMode,
		Transport:         r.Transport,
		RequiresLicense:   r.RequiresLicense != 0,
		RequiresAPIKey:    r.RequiresAPIKey != 0,
		SupportsStreaming: r.SupportsStreaming != 0,
		SupportsTools:     r.SupportsTools != 0,
		SupportsImport:    r.SupportsImport != 0,
		SupportsHandoff:   r.SupportsHandoff != 0,
		CostModel:         r.CostModel,
		Status:            r.Status,
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

func randomHex(bytesLen int) string {
	buf := make([]byte, bytesLen)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buf)
}
