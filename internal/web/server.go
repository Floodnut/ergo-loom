package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	webapp "github.com/Floodnut/ergo-loom/apps/desktop-or-web"
	"github.com/Floodnut/ergo-loom/internal/chatfilter"
	"github.com/Floodnut/ergo-loom/internal/core"
	"github.com/Floodnut/ergo-loom/internal/handoffpolicy"
	"github.com/Floodnut/ergo-loom/internal/knowledge"
	"github.com/Floodnut/ergo-loom/internal/packetpolicy"
	"github.com/Floodnut/ergo-loom/internal/provider"
	"github.com/Floodnut/ergo-loom/internal/routepolicy"
	"github.com/Floodnut/ergo-loom/internal/storage/sqlitecli"
	"github.com/Floodnut/ergo-loom/internal/toolruntime"
)

// PolicyEntry is the shape returned by GET /api/plugins for each policy option.
type PolicyEntry struct {
	Name  string `json:"name"`
	Label string `json:"label"`
}

type Server struct {
	store              sqlitecli.Store
	approvals          *approvalBroker
	filters            chatfilter.Chain
	drivers            provider.DriverRegistry
	packetPolicies     packetpolicy.Registry
	handoffPolicies    handoffpolicy.Registry
	routePolicies      routepolicy.Registry
	toolApprovalPolicy []PolicyEntry
	kbScopePolicy      []PolicyEntry
	knowledge          core.KnowledgeRetriever
	moderator          core.Moderator
	sessionCancelMu    sync.Mutex
	sessionCancels     map[string]context.CancelFunc
	sessionExcludedMu  sync.Mutex
	sessionExcluded    map[string]map[string]bool // sessionID → set of excluded routeIDs
}

const providerSoftTokenCap = 50000
const contextKnowledgeLimit = 3

type chatSelection struct {
	Route   sqlitecli.AccessRoute
	Model   sqlitecli.ProviderModel
	Profile sqlitecli.ProviderProfile
}

type RunRequest struct {
	SessionID      string
	Content        string
	ThinkingEffort string
	Selection      chatSelection
	ContextNote    string
}

type moderatorHandoff struct {
	FromSelection chatSelection
	ToSelection   chatSelection
	Reason        string
}

type authStatus struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	AccountLabel string `json:"accountLabel"`
	Connected    bool   `json:"connected"`
	Status       string `json:"status"`
	Detail       string `json:"detail"`
}

type runtimeDiagnostics struct {
	Desktop       bool                `json:"desktop"`
	AppRoot       string              `json:"appRoot"`
	DataDir       string              `json:"dataDir"`
	HandoffBridge string              `json:"handoffBridge"`
	Path          string              `json:"path"`
	Executables   []runtimeExecutable `json:"executables"`
}

type runtimeExecutable struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Path   string `json:"path"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

func (s Server) projectFromRequest(r *http.Request, projects []sqlitecli.Project) (sqlitecli.Project, error) {
	projectID := strings.TrimSpace(r.URL.Query().Get("projectId"))
	if projectID != "" {
		for _, project := range projects {
			if project.ID == projectID {
				return project, nil
			}
		}
	}
	if len(projects) > 0 {
		for _, project := range projects {
			if project.IsDefault {
				return project, nil
			}
		}
		return projects[0], nil
	}
	return s.store.DefaultProject()
}

type approvalBroker struct {
	mu      sync.Mutex
	pending map[string]chan string
}

func newApprovalBroker() *approvalBroker {
	return &approvalBroker{pending: map[string]chan string{}}
}

func (b *approvalBroker) request(ctx context.Context, event toolruntime.Event) (string, error) {
	id := strings.TrimSpace(event.ApprovalID)
	if id == "" {
		return "decline", errors.New("approval id is required")
	}
	ch := make(chan string, 1)
	b.mu.Lock()
	b.pending[id] = ch
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		delete(b.pending, id)
		b.mu.Unlock()
	}()

	select {
	case decision := <-ch:
		return decision, nil
	case <-ctx.Done():
		return "decline", ctx.Err()
	}
}

func (b *approvalBroker) resolve(id string, decision string) bool {
	b.mu.Lock()
	ch, ok := b.pending[id]
	b.mu.Unlock()
	if !ok {
		return false
	}
	select {
	case ch <- decision:
	default:
	}
	return true
}

func NewServer(store sqlitecli.Store) Server {
	return Server{
		store:           store,
		approvals:       newApprovalBroker(),
		filters:         chatfilter.NewChain(chatfilter.IdentityFilter{}),
		packetPolicies:  packetpolicy.NewRegistry(),
		handoffPolicies: handoffpolicy.NewRegistry(),
		routePolicies:   routepolicy.NewRegistry(),
		knowledge:       knowledge.NewKeywordRetriever(store),
		moderator:       core.DefaultModerator{},
		sessionCancels:  make(map[string]context.CancelFunc),
		sessionExcluded: make(map[string]map[string]bool),
		toolApprovalPolicy: []PolicyEntry{
			{Name: "safe-only", Label: "Safe Only"},
			{Name: "ask-per-command", Label: "Ask Per Command"},
			{Name: "allow-all", Label: "Allow All"},
		},
		kbScopePolicy: []PolicyEntry{
			{Name: "project-only", Label: "Project Only"},
			{Name: "project-and-global", Label: "Project and Global"},
			{Name: "disabled", Label: "Disabled"},
		},
		drivers: provider.NewDriverRegistry(
			provider.CodexAppServerDriver{},
			provider.UnavailableDriver{ProviderID: "openai", Reason: "ChatGPT handoff driver is not implemented yet"},
			provider.ClaudeCLIDriver{},
			provider.CopilotBridgeDriver{},
			provider.UnavailableDriver{ProviderID: "gemini", Reason: "Gemini bridge driver is not implemented yet"},
			provider.UnavailableDriver{ProviderID: "ollama", Reason: "Ollama local model driver is not implemented yet"},
		),
	}
}

func (s Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/state", s.state)
	mux.HandleFunc("GET /api/plugins", s.plugins)
	mux.HandleFunc("GET /api/auth/status", s.authStatus)
	mux.HandleFunc("POST /api/projects", s.createProject)
	mux.HandleFunc("PATCH /api/projects/", s.renameProject)
	mux.HandleFunc("PATCH /api/projects/{projectID}/settings", s.updateProjectSettings)
	mux.HandleFunc("POST /api/projects/", s.projectRoute)
	mux.HandleFunc("DELETE /api/projects/", s.projectRoute)
	mux.HandleFunc("POST /api/provider-profiles/connect", s.connectProviderProfile)
	mux.HandleFunc("GET /api/sessions/search", s.searchSessions)
	mux.HandleFunc("PATCH /api/sessions/{sessionID}/providers", s.updateSessionProviders)
	mux.HandleFunc("PATCH /api/sessions/{sessionID}/route", s.setSessionActiveRoute)
	mux.HandleFunc("POST /api/sessions/{sessionID}/steering", s.recordSteering)
	mux.HandleFunc("GET /api/sessions/{sessionID}/queue", s.sessionQueue)
	mux.HandleFunc("POST /api/sessions/{sessionID}/queue", s.sessionQueue)
	mux.HandleFunc("PATCH /api/sessions/{sessionID}/queue", s.sessionQueue)
	mux.HandleFunc("POST /api/sessions/{sessionID}/parallel", s.startParallelRun)
	mux.HandleFunc("DELETE /api/sessions/{sessionID}/run", s.cancelSessionRun)
	mux.HandleFunc("PATCH /api/candidates/{candidateID}", s.updateCandidateOutput)
	mux.HandleFunc("GET /api/sessions/", s.session)
	mux.HandleFunc("PATCH /api/sessions/", s.renameSession)
	mux.HandleFunc("DELETE /api/sessions/", s.deleteSession)
	mux.HandleFunc("POST /api/sessions/", s.sessionMessage)
	mux.HandleFunc("POST /api/sessions", s.createSession)
	mux.HandleFunc("GET /api/knowledge", s.listKnowledge)
	mux.HandleFunc("POST /api/knowledge", s.addKnowledge)
	mux.HandleFunc("POST /api/files/read", s.readFile)
	mux.HandleFunc("POST /api/terminal/run", s.runTerminalCommand)
	mux.HandleFunc("POST /api/tool-approvals/", s.resolveToolApproval)
	staticFiles, err := fs.Sub(webapp.Files(), "static")
	if err != nil {
		panic(err)
	}
	mux.Handle("GET /", http.FileServer(http.FS(staticFiles)))
	return mux
}

func (s Server) plugins(w http.ResponseWriter, r *http.Request) {
	providers, err := s.store.ListProviderPlugins()
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	agents, err := s.store.ListAgentPlugins()
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	capabilities, err := s.store.ListCapabilities()
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	tools, err := s.store.ListTools()
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	routes, err := s.store.ListAccessRoutes()
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	models, err := s.store.ListProviderModels()
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]any{
		"providers":    providers,
		"agents":       agents,
		"capabilities": capabilities,
		"tools":        tools,
		"routes":       routes,
		"models":       models,
		"policies": map[string]any{
			"contextPackets": s.packetPolicies.List(),
			"handoffs":       s.handoffPolicies.List(),
			"routeSelection": s.routePolicies.List(),
			"toolApproval":   s.toolApprovalPolicy,
			"kbScope":        s.kbScopePolicy,
		},
	})
}

func (s Server) state(w http.ResponseWriter, r *http.Request) {
	projects, err := s.store.ListProjects()
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	project, err := s.projectFromRequest(r, projects)
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	sessions, err := s.store.ListSessionsForProject(project.ID)
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	providers, err := s.store.ListProviderPlugins()
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	agents, err := s.store.ListAgentPlugins()
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	tools, err := s.store.ListTools()
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	profiles, err := s.store.ListProviderProfiles()
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	models, err := s.store.ListProviderModels()
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	usage, err := s.store.TokenUsageSummary()
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	routes, err := s.store.ListAccessRoutes()
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	if project.IsDefault {
		s.ensureDefaultProjectRoutes(project.ID)
	}
	projectRoutes, err := s.store.ListProjectAccessRoutes(project.ID)
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	if len(projectRoutes) == 0 && !project.IsDefault {
		s.copyDefaultProjectRoutes(project.ID)
		projectRoutes, err = s.store.ListProjectAccessRoutes(project.ID)
		if err != nil {
			writeError(w, err, http.StatusInternalServerError)
			return
		}
	}
	moderator, err := s.store.ModeratorPreference(project.ID)
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]any{
		"sessions":            sessions,
		"projects":            projects,
		"providers":           providers,
		"agents":              agents,
		"tools":               tools,
		"profiles":            profiles,
		"models":              models,
		"routes":              routes,
		"project":             project,
		"projectRoutes":       projectRoutes,
		"moderatorPreference": moderator,
		"usage":               usage,
		"auth":                detectAuthStatuses(),
		"diagnostics":         detectRuntimeDiagnostics(),
	})
}

func (s Server) ensureDefaultProjectRoutes(projectID string) {
	for _, routeID := range []string{"codex-subscription-cli"} {
		_ = s.store.AddProjectAccessRoute(projectID, routeID)
	}
}

func (s Server) authStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"auth": detectAuthStatuses()})
}

func (s Server) createProject(w http.ResponseWriter, r *http.Request) {
	var input struct {
		DisplayName string `json:"displayName"`
		RootPath    string `json:"rootPath"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	project, err := s.store.CreateProject(input.DisplayName, input.RootPath)
	if err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	s.copyDefaultProjectRoutes(project.ID)
	writeJSON(w, map[string]any{"project": project})
}

func (s Server) copyDefaultProjectRoutes(projectID string) {
	defaultProject, err := s.store.DefaultProject()
	if err != nil || defaultProject.ID == projectID {
		return
	}
	routes, err := s.store.ListProjectAccessRoutes(defaultProject.ID)
	if err != nil {
		return
	}
	for _, route := range routes {
		if route.Enabled {
			_ = s.store.AddProjectAccessRoute(projectID, route.Route.ID)
		}
	}
}

func (s Server) renameProject(w http.ResponseWriter, r *http.Request) {
	projectID, ok := projectIDFromPath(r.URL.Path)
	if !ok {
		writeError(w, errors.New("project id is required"), http.StatusBadRequest)
		return
	}
	var input struct {
		DisplayName string `json:"displayName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	project, err := s.store.RenameProject(projectID, input.DisplayName)
	if err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"project": project})
}

func (s Server) updateProjectSettings(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectID")
	if projectID == "" {
		writeError(w, errors.New("project id is required"), http.StatusBadRequest)
		return
	}
	var input struct {
		ContextPolicy      string `json:"contextPolicy"`
		HandoffPolicy      string `json:"handoffPolicy"`
		RoutePolicy        string `json:"routePolicy"`
		ToolApprovalPolicy string `json:"toolApprovalPolicy"`
		KbScopePolicy      string `json:"kbScopePolicy"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	if err := s.validateProjectSettings(input.ContextPolicy, input.HandoffPolicy, input.RoutePolicy, input.ToolApprovalPolicy, input.KbScopePolicy); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	project, err := s.store.UpdateProjectSettings(projectID, sqlitecli.ProjectSettingsUpdate{
		ContextPolicy:      input.ContextPolicy,
		HandoffPolicy:      input.HandoffPolicy,
		RoutePolicy:        input.RoutePolicy,
		ToolApprovalPolicy: input.ToolApprovalPolicy,
		KbScopePolicy:      input.KbScopePolicy,
	})
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"project": project})
}

func (s Server) validateProjectSettings(contextPolicy, handoffPolicy, routePolicy, toolApprovalPolicy, kbScopePolicy string) error {
	if contextPolicy != "" && !containsString(s.packetPolicies.List(), contextPolicy) {
		return fmt.Errorf("unknown contextPolicy: %s", contextPolicy)
	}
	if handoffPolicy != "" && !containsString(s.handoffPolicies.List(), handoffPolicy) {
		return fmt.Errorf("unknown handoffPolicy: %s", handoffPolicy)
	}
	if routePolicy != "" && !containsString(s.routePolicies.List(), routePolicy) {
		return fmt.Errorf("unknown routePolicy: %s", routePolicy)
	}
	if toolApprovalPolicy != "" && !s.isValidToolApprovalPolicy(toolApprovalPolicy) {
		return fmt.Errorf("unknown toolApprovalPolicy: %s", toolApprovalPolicy)
	}
	if kbScopePolicy != "" && !s.isValidKbScopePolicy(kbScopePolicy) {
		return fmt.Errorf("unknown kbScopePolicy: %s", kbScopePolicy)
	}
	return nil
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func (s Server) isValidToolApprovalPolicy(name string) bool {
	for _, e := range s.toolApprovalPolicy {
		if e.Name == name {
			return true
		}
	}
	return false
}

func (s Server) isValidKbScopePolicy(name string) bool {
	for _, e := range s.kbScopePolicy {
		if e.Name == name {
			return true
		}
	}
	return false
}

func (s Server) searchSessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.store.SearchSessions(r.URL.Query().Get("q"))
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"sessions": sessions})
}

func (s Server) renameSession(w http.ResponseWriter, r *http.Request) {
	sessionID, ok := sessionIDFromPath(r.URL.Path)
	if !ok {
		writeError(w, errors.New("session id is required"), http.StatusBadRequest)
		return
	}
	var input struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	session, err := s.store.RenameSession(sessionID, input.Title)
	if err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"session": session})
}

func (s Server) setSessionActiveRoute(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(r.PathValue("sessionID"))
	if sessionID == "" {
		writeError(w, errors.New("session id is required"), http.StatusBadRequest)
		return
	}
	var input struct {
		RouteID string `json:"routeId"`
		ModelID string `json:"modelId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	if err := s.store.SetSessionActiveRoute(sessionID, input.RouteID, input.ModelID); err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	session, _, err := s.store.GetSession(sessionID)
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"activeRouteId": session.ActiveRouteID,
		"activeModelId": session.ActiveModelID,
	})
}

func (s Server) updateSessionProviders(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(r.PathValue("sessionID"))
	if sessionID == "" {
		writeError(w, errors.New("session id is required"), http.StatusBadRequest)
		return
	}
	var input struct {
		ProviderGroupIDs []string `json:"providerGroupIds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	if _, _, err := s.store.GetSession(sessionID); err != nil {
		writeError(w, err, http.StatusNotFound)
		return
	}
	if err := s.store.SetSessionProviderGroups(sessionID, compactStrings(input.ProviderGroupIDs)); err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	groups, err := s.store.ListSessionProviderGroups(sessionID)
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"providerGroupIds": groups})
}

func (s Server) recordSteering(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(r.PathValue("sessionID"))
	if sessionID == "" {
		writeError(w, errors.New("session id is required"), http.StatusBadRequest)
		return
	}
	var input struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	content := strings.TrimSpace(input.Content)
	if content == "" {
		writeError(w, errors.New("steering content is required"), http.StatusBadRequest)
		return
	}
	record, run, segment, err := s.addSteeringToActiveRun(sessionID, content)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, sqlitecli.ErrNotFound) {
			status = http.StatusConflict
		}
		writeError(w, err, status)
		return
	}
	writeJSON(w, map[string]any{"steering": record, "chatRun": run, "providerSegment": segment})
}

func (s *Server) cancelSessionRun(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(r.PathValue("sessionID"))
	if sessionID == "" {
		writeError(w, errors.New("session id is required"), http.StatusBadRequest)
		return
	}
	s.cancelActiveRun(sessionID)
	writeJSON(w, map[string]any{"cancelled": true})
}

func (s *Server) addSteeringToActiveRun(sessionID string, content string) (sqlitecli.SteeringRecord, core.ChatRun, core.ProviderSegment, error) {
	run, err := s.store.GetActiveChatRun(sessionID)
	if err != nil {
		return sqlitecli.SteeringRecord{}, core.ChatRun{}, core.ProviderSegment{}, err
	}
	segment, err := s.store.ActiveProviderSegment(run.ID)
	if err != nil && !errors.Is(err, sqlitecli.ErrNotFound) {
		return sqlitecli.SteeringRecord{}, core.ChatRun{}, core.ProviderSegment{}, err
	}
	event := s.recordContextEventAfter(sessionID, core.EventSteeringAdded, "steering:"+run.ID, run.InputEventID)
	record, err := s.store.RecordSteering(sqlitecli.SteeringInput{
		ChatRunID:         run.ID,
		ProviderSegmentID: segment.ID,
		EventID:           event.ID,
		Content:           content,
		Status:            "recorded",
	})
	if err != nil {
		return sqlitecli.SteeringRecord{}, core.ChatRun{}, core.ProviderSegment{}, err
	}
	s.cancelActiveRun(sessionID)
	return record, run, segment, nil
}

func (s Server) listKnowledge(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	scope := core.KnowledgeScope(strings.TrimSpace(r.URL.Query().Get("scope")))
	items, err := s.store.SearchKnowledge(sqlitecli.KnowledgeSearchOptions{
		Query: q,
		Scope: scope,
		Limit: 20,
	})
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"items": items})
}

func (s Server) addKnowledge(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Scope     string `json:"scope"`
		Kind      string `json:"kind"`
		Title     string `json:"title"`
		ProjectID string `json:"projectId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	item, err := s.store.AddKnowledgeItem(core.KnowledgeItem{
		Scope:     core.KnowledgeScope(strings.TrimSpace(input.Scope)),
		Kind:      strings.TrimSpace(input.Kind),
		Title:     strings.TrimSpace(input.Title),
		ProjectID: strings.TrimSpace(input.ProjectID),
	})
	if err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"item": item})
}

func (s Server) startParallelRun(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(r.PathValue("sessionID"))
	if sessionID == "" {
		writeError(w, errors.New("session id is required"), http.StatusBadRequest)
		return
	}
	var input struct {
		Content        string `json:"content"`
		RouteID        string `json:"routeId"`
		ModelID        string `json:"modelId"`
		ThinkingEffort string `json:"thinkingEffort"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	content := strings.TrimSpace(input.Content)
	if content == "" {
		writeError(w, errors.New("content is required"), http.StatusBadRequest)
		return
	}
	session, _, err := s.store.GetSession(sessionID)
	if err != nil {
		writeError(w, err, http.StatusNotFound)
		return
	}
	selection, err := s.resolveChatSelection(sessionID, input.RouteID, input.ModelID)
	if err != nil {
		writeError(w, fmt.Errorf("cannot resolve provider for parallel run: %w", err), http.StatusBadRequest)
		return
	}
	packet := s.buildContextPacket(sessionID, content, selection, "parallel run")
	if _, err := s.store.SaveContextPacket(packet); err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	triggerEventID := s.latestUserTriggerEventID(sessionID)
	run, err := s.store.StartChatRun(sqlitecli.ChatRunInput{
		ProjectID:       session.ProjectID,
		SessionID:       sessionID,
		BranchID:        "main",
		Role:            core.ChatRunRoleParallel,
		Status:          core.ChatRunRunning,
		InputEventID:    triggerEventID,
		ContextPacketID: packet.ID,
	})
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	candidate, err := s.store.AddCandidateOutput(sqlitecli.CandidateOutputInput{
		ChatRunID:      run.ID,
		SessionID:      sessionID,
		BranchID:       "main",
		TriggerEventID: triggerEventID,
		Content:        "",
		Status:         core.CandidateOutputPending,
	})
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	thinkingEffort := strings.TrimSpace(input.ThinkingEffort)
	go func() {
		seg := s.startProviderSegment(run.ID, selection, "")
		text, _, runErr := s.runAssistant(context.Background(), sessionID, content, thinkingEffort, selection, func(event provider.Event) {})
		segStatus := core.ProviderSegmentCompleted
		candidateStatus := core.CandidateOutputReady
		if runErr != nil {
			segStatus = core.ProviderSegmentFailed
			candidateStatus = core.CandidateOutputFailed
			text = runErr.Error()
		}
		s.completeProviderSegment(seg.ID, segStatus, "")
		s.store.UpdateCandidateOutput(candidate.ID, text, candidateStatus)
		runStatus := core.ChatRunCompleted
		if runErr != nil {
			runStatus = core.ChatRunFailed
		}
		s.store.CompleteChatRun(run.ID, runStatus, "")
	}()
	writeJSON(w, map[string]any{"candidateId": candidate.ID, "chatRunId": run.ID})
}

func (s Server) updateCandidateOutput(w http.ResponseWriter, r *http.Request) {
	candidateID := strings.TrimSpace(r.PathValue("candidateID"))
	if candidateID == "" {
		writeError(w, errors.New("candidate id is required"), http.StatusBadRequest)
		return
	}
	var input struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	status := core.CandidateOutputStatus(strings.TrimSpace(input.Status))
	switch status {
	case core.CandidateOutputAccepted, core.CandidateOutputRejected:
	default:
		writeError(w, errors.New("candidate status must be accepted or rejected"), http.StatusBadRequest)
		return
	}
	if status == core.CandidateOutputAccepted {
		result, err := s.store.MergeCandidateOutput(candidateID)
		if err != nil {
			writeError(w, err, http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{
			"candidate":              result.Candidate,
			"message":                result.Message,
			"event":                  result.Event,
			"supersededCandidateIds": result.SupersededCandidateIDs,
		})
		return
	}
	candidate, err := s.store.UpdateCandidateOutputStatus(candidateID, status)
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"candidate": candidate})
}

func (s Server) deleteSession(w http.ResponseWriter, r *http.Request) {
	sessionID, ok := sessionIDFromPath(r.URL.Path)
	if !ok {
		writeError(w, errors.New("session id is required"), http.StatusBadRequest)
		return
	}
	if err := s.store.DeleteSession(sessionID); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

func (s Server) connectProviderProfile(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ProviderPluginID string `json:"providerPluginId"`
		DisplayName      string `json:"displayName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	displayName, detail, err := detectProviderAccount(input.ProviderPluginID, input.DisplayName)
	if err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	profile, err := s.store.UpsertProviderProfile(input.ProviderPluginID, displayName)
	if err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"profile": profile, "detail": detail})
}

func (s Server) projectRoute(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(strings.TrimRight(r.URL.Path, "/"), "/moderator") {
		if r.Method == http.MethodPost {
			s.updateProjectModerator(w, r)
			return
		}
		writeError(w, errors.New("method not allowed"), http.StatusMethodNotAllowed)
		return
	}
	if r.Method == http.MethodPost {
		s.addProjectRoute(w, r)
		return
	}
	if r.Method == http.MethodDelete {
		if _, _, ok := projectRouteIDsFromPath(r.URL.Path); !ok {
			s.deleteProject(w, r)
			return
		}
		s.removeProjectRoute(w, r)
		return
	}
	writeError(w, errors.New("method not allowed"), http.StatusMethodNotAllowed)
}

func (s Server) deleteProject(w http.ResponseWriter, r *http.Request) {
	projectID, ok := projectIDFromPath(r.URL.Path)
	if !ok {
		writeError(w, errors.New("project id is required"), http.StatusBadRequest)
		return
	}
	if err := s.store.DeleteProject(projectID); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

func (s Server) updateProjectModerator(w http.ResponseWriter, r *http.Request) {
	projectID, ok := projectModeratorProjectIDFromPath(r.URL.Path)
	if !ok {
		writeError(w, errors.New("project moderator path is invalid"), http.StatusBadRequest)
		return
	}
	var input struct {
		Mode                     string `json:"mode"`
		PrimaryProviderGroupID   string `json:"primaryProviderGroupId"`
		SecondaryProviderGroupID string `json:"secondaryProviderGroupId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	pref, err := s.store.UpsertProjectModeratorPreference(
		projectID,
		input.Mode,
		input.PrimaryProviderGroupID,
		input.SecondaryProviderGroupID,
	)
	if err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"moderatorPreference": pref})
}

func (s Server) addProjectRoute(w http.ResponseWriter, r *http.Request) {
	projectID, ok := projectRouteProjectIDFromPath(r.URL.Path)
	if !ok {
		writeError(w, errors.New("project route path is invalid"), http.StatusBadRequest)
		return
	}
	var input struct {
		RouteID string `json:"routeId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	if err := s.store.AddProjectAccessRoute(projectID, input.RouteID); err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

func (s Server) removeProjectRoute(w http.ResponseWriter, r *http.Request) {
	projectID, routeID, ok := projectRouteIDsFromPath(r.URL.Path)
	if !ok {
		writeError(w, errors.New("project route path is invalid"), http.StatusBadRequest)
		return
	}
	if err := s.store.RemoveProjectAccessRoute(projectID, routeID); err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

func (s Server) createSession(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Title            string   `json:"title"`
		ProjectID        string   `json:"projectId"`
		ProviderGroupIDs []string `json:"providerGroupIds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	session, err := s.store.CreateChatSessionForProject(input.ProjectID, input.Title)
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	providerGroupIDs := compactStrings(input.ProviderGroupIDs)
	if len(providerGroupIDs) > 0 {
		if err := s.store.SetSessionProviderGroups(session.ID, providerGroupIDs); err != nil {
			writeError(w, err, http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w, map[string]any{"session": session})
}

func (s Server) runTerminalCommand(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Command    string `json:"command"`
		SessionID  string `json:"sessionId"`
		WorkingDir string `json:"workingDir"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	command := strings.TrimSpace(input.Command)
	if command == "" {
		writeError(w, errors.New("command is required"), http.StatusBadRequest)
		return
	}

	project, err := s.store.DefaultProject()
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	workingDir := project.RootPath
	if strings.TrimSpace(input.WorkingDir) != "" {
		workingDir = expandPath(input.WorkingDir)
	}
	if strings.TrimSpace(workingDir) == "" {
		workingDir, err = os.Getwd()
		if err != nil {
			writeError(w, err, http.StatusInternalServerError)
			return
		}
	}
	workingDir = filepath.Clean(workingDir)
	info, err := os.Stat(workingDir)
	if err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	if !info.IsDir() {
		writeError(w, errors.New("working directory is not a directory"), http.StatusBadRequest)
		return
	}

	startedAt := time.Now().UTC().Format(time.RFC3339Nano)
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	executor := toolruntime.ShellExecutor{WorkingDir: workingDir}
	result, _ := executor.Execute(ctx, toolruntime.Request{
		ToolID:       "shell.zsh",
		InvocationID: "terminal_" + time.Now().UTC().Format("20060102150405.000000000"),
		Command:      command,
	}, func(toolruntime.Event) {})

	run, err := s.store.AddCommandRun(sqlitecli.CommandRun{
		ProjectID:  project.ID,
		SessionID:  input.SessionID,
		Command:    command,
		WorkingDir: workingDir,
		Status:     result.Status,
		ExitCode:   result.ExitCode,
		Stdout:     result.Stdout,
		Stderr:     result.Stderr,
		StartedAt:  startedAt,
		FinishedAt: time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"run": run})
}

func (s Server) readFile(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	filePath := strings.TrimSpace(input.Path)
	if filePath == "" {
		writeError(w, errors.New("path is required"), http.StatusBadRequest)
		return
	}
	filePath = expandPath(filePath)
	if !filepath.IsAbs(filePath) {
		project, err := s.store.DefaultProject()
		if err != nil {
			writeError(w, err, http.StatusInternalServerError)
			return
		}
		base := strings.TrimSpace(project.RootPath)
		if base == "" {
			base, err = os.Getwd()
			if err != nil {
				writeError(w, err, http.StatusInternalServerError)
				return
			}
		}
		filePath = filepath.Join(base, filePath)
	}
	cleanPath := filepath.Clean(filePath)
	info, err := os.Stat(cleanPath)
	if err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	if info.IsDir() {
		writeError(w, errors.New("path is a directory"), http.StatusBadRequest)
		return
	}
	const maxFileSize = 512 * 1024
	if info.Size() > maxFileSize {
		writeError(w, fmt.Errorf("file is too large for preview: %d bytes", info.Size()), http.StatusBadRequest)
		return
	}
	content, err := os.ReadFile(cleanPath)
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"path":    cleanPath,
		"content": string(content),
		"size":    info.Size(),
	})
}

func (s Server) resolveToolApproval(w http.ResponseWriter, r *http.Request) {
	approvalID := strings.TrimPrefix(r.URL.Path, "/api/tool-approvals/")
	approvalID = strings.Trim(approvalID, "/")
	if approvalID == "" {
		writeError(w, errors.New("approval id is required"), http.StatusBadRequest)
		return
	}
	var input struct {
		Decision string `json:"decision"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	decision := strings.TrimSpace(input.Decision)
	if decision == "" {
		decision = "decline"
	}
	if !s.approvals.resolve(approvalID, decision) {
		writeError(w, errors.New("approval request is no longer pending"), http.StatusNotFound)
		return
	}
	writeJSON(w, map[string]string{"status": "recorded", "decision": decision})
}

func (s Server) session(w http.ResponseWriter, r *http.Request) {
	sessionID, ok := sessionIDFromPath(r.URL.Path)
	if !ok {
		writeError(w, errors.New("session id is required"), http.StatusBadRequest)
		return
	}
	session, messages, err := s.store.GetSession(sessionID)
	if err != nil {
		writeError(w, err, http.StatusNotFound)
		return
	}
	providerGroupIDs, err := s.store.ListSessionProviderGroups(sessionID)
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	events, err := s.store.ListMessageEvents(sessionID)
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	providerChats, err := s.store.ListProviderChatBindings(sessionID)
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	queueItems, err := s.store.ListPendingQueueItems(sessionID)
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	candidateOutputs, err := s.store.ListPendingCandidateOutputs(sessionID)
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"session":          session,
		"messages":         messages,
		"events":           events,
		"providerChats":    providerChats,
		"queueItems":       queueItems,
		"candidateOutputs": candidateOutputs,
		"context":          contextUsageOrZero(s.store, sessionID),
		"providerGroupIds": providerGroupIDs,
	})
}

func (s Server) sessionQueue(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(r.PathValue("sessionID"))
	if sessionID == "" {
		writeError(w, errors.New("session id is required"), http.StatusBadRequest)
		return
	}
	if _, _, err := s.store.GetSession(sessionID); err != nil {
		writeError(w, err, http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		items, err := s.store.ListPendingQueueItems(sessionID)
		if err != nil {
			writeError(w, err, http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"queueItems": items})
	case http.MethodPost:
		var input struct {
			Content        string `json:"content"`
			Mode           string `json:"mode"`
			RouteID        string `json:"routeId"`
			ModelID        string `json:"modelId"`
			ThinkingEffort string `json:"thinkingEffort"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, err, http.StatusBadRequest)
			return
		}
		item, err := s.store.AddQueueItem(sqlitecli.QueueItemInput{
			SessionID:      sessionID,
			BranchID:       "main",
			Content:        input.Content,
			Mode:           core.QueueItemMode(strings.TrimSpace(input.Mode)),
			RouteID:        input.RouteID,
			ModelID:        input.ModelID,
			ThinkingEffort: input.ThinkingEffort,
		})
		if err != nil {
			writeError(w, err, http.StatusBadRequest)
			return
		}
		s.recordContextEvent(sessionID, core.EventQueueItemCreated, "queue:"+item.ID)
		writeJSON(w, map[string]any{"queueItem": item})
	case http.MethodPatch:
		var input struct {
			ItemIDs []string `json:"itemIds"`
			ItemID  string   `json:"itemId"`
			Status  string   `json:"status"`
			Mode    string   `json:"mode"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, err, http.StatusBadRequest)
			return
		}
		if len(input.ItemIDs) > 0 {
			if err := s.store.ReorderQueueItems(sessionID, input.ItemIDs); err != nil {
				writeError(w, err, http.StatusBadRequest)
				return
			}
			s.recordContextEvent(sessionID, core.EventQueueItemReordered, "queue:reorder")
		}
		var item core.QueueItem
		if strings.TrimSpace(input.ItemID) != "" {
			if strings.TrimSpace(input.Mode) != "" {
				updated, err := s.store.UpdateQueueItemMode(input.ItemID, core.QueueItemMode(strings.TrimSpace(input.Mode)))
				if err != nil {
					writeError(w, err, http.StatusBadRequest)
					return
				}
				item = updated
			} else {
				status := core.QueueItemStatus(strings.TrimSpace(input.Status))
				if status == "" {
					status = core.QueueItemConsumed
				}
				updated, err := s.store.UpdateQueueItemStatus(input.ItemID, status)
				if err != nil {
					writeError(w, err, http.StatusBadRequest)
					return
				}
				item = updated
			}
		}
		items, err := s.store.ListPendingQueueItems(sessionID)
		if err != nil {
			writeError(w, err, http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"queueItem": item, "queueItems": items})
	default:
		writeError(w, errors.New("method not allowed"), http.StatusMethodNotAllowed)
	}
}

func (s Server) sessionMessage(w http.ResponseWriter, r *http.Request) {
	if sessionID, ok := streamSessionIDFromPath(r.URL.Path); ok {
		s.streamSessionMessage(w, r, sessionID)
		return
	}

	sessionID, ok := messageSessionIDFromPath(r.URL.Path)
	if !ok {
		writeError(w, errors.New("session message path is invalid"), http.StatusBadRequest)
		return
	}

	var input struct {
		Content        string `json:"content"`
		RouteID        string `json:"routeId"`
		ModelID        string `json:"modelId"`
		ThinkingEffort string `json:"thinkingEffort"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	content := strings.TrimSpace(input.Content)
	if content == "" {
		writeError(w, errors.New("message content is required"), http.StatusBadRequest)
		return
	}
	filtered, err := s.filterChatInput(r.Context(), sessionID, chatfilter.Input{
		SessionID:      sessionID,
		Content:        content,
		RouteID:        input.RouteID,
		ModelID:        input.ModelID,
		ThinkingEffort: input.ThinkingEffort,
	})
	if err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	content = filtered.Content

	selection, err := s.resolveChatSelection(sessionID, input.RouteID, input.ModelID)
	if err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}

	userMessage, err := s.store.AddMessage(sessionID, "user", content)
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	selection, _, err = s.moderatedSelectionForActiveChat(sessionID, selection)
	if err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	assistantMessage, err := s.store.AddMessage(sessionID, "assistant", localUnavailableMessage(selection))
	if err != nil {
		writeError(w, err, http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"messages": []any{userMessage, assistantMessage},
	})
}

func (s Server) filterChatInput(ctx context.Context, sessionID string, input chatfilter.Input) (chatfilter.Result, error) {
	input.SessionID = sessionID
	result, err := s.filters.Apply(ctx, input)
	if err != nil {
		return chatfilter.Result{}, err
	}
	if result.Decision == chatfilter.DecisionBlock {
		if strings.TrimSpace(result.Reason) == "" {
			result.Reason = "message blocked by Ergo Loom input filter"
		}
		return result, errors.New(result.Reason)
	}
	if strings.TrimSpace(result.Content) == "" {
		return result, errors.New("message content is required")
	}
	return result, nil
}

func (s Server) streamSessionMessage(w http.ResponseWriter, r *http.Request, sessionID string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, errors.New("streaming is not supported"), http.StatusInternalServerError)
		return
	}

	var input struct {
		Content        string `json:"content"`
		Mode           string `json:"mode"`
		RouteID        string `json:"routeId"`
		ModelID        string `json:"modelId"`
		ThinkingEffort string `json:"thinkingEffort"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	content := strings.TrimSpace(input.Content)
	if content == "" {
		writeError(w, errors.New("message content is required"), http.StatusBadRequest)
		return
	}
	filtered, err := s.filterChatInput(r.Context(), sessionID, chatfilter.Input{
		SessionID:      sessionID,
		Content:        content,
		RouteID:        input.RouteID,
		ModelID:        input.ModelID,
		ThinkingEffort: input.ThinkingEffort,
	})
	if err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}
	content = filtered.Content

	mode := core.QueueItemMode(strings.TrimSpace(input.Mode))
	if mode == "" {
		mode = core.QueueItemNormal
	}
	activeRun, activeErr := s.store.GetActiveChatRun(sessionID)
	if activeErr != nil && !errors.Is(activeErr, sqlitecli.ErrNotFound) {
		writeError(w, activeErr, http.StatusInternalServerError)
		return
	}
	if activeErr == nil {
		w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		if mode == core.QueueItemSteering || strings.TrimSpace(input.Mode) == "steer" {
			record, run, segment, err := s.addSteeringToActiveRun(sessionID, content)
			if err != nil {
				writeStreamEvent(w, "error", map[string]string{"message": err.Error()})
				flusher.Flush()
				return
			}
			writeStreamEvent(w, "steering", map[string]any{
				"steering":        record,
				"chatRun":         run,
				"providerSegment": segment,
			})
			flusher.Flush()
			return
		}
		item, err := s.store.AddQueueItem(sqlitecli.QueueItemInput{
			SessionID:      sessionID,
			BranchID:       activeRun.BranchID,
			Content:        content,
			Mode:           core.QueueItemNormal,
			RouteID:        input.RouteID,
			ModelID:        input.ModelID,
			ThinkingEffort: input.ThinkingEffort,
		})
		if err != nil {
			writeStreamEvent(w, "error", map[string]string{"message": err.Error()})
			flusher.Flush()
			return
		}
		s.recordContextEvent(sessionID, core.EventQueueItemCreated, "queue:"+item.ID)
		writeStreamEvent(w, "queued", map[string]any{"queueItem": item, "activeChatRun": activeRun})
		flusher.Flush()
		return
	}

	selection, err := s.resolveChatSelection(sessionID, input.RouteID, input.ModelID)
	if err != nil {
		writeError(w, err, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")

	_ = s.executeMainRun(r.Context(), RunRequest{
		SessionID:      sessionID,
		Content:        content,
		ThinkingEffort: input.ThinkingEffort,
		Selection:      selection,
	}, func(kind string, payload any) {
		writeStreamEvent(w, kind, payload)
		flusher.Flush()
	})
}

func (s *Server) registerSessionCancel(sessionID string, cancel context.CancelFunc) {
	s.sessionCancelMu.Lock()
	defer s.sessionCancelMu.Unlock()
	if prev, ok := s.sessionCancels[sessionID]; ok {
		prev()
	}
	s.sessionCancels[sessionID] = cancel
}

func (s *Server) unregisterSessionCancel(sessionID string) {
	s.sessionCancelMu.Lock()
	defer s.sessionCancelMu.Unlock()
	delete(s.sessionCancels, sessionID)
}

func (s *Server) cancelActiveRun(sessionID string) {
	s.sessionCancelMu.Lock()
	defer s.sessionCancelMu.Unlock()
	if cancel, ok := s.sessionCancels[sessionID]; ok {
		cancel()
	}
}

func (s *Server) addSessionExcludedRoute(sessionID, routeID string) {
	if sessionID == "" || routeID == "" {
		return
	}
	s.sessionExcludedMu.Lock()
	defer s.sessionExcludedMu.Unlock()
	if s.sessionExcluded[sessionID] == nil {
		s.sessionExcluded[sessionID] = make(map[string]bool)
	}
	s.sessionExcluded[sessionID][routeID] = true
}

func (s *Server) sessionExcludedRoutes(sessionID string) map[string]bool {
	s.sessionExcludedMu.Lock()
	defer s.sessionExcludedMu.Unlock()
	result := make(map[string]bool)
	for k, v := range s.sessionExcluded[sessionID] {
		result[k] = v
	}
	return result
}

func (s *Server) executeMainRun(ctx context.Context, req RunRequest, onEvent func(kind string, payload any)) error {
	if onEvent == nil {
		onEvent = func(string, any) {}
	}
	sessionID := strings.TrimSpace(req.SessionID)
	content := strings.TrimSpace(req.Content)
	if sessionID == "" {
		return errors.New("session id is required")
	}
	if content == "" {
		return errors.New("message content is required")
	}

	ctx, cancel := context.WithCancel(ctx)
	s.registerSessionCancel(sessionID, cancel)
	defer func() {
		cancel()
		s.unregisterSessionCancel(sessionID)
	}()

	userMessage, err := s.store.AddMessage(sessionID, "user", content)
	if err != nil {
		onEvent("error", map[string]string{"message": err.Error()})
		return err
	}
	onEvent("message", userMessage)

	assistantActivityIndex := s.nextAssistantActivityIndex(sessionID)
	onEvent("assistant_start", map[string]string{"role": "assistant"})
	s.recordMessageEvent(sessionID, assistantActivityIndex, "status", map[string]string{"text": "Started assistant reply"})

	selection, handoff, err := s.moderatedSelectionForActiveChat(sessionID, req.Selection)
	if err != nil {
		onEvent("error", map[string]string{"message": err.Error()})
		return err
	}
	contextNote := strings.TrimSpace(req.ContextNote)
	if handoff != nil {
		contextNote = strings.TrimSpace(strings.Join(compactStrings([]string{contextNote, handoff.Reason}), " · "))
		payload := map[string]string{"text": handoff.Reason}
		onEvent("assistant_status", payload)
		s.recordMessageEvent(sessionID, assistantActivityIndex, "status", payload)
	}

	chatRun, providerSegment, assistantInput, err := s.prepareMainRun(sessionID, content, selection, contextNote, assistantActivityIndex, onEvent)
	if err != nil {
		return err
	}
	onAssistantEvent := func(event provider.Event) {
		switch event.Kind {
		case provider.EventKindDelta:
			onEvent("assistant_delta", map[string]string{"text": event.Text})
		case provider.EventKindStatus:
			payload := map[string]string{"text": event.Text}
			onEvent("assistant_status", payload)
			s.recordMessageEvent(sessionID, assistantActivityIndex, "status", payload)
		case provider.EventKindToolStart, provider.EventKindToolResult, provider.EventKindApprovalRequest, provider.EventKindToolError, provider.EventKindTurnAborted:
			payload := toolEventPayloadForSession(sessionID, event)
			onEvent(string(event.Kind), payload)
			messageEvent := s.recordMessageEvent(sessionID, assistantActivityIndex, streamEventKindToActivityKind(event.Kind), payload)
			s.recordProviderToolEvent(sessionID, event, messageEvent.ID)
		default:
			return
		}
	}

	assistantContent, streamed, err := s.runAssistant(ctx, sessionID, assistantInput, req.ThinkingEffort, selection, onAssistantEvent)
	if err != nil {
		reason := segmentEndReason(err)
		s.recordContextEvent(sessionID, core.EventProviderRunCompleted, providerRunPayloadRef(selection, string(reason)))
		s.completeProviderSegment(providerSegment.ID, core.ProviderSegmentFailed, "")
		s.recordTokenUsage(sessionID, selection, assistantInput, "", "error")
		decision := s.moderationDecision(sessionID, providerSegment, reason)
		if decision.Action == core.ActionFailover {
			s.addSessionExcludedRoute(sessionID, selection.Route.ID)
			nextSelection, failoverErr := s.resolveChatSelectionExcluding(sessionID, selection.Route.ID)
			if failoverErr == nil {
				selection = nextSelection
				contextNote = "Failover after " + string(reason)
				statusPayload := map[string]string{"text": "Failing over to " + selection.Route.DisplayName + " / " + selection.Model.DisplayName}
				onEvent("assistant_status", statusPayload)
				s.recordMessageEvent(sessionID, assistantActivityIndex, "status", statusPayload)
				providerSegment, assistantInput, err = s.prepareMainRunForExistingChatRun(chatRun, sessionID, content, selection, contextNote, assistantActivityIndex, onEvent)
				if err == nil {
					assistantContent, streamed, err = s.runAssistant(ctx, sessionID, assistantInput, req.ThinkingEffort, selection, onAssistantEvent)
				}
			} else {
				err = fmt.Errorf("%w; failover unavailable: %v", err, failoverErr)
			}
		}
		if err != nil {
			reason := segmentEndReason(err)
			s.recordContextEvent(sessionID, core.EventProviderRunCompleted, providerRunPayloadRef(selection, string(reason)))
			s.completeProviderSegment(providerSegment.ID, core.ProviderSegmentFailed, "")
			s.completeChatRun(chatRun.ID, core.ChatRunFailed, "")
			payload := map[string]string{"message": err.Error(), "reason": string(reason), "action": string(decision.Action)}
			onEvent("error", payload)
			s.recordMessageEvent(sessionID, assistantActivityIndex, "error", map[string]string{"text": err.Error(), "toolName": "chat", "reason": string(reason), "action": string(decision.Action)})
			s.recordTokenUsage(sessionID, selection, assistantInput, "", "error")
			return err
		}
	}
	if strings.TrimSpace(assistantContent) == "" {
		s.recordContextEvent(sessionID, core.EventProviderRunCompleted, providerRunPayloadRef(selection, "empty"))
		s.completeProviderSegment(providerSegment.ID, core.ProviderSegmentFailed, "")
		s.completeChatRun(chatRun.ID, core.ChatRunFailed, "")
		payload := map[string]string{"message": "assistant returned an empty response"}
		onEvent("error", payload)
		s.recordMessageEvent(sessionID, assistantActivityIndex, "error", map[string]string{"text": payload["message"], "toolName": "chat"})
		return errors.New(payload["message"])
	}

	if !streamed {
		emitTextChunks(ctx, assistantContent, onEvent)
	}

	s.recordContextEvent(sessionID, core.EventProviderRunCompleted, providerRunPayloadRef(selection, "completed"))
	s.completeProviderSegment(providerSegment.ID, core.ProviderSegmentCompleted, "")
	assistantMessage, err := s.store.AddMessage(sessionID, "assistant", assistantContent)
	if err != nil {
		onEvent("error", map[string]string{"message": err.Error()})
		return err
	}
	outputEventID := s.currentHeadEventID(sessionID)
	s.completeChatRun(chatRun.ID, core.ChatRunCompleted, outputEventID)
	s.recordTokenUsage(sessionID, selection, assistantInput, assistantContent, "success")
	onEvent("assistant_done", assistantMessage)
	s.maybeConsumeQueue(sessionID)
	return nil
}

const budgetWarnRatio = 0.80

func (s Server) prepareMainRun(sessionID string, content string, selection chatSelection, contextNote string, activityIndex int, onEvent func(string, any)) (core.ChatRun, core.ProviderSegment, string, error) {
	s.maybeGenerateHandoffSummary(sessionID, selection)
	contextPacket := s.buildContextPacket(sessionID, content, selection, contextNote)
	if _, err := s.store.SaveContextPacket(contextPacket); err != nil {
		payload := map[string]string{"text": "Context packet persistence failed: " + err.Error()}
		onEvent("assistant_status", payload)
		s.recordMessageEvent(sessionID, activityIndex, "status", payload)
	}
	s.maybeWarnBudget(sessionID, contextPacket, selection)
	assistantInput := contextPacket.Content
	packetPayload := map[string]string{"text": "Prepared Ergo Loom local context packet"}
	onEvent("assistant_status", packetPayload)
	s.recordMessageEvent(sessionID, activityIndex, "status", packetPayload)
	chatRun := s.startMainChatRun(sessionID, contextPacket)
	providerSegment := s.startProviderSegment(chatRun.ID, selection, contextNote)
	s.recordContextEvent(sessionID, core.EventProviderRunStarted, providerRunPayloadRef(selection, "started"))
	return chatRun, providerSegment, assistantInput, nil
}

func (s Server) prepareMainRunForExistingChatRun(chatRun core.ChatRun, sessionID string, content string, selection chatSelection, contextNote string, activityIndex int, onEvent func(string, any)) (core.ProviderSegment, string, error) {
	s.maybeGenerateHandoffSummary(sessionID, selection)
	contextPacket := s.buildContextPacket(sessionID, content, selection, contextNote)
	if _, err := s.store.SaveContextPacket(contextPacket); err != nil {
		payload := map[string]string{"text": "Context packet persistence failed: " + err.Error()}
		onEvent("assistant_status", payload)
		s.recordMessageEvent(sessionID, activityIndex, "status", payload)
	}
	s.maybeWarnBudget(sessionID, contextPacket, selection)
	providerSegment := s.startProviderSegment(chatRun.ID, selection, contextNote)
	s.recordContextEvent(sessionID, core.EventProviderRunStarted, providerRunPayloadRef(selection, "started"))
	return providerSegment, contextPacket.Content, nil
}

func (s Server) maybeWarnBudget(sessionID string, packet core.ContextPacket, selection chatSelection) {
	budget := selection.Route.ContextBudgetChars
	if budget <= 0 {
		return
	}
	if float64(len(packet.Content)) > budgetWarnRatio*float64(budget) {
		s.moderator.OnBudgetWarning(core.ModerationContext{})
	}
}

func segmentEndReason(err error) core.SegmentEndReason {
	var driverErr *provider.DriverError
	if !errors.As(err, &driverErr) {
		return core.ReasonError
	}
	switch driverErr.Kind {
	case provider.ErrKindAuthFailure:
		return core.ReasonAuthFailure
	case provider.ErrKindRateLimit, provider.ErrKindTransient:
		return core.ReasonTimeout
	case provider.ErrKindSessionEnd:
		return core.ReasonSessionEnd
	case provider.ErrKindUnavailable, provider.ErrKindFatal:
		return core.ReasonError
	default:
		return core.ReasonError
	}
}

func (s Server) moderationDecision(sessionID string, segment core.ProviderSegment, reason core.SegmentEndReason) core.ModerationDecision {
	if s.moderator == nil {
		return core.ModerationDecision{Action: core.ActionSuspend}
	}
	session, _, err := s.store.GetSession(sessionID)
	if err != nil {
		return core.ModerationDecision{Action: core.ActionSuspend}
	}
	queueDepth := 0
	if items, err := s.store.ListPendingQueueItems(sessionID); err == nil {
		queueDepth = len(items)
	}
	return s.moderator.OnSegmentEnd(core.ModerationContext{
		Session:       session,
		ActiveSegment: segment,
		Reason:        reason,
		QueueDepth:    queueDepth,
	})
}

func (s *Server) maybeConsumeQueue(sessionID string) {
	if strings.TrimSpace(sessionID) == "" {
		return
	}
	item, err := s.store.ConsumeNextQueueItem(sessionID)
	if err != nil {
		return
	}
	switch item.Mode {
	case core.QueueItemParallel:
		go func() {
			_ = s.startParallelRunFromQueueItem(item)
		}()
	case core.QueueItemSteering:
		s.cancelActiveRun(sessionID)
		req, err := s.runRequestFromQueueItem(item)
		if err != nil {
			return
		}
		go func() {
			_ = s.executeMainRun(context.Background(), req, func(string, any) {})
		}()
	default:
		req, err := s.runRequestFromQueueItem(item)
		if err != nil {
			return
		}
		go func() {
			_ = s.executeMainRun(context.Background(), req, func(string, any) {})
		}()
	}
}

func (s *Server) startParallelRunFromQueueItem(item core.QueueItem) error {
	session, _, err := s.store.GetSession(item.SessionID)
	if err != nil {
		return err
	}
	selection, err := s.resolveChatSelection(item.SessionID, item.RouteID, item.ModelID)
	if err != nil {
		return err
	}
	packet := s.buildContextPacket(item.SessionID, item.Content, selection, "parallel run")
	if _, err := s.store.SaveContextPacket(packet); err != nil {
		return err
	}
	triggerEventID := s.latestUserTriggerEventID(item.SessionID)
	run, err := s.store.StartChatRun(sqlitecli.ChatRunInput{
		ProjectID:       session.ProjectID,
		SessionID:       item.SessionID,
		BranchID:        item.BranchID,
		Role:            core.ChatRunRoleParallel,
		Status:          core.ChatRunRunning,
		InputEventID:    triggerEventID,
		ContextPacketID: packet.ID,
	})
	if err != nil {
		return err
	}
	candidate, err := s.store.AddCandidateOutput(sqlitecli.CandidateOutputInput{
		ChatRunID:      run.ID,
		SessionID:      item.SessionID,
		BranchID:       item.BranchID,
		TriggerEventID: triggerEventID,
		Content:        "",
		Status:         core.CandidateOutputPending,
	})
	if err != nil {
		return err
	}
	thinkingEffort := strings.TrimSpace(item.ThinkingEffort)
	go func() {
		seg := s.startProviderSegment(run.ID, selection, "")
		text, _, runErr := s.runAssistant(context.Background(), item.SessionID, item.Content, thinkingEffort, selection, func(provider.Event) {})
		segStatus := core.ProviderSegmentCompleted
		candidateStatus := core.CandidateOutputReady
		if runErr != nil {
			segStatus = core.ProviderSegmentFailed
			candidateStatus = core.CandidateOutputFailed
			text = runErr.Error()
		}
		s.completeProviderSegment(seg.ID, segStatus, "")
		s.store.UpdateCandidateOutput(candidate.ID, text, candidateStatus)
		runStatus := core.ChatRunCompleted
		if runErr != nil {
			runStatus = core.ChatRunFailed
		}
		s.store.CompleteChatRun(run.ID, runStatus, "")
	}()
	return nil
}

func (s *Server) runRequestFromQueueItem(item core.QueueItem) (RunRequest, error) {
	selection, err := s.resolveChatSelection(item.SessionID, item.RouteID, item.ModelID)
	if err != nil {
		return RunRequest{}, err
	}
	return RunRequest{
		SessionID:      item.SessionID,
		Content:        item.Content,
		ThinkingEffort: item.ThinkingEffort,
		Selection:      selection,
	}, nil
}

func (s Server) recordTokenUsage(sessionID string, selection chatSelection, prompt string, completion string, status string) {
	_ = s.store.AddTokenUsage(sqlitecli.TokenUsageInput{
		ProviderPluginID:  selection.Route.ProviderPluginID,
		ProviderProfileID: selection.Profile.ID,
		SessionID:         sessionID,
		Model:             selection.Model.ModelRef,
		PromptTokens:      estimateTokens(prompt),
		CompletionTokens:  estimateTokens(completion),
		Status:            status,
	})
}

func emitTextChunks(ctx context.Context, text string, onEvent func(string, any)) {
	for _, chunk := range textChunks(text) {
		select {
		case <-ctx.Done():
			return
		default:
		}
		onEvent("assistant_delta", map[string]string{"text": chunk})
		time.Sleep(18 * time.Millisecond)
	}
}

func streamTextChunks(ctx context.Context, w http.ResponseWriter, flusher http.Flusher, text string) {
	for _, chunk := range textChunks(text) {
		select {
		case <-ctx.Done():
			return
		default:
		}
		writeStreamEvent(w, "assistant_delta", map[string]string{"text": chunk})
		flusher.Flush()
		time.Sleep(18 * time.Millisecond)
	}
}

func streamEventKindToActivityKind(kind provider.EventKind) string {
	switch kind {
	case provider.EventKindToolStart:
		return "tool"
	case provider.EventKindToolResult:
		return "result"
	case provider.EventKindApprovalRequest:
		return "approval"
	case provider.EventKindToolError, provider.EventKindTurnAborted:
		return "error"
	default:
		return string(kind)
	}
}

func (s Server) nextAssistantActivityIndex(sessionID string) int {
	_, messages, err := s.store.GetSession(sessionID)
	if err != nil {
		return 1
	}
	count := 0
	for _, message := range messages {
		if message.Role == "assistant" {
			count++
		}
	}
	return count + 1
}

func (s Server) recordMessageEvent(sessionID string, activityIndex int, kind string, payload any) sqlitecli.MessageEvent {
	if strings.TrimSpace(sessionID) == "" || activityIndex <= 0 || strings.TrimSpace(kind) == "" {
		return sqlitecli.MessageEvent{}
	}
	data, err := json.Marshal(payload)
	if err != nil {
		data = []byte("{}")
	}
	event, _ := s.store.AddMessageEvent(sqlitecli.MessageEventInput{
		SessionID:     sessionID,
		ActivityIndex: activityIndex,
		Kind:          kind,
		PayloadJSON:   string(data),
	})
	return event
}

func (s Server) recordProviderToolEvent(sessionID string, event provider.Event, messageEventID string) {
	eventType, ok := contextEventTypeForProviderEvent(event.Kind)
	if !ok {
		return
	}
	payloadRef := "provider_event:" + string(event.Kind)
	if strings.TrimSpace(messageEventID) != "" {
		payloadRef = "message_event:" + messageEventID
	}
	s.recordContextEvent(sessionID, eventType, payloadRef)
}

func (s Server) startMainChatRun(sessionID string, packet core.ContextPacket) core.ChatRun {
	inputEventID := packet.HeadEventID
	if strings.TrimSpace(inputEventID) == "" {
		inputEventID = s.currentHeadEventID(sessionID)
	}
	branchID := strings.TrimSpace(packet.BranchID)
	if branchID == "" {
		branchID = "main"
	}
	run, err := s.store.StartChatRun(sqlitecli.ChatRunInput{
		ProjectID:       packet.ProjectID,
		SessionID:       sessionID,
		BranchID:        branchID,
		Role:            core.ChatRunRoleMain,
		Status:          core.ChatRunRunning,
		InputEventID:    inputEventID,
		ContextPacketID: packet.ID,
	})
	if err != nil {
		return core.ChatRun{}
	}
	return run
}

func (s Server) completeChatRun(chatRunID string, status core.ChatRunStatus, outputEventID string) {
	if strings.TrimSpace(chatRunID) == "" {
		return
	}
	_, _ = s.store.CompleteChatRun(chatRunID, status, outputEventID)
}

func (s Server) startProviderSegment(chatRunID string, selection chatSelection, handoffReason string) core.ProviderSegment {
	if strings.TrimSpace(chatRunID) == "" {
		return core.ProviderSegment{}
	}
	segment, err := s.store.StartProviderSegment(sqlitecli.ProviderSegmentInput{
		ChatRunID:     chatRunID,
		ProviderID:    selection.Route.ProviderPluginID,
		RouteID:       selection.Route.ID,
		ModelID:       selection.Model.ID,
		Status:        core.ProviderSegmentRunning,
		HandoffReason: handoffReason,
	})
	if err != nil {
		return core.ProviderSegment{}
	}
	return segment
}

func (s Server) completeProviderSegment(segmentID string, status core.ProviderSegmentStatus, externalThreadID string) {
	if strings.TrimSpace(segmentID) == "" {
		return
	}
	_, _ = s.store.CompleteProviderSegment(segmentID, status, externalThreadID)
}

func (s Server) currentHeadEventID(sessionID string) string {
	session, _, err := s.store.GetSession(sessionID)
	if err != nil {
		return ""
	}
	head, err := s.store.GetHead(session.ProjectID, session.ID, "main")
	if err != nil {
		return ""
	}
	return head.EventID
}

func (s Server) latestUserTriggerEventID(sessionID string) string {
	session, _, err := s.store.GetSession(sessionID)
	if err != nil {
		return ""
	}
	head, err := s.store.GetHead(session.ProjectID, session.ID, "main")
	if err != nil || strings.TrimSpace(head.EventID) == "" {
		return ""
	}
	events, err := s.store.ListAncestors(head.EventID, 200)
	if err != nil {
		return ""
	}
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Type == core.EventMessageUser {
			return events[i].ID
		}
	}
	return ""
}

func contextEventTypeForProviderEvent(kind provider.EventKind) (core.EventType, bool) {
	switch kind {
	case provider.EventKindToolStart, provider.EventKindApprovalRequest:
		return core.EventToolRequested, true
	case provider.EventKindToolResult:
		return core.EventToolCompleted, true
	case provider.EventKindToolError:
		return core.EventToolFailed, true
	case provider.EventKindTurnAborted:
		return core.EventTurnAborted, true
	default:
		return "", false
	}
}

func providerRunPayloadRef(selection chatSelection, status string) string {
	parts := []string{
		"provider_run",
		status,
		selection.Route.ProviderPluginID,
		selection.Route.ID,
		selection.Model.ID,
	}
	return strings.Join(compactStrings(parts), ":")
}

func (s Server) recordContextEvent(sessionID string, eventType core.EventType, payloadRef string) core.Event {
	return s.recordContextEventAfter(sessionID, eventType, payloadRef, "")
}

func (s Server) recordContextEventAfter(sessionID string, eventType core.EventType, payloadRef string, preferredParentID string) core.Event {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || strings.TrimSpace(string(eventType)) == "" {
		return core.Event{}
	}
	session, _, err := s.store.GetSession(sessionID)
	if err != nil {
		return core.Event{}
	}
	parentIDs := []string{}
	preferredParentID = strings.TrimSpace(preferredParentID)
	if preferredParentID != "" {
		parentIDs = append(parentIDs, preferredParentID)
	} else if head, err := s.store.GetHead(session.ProjectID, session.ID, "main"); err == nil && strings.TrimSpace(head.EventID) != "" {
		parentIDs = append(parentIDs, head.EventID)
	}
	event, err := s.store.AppendEvent(core.EventInput{
		Type:           eventType,
		ProjectID:      session.ProjectID,
		SessionID:      session.ID,
		BranchID:       "main",
		ParentEventIDs: parentIDs,
		PayloadRef:     strings.TrimSpace(payloadRef),
	})
	if err != nil {
		return core.Event{}
	}
	if _, err := s.store.MoveHead(session.ProjectID, session.ID, "main", event.ID); err != nil {
		return core.Event{}
	}
	return event
}

func toolEventPayload(event provider.Event) map[string]any {
	if event.Tool == nil {
		return map[string]any{"text": event.Text}
	}
	return map[string]any{
		"type":         event.Tool.Type,
		"toolId":       event.Tool.ToolID,
		"toolName":     event.Tool.ToolName,
		"invocationId": event.Tool.InvocationID,
		"approvalId":   event.Tool.ApprovalID,
		"command":      event.Tool.Command,
		"text":         event.Tool.Text,
		"status":       event.Tool.Status,
		"raw":          event.Tool.Payload,
	}
}

func toolEventPayloadForSession(sessionID string, event provider.Event) map[string]any {
	payload := toolEventPayload(event)
	payload["sessionId"] = sessionID
	return payload
}

func textChunks(text string) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}
	chunks := make([]string, 0, len(words))
	for i, word := range words {
		if i < len(words)-1 {
			word += " "
		}
		chunks = append(chunks, word)
	}
	return chunks
}

func (s Server) runAssistant(ctx context.Context, sessionID string, content string, thinkingEffort string, selection chatSelection, onEvent func(provider.Event)) (string, bool, error) {
	driver, ok := s.drivers.Get(selection.Route.ProviderPluginID)
	if !ok {
		return "", false, fmt.Errorf("%s has no Ergo Loom chat driver", selection.Route.DisplayName)
	}
	request, err := s.providerChatRequest(sessionID, content, thinkingEffort, selection)
	if err != nil {
		return "", false, err
	}
	if !driver.CanExecute(request) {
		return "", false, fmt.Errorf("%s is not executable from Ergo Loom chat yet", selection.Route.DisplayName)
	}
	response, err := driver.Respond(ctx, request, onEvent)
	if response.ExternalThreadID != "" {
		bindingInput := providerChatBindingInput(sessionID, selection)
		bindingInput.ExternalThreadID = response.ExternalThreadID
		if _, bindErr := s.store.UpsertProviderChatBinding(bindingInput); bindErr != nil && err == nil {
			err = bindErr
		}
	}
	return response.Text, response.Streamed, err
}

func (s *Server) moderatedSelectionForActiveChat(sessionID string, selection chatSelection) (chatSelection, *moderatorHandoff, error) {
	expiredGroup := providerGroupID(selection.Route.ProviderPluginID)
	expired, used, err := s.providerGroupSoftExpired(expiredGroup)
	if err != nil {
		return selection, nil, err
	}
	if !expired {
		return selection, nil, nil
	}

	s.addSessionExcludedRoute(sessionID, selection.Route.ID)
	next, err := s.moderatorFallbackSelection(sessionID, expiredGroup)
	if err != nil {
		return selection, nil, fmt.Errorf("%s token quota is exhausted and no moderator fallback provider is available: %w", providerGroupLabel(expiredGroup), err)
	}
	reason := fmt.Sprintf(
		"Moderator detected %s token quota exhaustion (%d/%d tracked tokens) and handed this chat context to %s.",
		providerGroupLabel(expiredGroup),
		used,
		providerSoftTokenCap,
		providerGroupLabel(providerGroupID(next.Route.ProviderPluginID)),
	)
	return next, &moderatorHandoff{
		FromSelection: selection,
		ToSelection:   next,
		Reason:        reason,
	}, nil
}

func (s Server) providerGroupSoftExpired(groupID string) (bool, int, error) {
	usage, err := s.store.TokenUsageSummary()
	if err != nil {
		return false, 0, err
	}
	total := 0
	for _, item := range usage {
		if providerGroupID(item.ProviderPluginID) != groupID {
			continue
		}
		total += item.PromptTokens + item.CompletionTokens
	}
	return total >= providerSoftTokenCap, total, nil
}

func (s Server) moderatorFallbackSelection(sessionID string, expiredGroup string) (chatSelection, error) {
	session, _, err := s.store.GetSession(sessionID)
	if err != nil {
		return chatSelection{}, err
	}
	projectID := strings.TrimSpace(session.ProjectID)
	if projectID == "" {
		projectID = "default"
	}
	projectRoutes, err := s.store.ListProjectAccessRoutes(projectID)
	if err != nil {
		return chatSelection{}, err
	}
	models, err := s.store.ListProviderModels()
	if err != nil {
		return chatSelection{}, err
	}
	pref, err := s.store.ModeratorPreference(projectID)
	if err != nil {
		return chatSelection{}, err
	}
	sessionGroups, err := s.store.ListSessionProviderGroups(sessionID)
	if err != nil {
		return chatSelection{}, err
	}
	allowedGroups := map[string]bool{}
	for _, groupID := range sessionGroups {
		allowedGroups[providerGroupID(groupID)] = true
	}

	for _, groupID := range moderatorGroupOrder(pref, projectRoutes) {
		if groupID == "" || groupID == expiredGroup {
			continue
		}
		if len(allowedGroups) > 0 && !allowedGroups[groupID] {
			continue
		}
		if expired, _, err := s.providerGroupSoftExpired(groupID); err != nil {
			return chatSelection{}, err
		} else if expired {
			continue
		}
		if selection, ok := s.selectionForProviderGroup(sessionID, groupID, projectRoutes, models); ok {
			return selection, nil
		}
	}
	return chatSelection{}, errors.New("no executable fallback provider in the current chat")
}

func (s Server) selectionForProviderGroup(sessionID string, groupID string, routes []sqlitecli.ProjectAccessRoute, models []sqlitecli.ProviderModel) (chatSelection, bool) {
	for _, route := range routes {
		if !route.Enabled || route.Route.Status != "available" || providerGroupID(route.Route.ProviderPluginID) != groupID {
			continue
		}
		for _, model := range preferredModelsForProvider(models, route.Route.ProviderPluginID) {
			selection, err := s.resolveChatSelection(sessionID, route.Route.ID, model.ID)
			if err == nil {
				return selection, true
			}
		}
	}
	return chatSelection{}, false
}

func preferredModelsForProvider(models []sqlitecli.ProviderModel, providerID string) []sqlitecli.ProviderModel {
	var defaults []sqlitecli.ProviderModel
	var rest []sqlitecli.ProviderModel
	for _, model := range models {
		if model.ProviderPluginID != providerID || model.Status != "available" {
			continue
		}
		if model.IsDefault {
			defaults = append(defaults, model)
			continue
		}
		rest = append(rest, model)
	}
	return append(defaults, rest...)
}

func moderatorGroupOrder(pref sqlitecli.ModeratorPreference, routes []sqlitecli.ProjectAccessRoute) []string {
	ordered := make([]string, 0, len(routes)+2)
	if pref.Mode == "manual" {
		ordered = append(ordered, providerGroupID(pref.PrimaryProviderGroupID), providerGroupID(pref.SecondaryProviderGroupID))
	}
	for _, route := range routes {
		if !route.Enabled {
			continue
		}
		ordered = append(ordered, providerGroupID(route.Route.ProviderPluginID))
	}
	return compactStrings(ordered)
}

// maybeGenerateHandoffSummary checks if the incoming selection represents a
// provider switch and, if so, generates and saves a handoff summary event.
func (s Server) maybeGenerateHandoffSummary(sessionID string, selection chatSelection) {
	lastSegment, err := s.store.LastCompletedSegment(sessionID)
	if err != nil {
		return // no prior segment — first message in session
	}

	session, _, err := s.store.GetSession(sessionID)
	if err != nil {
		return
	}

	policyName := "route-change"
	if proj, err := s.store.GetProject(session.ProjectID); err == nil && proj.HandoffPolicy != "" {
		policyName = proj.HandoffPolicy
	}
	policy := s.handoffPolicies.GetOrDefault(policyName)

	if !policy.DetectSwitch(lastSegment, core.HandoffCandidate{RouteID: selection.Route.ID, ModelID: selection.Model.ID}) {
		return
	}

	msgs, err := s.store.MessagesSince(sessionID, lastSegment.StartedAt)
	if err != nil || len(msgs) == 0 {
		return
	}

	// Build a single-shot CallProvider using the previous segment's route/driver.
	// This lets AI policies call the outgoing provider to generate the summary.
	var callProvider func(prompt string) (string, error)
	if prevSelection, err := s.resolveChatSelection(sessionID, lastSegment.RouteID, lastSegment.ModelID); err == nil {
		if driver, ok := s.drivers.Get(prevSelection.Route.ProviderPluginID); ok {
			callProvider = func(prompt string) (string, error) {
				resp, err := driver.Respond(context.Background(), provider.ChatRequest{
					SessionID:        sessionID,
					RouteID:          prevSelection.Route.ID,
					RouteDisplayName: prevSelection.Route.DisplayName,
					RouteTransport:   prevSelection.Route.Transport,
					ProviderPluginID: prevSelection.Route.ProviderPluginID,
					ModelID:          prevSelection.Model.ID,
					ModelDisplayName: prevSelection.Model.DisplayName,
					ModelRef:         prevSelection.Model.ModelRef,
					Input:            prompt,
				}, func(provider.Event) {})
				if err != nil {
					return "", err
				}
				return resp.Text, nil
			}
		}
	}

	payload, err := policy.Summarize(core.HandoffContext{
		Session:      session,
		Segment:      lastSegment,
		Messages:     msgs,
		CallProvider: callProvider,
	})
	if err != nil || strings.TrimSpace(payload.Text) == "" {
		return
	}

	summaryID, err := s.store.SaveSummary(payload)
	if err != nil {
		return
	}

	_, _ = s.store.AppendEvent(core.EventInput{
		Type:       core.EventSummaryCreated,
		SessionID:  sessionID,
		BranchID:   "main",
		PayloadRef: "summary:" + summaryID,
	})
}

func (s Server) buildContextPacket(sessionID string, latestUserInput string, selection chatSelection, note string) core.ContextPacket {
	session, messages, err := s.store.GetSession(sessionID)
	if err != nil {
		return core.ContextPacket{
			ID:        fmt.Sprintf("context_packet_%d", time.Now().UTC().UnixNano()),
			SessionID: sessionID,
			UserInput: latestUserInput,
			Content:   latestUserInput,
			CreatedAt: time.Now().UTC(),
		}
	}

	// Resolve ancestors via event graph; fall back to empty slice if unavailable.
	var ancestors []core.Event
	var headEventID string
	if head, err := s.store.GetHead(session.ProjectID, session.ID, "main"); err == nil {
		headEventID = head.EventID
		if list, err := s.store.ListAncestors(head.EventID, 200); err == nil {
			ancestors = list
		}
	}

	// Determine context budget from route. 0 means policy default.
	contextBudget := selection.Route.ContextBudgetChars

	// Look up policy from project; fall back to flat-trim.
	policyName := "flat-trim"
	if proj, err := s.store.GetProject(session.ProjectID); err == nil && proj.ContextPolicy != "" {
		policyName = proj.ContextPolicy
	}
	policy := s.packetPolicies.GetOrDefault(policyName)

	pbc := core.PacketBuildContext{
		Session:       session,
		Messages:      messages,
		Ancestors:     ancestors,
		HeadEventID:   headEventID,
		UserInput:     latestUserInput,
		Note:          note,
		ContextBudget: contextBudget,
		RouteLabel:    selection.Route.DisplayName + " / " + selection.Model.DisplayName,
		LoadSummary:   s.store.GetSummary,
		RetrieveKnowledge: func(text string) ([]core.KnowledgeItem, error) {
			if s.knowledge == nil {
				return nil, nil
			}
			return s.knowledge.Search(context.Background(), core.KnowledgeQuery{
				SessionID: session.ID,
				ProjectID: session.ProjectID,
				Text:      text,
				Limit:     contextKnowledgeLimit,
			})
		},
	}
	packet := policy.Build(pbc)

	// Attach references for traceability.
	if headEventID != "" {
		packet.References = append(packet.References, core.ContextReference{Kind: "head", ID: headEventID, Ref: "context_heads/main"})
	}
	for _, event := range ancestors {
		packet.References = append(packet.References, core.ContextReference{Kind: string(event.Type), ID: event.ID, Ref: event.PayloadRef})
	}

	return packet
}

func (s Server) moderatorHandoffPrompt(sessionID string, latestUserInput string, handoff moderatorHandoff) string {
	_, messages, err := s.store.GetSession(sessionID)
	if err != nil {
		return latestUserInput
	}
	const maxContextChars = 12000
	lines := []string{
		"You are Ergo Loom. Continue this chat after a moderator provider handoff.",
		fmt.Sprintf("Previous provider route: %s / %s.", handoff.FromSelection.Route.DisplayName, handoff.FromSelection.Model.DisplayName),
		fmt.Sprintf("New provider route: %s / %s.", handoff.ToSelection.Route.DisplayName, handoff.ToSelection.Model.DisplayName),
		"Use the conversation context below as the transferred provider context. Answer the latest user message directly.",
		"",
		"Conversation context:",
	}
	for _, message := range messages {
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s: %s", message.Role, content))
	}
	prompt := strings.Join(lines, "\n")
	if len([]rune(prompt)) <= maxContextChars {
		return prompt
	}
	runes := []rune(prompt)
	return strings.Join([]string{
		"You are Ergo Loom. Continue this chat after a moderator provider handoff.",
		"The transferred context was truncated to fit the fallback provider request.",
		"",
		string(runes[len(runes)-maxContextChars:]),
	}, "\n")
}

func providerGroupID(providerID string) string {
	switch strings.TrimSpace(providerID) {
	case "codex", "openai", "codex/openai", "codex-openai":
		return "codex-openai"
	default:
		return strings.TrimSpace(providerID)
	}
}

func providerGroupLabel(groupID string) string {
	switch providerGroupID(groupID) {
	case "codex-openai":
		return "Codex/ChatGPT"
	case "anthropic":
		return "Claude"
	case "copilot":
		return "VSCode Copilot"
	case "gemini":
		return "Gemini"
	case "ollama":
		return "Ollama(Local Model)"
	default:
		return groupID
	}
}

func (s Server) resolveChatSelection(sessionID string, routeID string, modelID string) (chatSelection, error) {
	return s.resolveChatSelectionWithExclusions(sessionID, routeID, modelID, nil)
}

func (s *Server) resolveChatSelectionExcluding(sessionID string, excludeRouteID string) (chatSelection, error) {
	excluded := map[string]bool{}
	if strings.TrimSpace(excludeRouteID) != "" {
		excluded[strings.TrimSpace(excludeRouteID)] = true
	}
	return s.resolveChatSelectionWithExclusions(sessionID, "", "", excluded)
}

func (s *Server) resolveChatSelectionWithExclusions(sessionID string, routeID string, modelID string, excludedRouteIDs map[string]bool) (chatSelection, error) {
	routeID = strings.TrimSpace(routeID)
	modelID = strings.TrimSpace(modelID)
	for k, v := range s.sessionExcludedRoutes(sessionID) {
		excludedRouteIDs[k] = v
	}

	project, err := s.store.DefaultProject()
	if err != nil {
		return chatSelection{}, err
	}
	var session core.Session
	if sessionID != sessionIDForCapabilityCheck {
		if sess, _, sessionErr := s.store.GetSession(sessionID); sessionErr == nil {
			session = sess
			if strings.TrimSpace(sess.ProjectID) != "" {
				project.ID = sess.ProjectID
			}
		}
	}

	projectRoutes, err := s.store.ListProjectAccessRoutes(project.ID)
	if err != nil {
		return chatSelection{}, err
	}

	models, err := s.store.ListProviderModels()
	if err != nil {
		return chatSelection{}, err
	}

	// Build candidates sorted by priority for route selection policy.
	candidates := make([]core.RouteCandidate, 0, len(projectRoutes))
	for _, r := range projectRoutes {
		if !r.Enabled || r.Route.Status != "available" || excludedRouteIDs[r.Route.ID] {
			continue
		}
		for _, model := range preferredModelsForProvider(models, r.Route.ProviderPluginID) {
			candidates = append(candidates, core.RouteCandidate{
				RouteID:  r.Route.ID,
				ModelID:  model.ID,
				Priority: r.Priority,
			})
			break
		}
	}

	// Route selection policy resolves the final routeID/modelID.
	policyName := "manual"
	if proj, err := s.store.GetProject(session.ProjectID); err == nil && proj.RoutePolicy != "" {
		policyName = proj.RoutePolicy
	}
	selectedRouteID, selectedModelID, err := s.routePolicies.GetOrDefault(policyName).Select(core.RouteSelectionContext{
		Session:          session,
		Candidates:       candidates,
		RequestedRouteID: routeID,
		RequestedModelID: modelID,
	})
	if err != nil {
		return chatSelection{}, err
	}
	if selectedRouteID == "" {
		return chatSelection{}, errors.New("chat route is required")
	}
	routeID = selectedRouteID
	if selectedModelID != "" {
		modelID = selectedModelID
	}
	if modelID == "" {
		return chatSelection{}, errors.New("chat model is required")
	}

	var selectedRoute sqlitecli.AccessRoute
	for _, route := range projectRoutes {
		if route.Enabled && route.Route.ID == routeID {
			selectedRoute = route.Route
			break
		}
	}
	if selectedRoute.ID == "" {
		return chatSelection{}, errors.New("selected route is not enabled for this project")
	}
	if selectedRoute.Status != "available" {
		return chatSelection{}, fmt.Errorf("%s is not available yet", selectedRoute.DisplayName)
	}

	var selectedModel sqlitecli.ProviderModel
	for _, model := range models {
		if model.ID == modelID {
			selectedModel = model
			break
		}
	}
	if selectedModel.ID == "" {
		return chatSelection{}, errors.New("selected model was not found")
	}
	if selectedModel.ProviderPluginID != selectedRoute.ProviderPluginID {
		return chatSelection{}, errors.New("selected model does not belong to the selected route provider")
	}
	if selectedModel.Status != "available" {
		return chatSelection{}, fmt.Errorf("%s is %s and cannot be executed directly yet", selectedModel.DisplayName, selectedModel.Status)
	}

	profiles, err := s.store.ListProviderProfiles()
	if err != nil {
		return chatSelection{}, err
	}
	var selectedProfile sqlitecli.ProviderProfile
	for _, profile := range profiles {
		if profile.ProviderPluginID == selectedRoute.ProviderPluginID && profile.IsDefault {
			selectedProfile = profile
			break
		}
	}
	if selectedProfile.ID == "" {
		for _, profile := range profiles {
			if profile.ProviderPluginID == selectedRoute.ProviderPluginID {
				selectedProfile = profile
				break
			}
		}
	}

	selection := chatSelection{
		Route:   selectedRoute,
		Model:   selectedModel,
		Profile: selectedProfile,
	}
	if !s.drivers.CanExecute(chatRequestFromSelection(sessionIDForCapabilityCheck, "", "", selection, "")) {
		return chatSelection{}, fmt.Errorf("%s is not executable from Ergo Loom chat yet", selection.Route.DisplayName)
	}
	return selection, nil
}

const sessionIDForCapabilityCheck = "__capability_check__"

func (s Server) providerChatRequest(sessionID string, content string, thinkingEffort string, selection chatSelection) (provider.ChatRequest, error) {
	threadID := ""
	bindingInput := providerChatBindingInput(sessionID, selection)
	if binding, err := s.store.GetProviderChatBinding(bindingInput); err == nil {
		threadID = binding.ExternalThreadID
	} else if !errors.Is(err, sqlitecli.ErrNotFound) {
		return provider.ChatRequest{}, err
	}
	request := chatRequestFromSelection(sessionID, content, thinkingEffort, selection, threadID)
	request.ApprovalHandler = s.approvals.request
	return request, nil
}

func providerChatBindingInput(sessionID string, selection chatSelection) sqlitecli.ProviderChatBindingInput {
	return sqlitecli.ProviderChatBindingInput{
		SessionID:         sessionID,
		ProviderPluginID:  selection.Route.ProviderPluginID,
		ProviderProfileID: selection.Profile.ID,
		AccessRouteID:     selection.Route.ID,
		ModelID:           selection.Model.ID,
	}
}

func chatRequestFromSelection(sessionID string, content string, thinkingEffort string, selection chatSelection, externalThreadID string) provider.ChatRequest {
	return provider.ChatRequest{
		SessionID:         sessionID,
		RouteID:           selection.Route.ID,
		RouteDisplayName:  selection.Route.DisplayName,
		RouteTransport:    selection.Route.Transport,
		ProviderPluginID:  selection.Route.ProviderPluginID,
		ProviderProfileID: selection.Profile.ID,
		ModelID:           selection.Model.ID,
		ModelDisplayName:  selection.Model.DisplayName,
		ModelRef:          selection.Model.ModelRef,
		Input:             content,
		ThinkingEffort:    thinkingEffort,
		ExternalThreadID:  externalThreadID,
	}
}

func contextUsageOrZero(store sqlitecli.Store, sessionID string) sqlitecli.SessionContextUsage {
	usage, err := store.SessionContextUsage(sessionID)
	if err != nil {
		return sqlitecli.SessionContextUsage{}
	}
	return usage
}

func localUnavailableMessage(selection chatSelection) string {
	return fmt.Sprintf("%s / %s is selected, but direct execution for this route is not available in the non-streaming fallback.", selection.Route.DisplayName, selection.Model.DisplayName)
}

func handoffPendingMessage(selection chatSelection) string {
	return fmt.Sprintf("%s / %s is selected. This account route is connected for Ergo Loom routing, but the external chat handoff worker is not implemented yet.", selection.Route.DisplayName, selection.Model.DisplayName)
}

func bridgePendingMessage(selection chatSelection) string {
	return fmt.Sprintf("%s / %s is selected. This provider needs the Copilot SDK or VS Code bridge worker before Ergo Loom can execute the chat directly.", selection.Route.DisplayName, selection.Model.DisplayName)
}

func detectProviderAccount(providerID string, displayName string) (string, string, error) {
	providerID = strings.TrimSpace(providerID)
	displayName = strings.TrimSpace(displayName)
	if providerID == "" {
		return "", "", errors.New("provider id is required")
	}
	if displayName != "" {
		return displayName, "manual account label", nil
	}

	switch providerID {
	case "codex", "openai":
		status := detectCodexAuth()
		if !status.Connected {
			return "", "", errors.New(status.Detail)
		}
		return status.AccountLabel, status.Detail, nil
	case "copilot":
		status := detectGitHubAuth("copilot", "VSCode Copilot")
		if !status.Connected {
			return "", "", errors.New(status.Detail)
		}
		return status.AccountLabel, status.Detail, nil
	case "anthropic":
		status := detectClaudeAuth()
		if !status.Connected {
			return "", "", errors.New(status.Detail)
		}
		return status.AccountLabel, status.Detail, nil
	case "gemini":
		if _, err := exec.LookPath("gemini"); err != nil {
			return "", "", errors.New("Gemini CLI was not found; install gemini before connecting")
		}
		return "Gemini local account", "gemini CLI detected", nil
	case "ollama":
		status := detectOllamaAuth()
		if !status.Connected {
			return "", "", errors.New(status.Detail)
		}
		return status.AccountLabel, status.Detail, nil
	default:
		return providerID + " account", "generic provider profile", nil
	}
}

func detectAuthStatuses() []authStatus {
	return []authStatus{
		detectCodexAuth(),
		detectGitHubAuth("github", "GitHub"),
		detectGitHubAuth("copilot", "VSCode Copilot"),
		detectClaudeAuth(),
		detectExecutableAuth("gemini", "Gemini", "gemini", []string{"--version"}, ""),
		detectOllamaAuth(),
	}
}

func detectRuntimeDiagnostics() runtimeDiagnostics {
	return runtimeDiagnostics{
		Desktop:       strings.TrimSpace(os.Getenv("ERGO_LOOM_DESKTOP")) == "1",
		AppRoot:       strings.TrimSpace(os.Getenv("ERGO_LOOM_APP_ROOT")),
		DataDir:       strings.TrimSpace(os.Getenv("ERGO_LOOM_DATA_DIR")),
		HandoffBridge: strings.TrimSpace(os.Getenv("ERGO_LOOM_HANDOFF_BRIDGE_URL")),
		Path:          os.Getenv("PATH"),
		Executables: []runtimeExecutable{
			executableDiagnostic("codex", "Codex/ChatGPT", codexExecutablePath),
			executableDiagnostic("claude", "Claude CLI", func() (string, error) {
				return executablePath("claude", "ERGO_CLAUDE_COMMAND", filepath.Join(os.Getenv("HOME"), ".local", "bin", "claude"))
			}),
			executableDiagnostic("gh", "GitHub CLI", func() (string, error) {
				return executablePath("gh", "")
			}),
			runtimeExecutable{
				ID:     "copilot-bridge",
				Label:  "Copilot Bridge",
				Path:   strings.TrimSpace(os.Getenv("ERGO_COPILOT_BRIDGE_URL")),
				Status: bridgeDiagnosticStatus(os.Getenv("ERGO_COPILOT_BRIDGE_URL")),
				Detail: bridgeDiagnosticDetail(os.Getenv("ERGO_COPILOT_BRIDGE_URL")),
			},
			executableDiagnostic("gemini", "Gemini CLI", func() (string, error) {
				return executablePath("gemini", "")
			}),
			executableDiagnostic("ollama", "Ollama", func() (string, error) {
				return executablePath("ollama", "ERGO_OLLAMA_COMMAND")
			}),
		},
	}
}

func bridgeDiagnosticStatus(value string) string {
	if strings.TrimSpace(value) == "" {
		return "missing"
	}
	return "ready"
}

func bridgeDiagnosticDetail(value string) string {
	if detail := strings.TrimSpace(value); detail != "" {
		return detail
	}
	return "Set ERGO_COPILOT_BRIDGE_URL after starting a Copilot bridge worker"
}

func executableDiagnostic(id string, label string, resolve func() (string, error)) runtimeExecutable {
	path, err := resolve()
	if err != nil {
		return runtimeExecutable{ID: id, Label: label, Status: "missing", Detail: err.Error()}
	}
	return runtimeExecutable{ID: id, Label: label, Path: path, Status: "ready", Detail: path}
}

func detectClaudeAuth() authStatus {
	path, err := executablePath("claude", "ERGO_CLAUDE_COMMAND", filepath.Join(os.Getenv("HOME"), ".local", "bin", "claude"))
	if err != nil {
		return authStatus{ID: "claude", Label: "Claude", AccountLabel: "", Connected: false, Status: "missing", Detail: "Claude CLI was not found"}
	}
	out, runErr := exec.Command(path, "auth", "status").CombinedOutput()
	raw := string(out)
	detail := compactDetail(raw)
	if runErr != nil {
		if strings.Contains(raw, `"loggedIn": false`) || strings.Contains(strings.ToLower(raw), "loggedin") {
			return authStatus{ID: "claude", Label: "Claude", AccountLabel: "", Connected: false, Status: "signed_out", Detail: "Claude CLI is installed but not logged in with a subscription token; run claude setup-token with your Pro/Max subscription account"}
		}
		if detail == "" {
			detail = runErr.Error()
		}
		if strings.Contains(strings.ToLower(detail), "credit balance too low") {
			detail = "Claude CLI is using API billing and reports low credit balance; run claude setup-token with your Pro/Max subscription account so Ergo Loom can use the subscription route"
		}
		return authStatus{ID: "claude", Label: "Claude", AccountLabel: "", Connected: false, Status: "error", Detail: detail}
	}
	account := "Claude account"
	if strings.Contains(detail, `"loggedIn": true`) {
		account = "Claude local account"
	}
	return authStatus{ID: "claude", Label: "Claude", AccountLabel: account, Connected: true, Status: "ready", Detail: detail}
}

func detectOllamaAuth() authStatus {
	path, err := executablePath("ollama", "ERGO_OLLAMA_COMMAND")
	if err != nil {
		return authStatus{ID: "ollama", Label: "Ollama(Local Model)", AccountLabel: "", Connected: false, Status: "missing", Detail: "ollama not found"}
	}
	versionOut, _ := exec.Command(path, "--version").CombinedOutput()
	listOut, listErr := exec.Command(path, "list").CombinedOutput()
	detail := compactDetail(string(listOut))
	if listErr != nil {
		if detail == "" {
			detail = compactDetail(string(versionOut))
		}
		if detail == "" {
			detail = listErr.Error()
		}
		return authStatus{ID: "ollama", Label: "Ollama(Local Model)", AccountLabel: "", Connected: false, Status: "not_running", Detail: detail}
	}
	lines := strings.Split(strings.TrimSpace(string(listOut)), "\n")
	modelCount := 0
	if len(lines) > 1 {
		modelCount = len(lines) - 1
	}
	account := "Ollama local runtime"
	if modelCount == 0 {
		return authStatus{ID: "ollama", Label: "Ollama(Local Model)", AccountLabel: account, Connected: true, Status: "ready_empty", Detail: "Ollama is running; no local models installed"}
	}
	return authStatus{ID: "ollama", Label: "Ollama(Local Model)", AccountLabel: account, Connected: true, Status: "ready", Detail: fmt.Sprintf("Ollama is running; %d local model(s) available", modelCount)}
}

func executablePathOrName(name string, envVar string, candidates ...string) string {
	path, err := executablePath(name, envVar, candidates...)
	if err != nil {
		return name
	}
	return path
}

func executablePath(name string, envVar string, candidates ...string) (string, error) {
	all := []string{strings.TrimSpace(os.Getenv(envVar)), name}
	all = append(all, candidates...)
	for _, candidate := range all {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if strings.ContainsRune(candidate, filepath.Separator) {
			if stat, err := os.Stat(candidate); err == nil && !stat.IsDir() {
				return candidate, nil
			}
			continue
		}
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		}
	}
	return "", exec.ErrNotFound
}

func detectCodexAuth() authStatus {
	path, err := codexExecutablePath()
	if err != nil {
		return authStatus{ID: "codex", Label: "Codex/ChatGPT", AccountLabel: "", Connected: false, Status: "missing", Detail: "Codex local runtime was not found"}
	}
	out, runErr := exec.Command(path, "login", "status").CombinedOutput()
	detail := compactDetail(string(out))
	if runErr != nil {
		if detail == "" {
			detail = runErr.Error()
		}
		return authStatus{ID: "codex", Label: "Codex/ChatGPT", AccountLabel: "", Connected: false, Status: "error", Detail: detail}
	}
	account := "ChatGPT account"
	if strings.Contains(strings.ToLower(detail), "api key") {
		account = "OpenAI API key"
	}
	return authStatus{ID: "codex", Label: "Codex/ChatGPT", AccountLabel: account, Connected: true, Status: "ready", Detail: detail}
}

func codexExecutablePath() (string, error) {
	candidates := []string{
		strings.TrimSpace(os.Getenv("ERGO_CODEX_COMMAND")),
		strings.TrimSpace(os.Getenv("CODEX_EXEC")),
		"codex",
		"/Applications/Codex.app/Contents/Resources/codex",
		filepath.Join(os.Getenv("HOME"), "Applications", "Codex.app", "Contents", "Resources", "codex"),
	}
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if strings.ContainsRune(candidate, filepath.Separator) {
			if stat, err := os.Stat(candidate); err == nil && !stat.IsDir() {
				return candidate, nil
			}
			continue
		}
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		}
	}
	return "", exec.ErrNotFound
}

func detectGitHubAuth(id string, label string) authStatus {
	path, err := exec.LookPath("gh")
	if err != nil {
		return authStatus{ID: id, Label: label, AccountLabel: "", Connected: false, Status: "missing", Detail: "gh not found"}
	}
	out, runErr := exec.Command(path, "auth", "status").CombinedOutput()
	detail := compactDetail(string(out))
	if runErr != nil {
		if detail == "" {
			detail = runErr.Error()
		}
		return authStatus{ID: id, Label: label, AccountLabel: "", Connected: false, Status: "error", Detail: detail}
	}
	account := githubAccountFromStatus(string(out))
	if account == "" {
		account = "GitHub account"
	}
	return authStatus{ID: id, Label: label, AccountLabel: account, Connected: true, Status: "ready", Detail: "gh authenticated"}
}

func githubAccountFromStatus(value string) string {
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		marker := " account "
		if index := strings.Index(line, marker); index >= 0 {
			rest := strings.TrimSpace(line[index+len(marker):])
			if end := strings.Index(rest, " "); end >= 0 {
				return rest[:end]
			}
			return rest
		}
	}
	return ""
}

func detectExecutableAuth(id string, label string, command string, args []string, fallback string) authStatus {
	path, err := exec.LookPath(command)
	if err != nil && fallback != "" {
		if _, statErr := os.Stat(fallback); statErr == nil {
			path = fallback
			err = nil
		}
	}
	if err != nil {
		return authStatus{
			ID:           id,
			Label:        label,
			AccountLabel: "",
			Connected:    false,
			Status:       "missing",
			Detail:       command + " not found",
		}
	}
	if len(args) == 0 {
		return authStatus{
			ID:           id,
			Label:        label,
			AccountLabel: label + " account",
			Connected:    true,
			Status:       "ready",
			Detail:       path,
		}
	}

	out, runErr := exec.Command(path, args...).CombinedOutput()
	detail := compactDetail(string(out))
	if detail == "" {
		detail = path
	}
	if id == "github" && runErr == nil {
		detail = "gh authenticated"
	}
	if runErr != nil {
		return authStatus{
			ID:           id,
			Label:        label,
			AccountLabel: "",
			Connected:    false,
			Status:       "error",
			Detail:       detail,
		}
	}
	return authStatus{
		ID:           id,
		Label:        label,
		AccountLabel: label + " account",
		Connected:    true,
		Status:       "ready",
		Detail:       detail,
	}
}

func compactDetail(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			if len([]rune(line)) > 180 {
				return string([]rune(line)[:180]) + "..."
			}
			return line
		}
	}
	return ""
}

func expandPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if value == "~" {
		if homeDir, err := os.UserHomeDir(); err == nil {
			return homeDir
		}
		return value
	}
	if strings.HasPrefix(value, "~/") {
		if homeDir, err := os.UserHomeDir(); err == nil {
			return filepath.Join(homeDir, strings.TrimPrefix(value, "~/"))
		}
	}
	return value
}

func estimateTokens(text string) int {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return 0
	}
	return max(1, (len([]rune(trimmed))+3)/4)
}

func compactStrings(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func sessionIDFromPath(path string) (string, bool) {
	value := strings.TrimPrefix(path, "/api/sessions/")
	if value == "" || strings.Contains(value, "/") {
		return "", false
	}
	return value, true
}

func messageSessionIDFromPath(path string) (string, bool) {
	value := strings.TrimPrefix(path, "/api/sessions/")
	sessionID, suffix, ok := strings.Cut(value, "/messages")
	if !ok || suffix != "" || sessionID == "" {
		return "", false
	}
	return sessionID, true
}

func streamSessionIDFromPath(path string) (string, bool) {
	value := strings.TrimPrefix(path, "/api/sessions/")
	sessionID, suffix, ok := strings.Cut(value, "/messages/stream")
	if !ok || suffix != "" || sessionID == "" {
		return "", false
	}
	return sessionID, true
}

func projectRouteProjectIDFromPath(path string) (string, bool) {
	value := strings.TrimPrefix(path, "/api/projects/")
	projectID, suffix, ok := strings.Cut(value, "/routes")
	if !ok || suffix != "" || projectID == "" {
		return "", false
	}
	return projectID, true
}

func projectModeratorProjectIDFromPath(path string) (string, bool) {
	value := strings.TrimPrefix(path, "/api/projects/")
	projectID, suffix, ok := strings.Cut(value, "/moderator")
	if !ok || suffix != "" || projectID == "" {
		return "", false
	}
	return projectID, true
}

func projectIDFromPath(path string) (string, bool) {
	value := strings.TrimPrefix(path, "/api/projects/")
	if value == "" || strings.Contains(value, "/") {
		return "", false
	}
	return value, true
}

func projectRouteIDsFromPath(path string) (string, string, bool) {
	value := strings.TrimPrefix(path, "/api/projects/")
	projectID, routeID, ok := strings.Cut(value, "/routes/")
	if !ok || projectID == "" || routeID == "" || strings.Contains(routeID, "/") {
		return "", "", false
	}
	return projectID, routeID, true
}

func writeStreamEvent(w http.ResponseWriter, eventType string, payload any) {
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":    eventType,
		"payload": payload,
	})
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func writeError(w http.ResponseWriter, err error, status int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprint(err)})
}
