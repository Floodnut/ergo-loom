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

	webapp "github.com/jkj-dev/ergo-loom/apps/desktop-or-web"
	"github.com/jkj-dev/ergo-loom/internal/chatfilter"
	"github.com/jkj-dev/ergo-loom/internal/provider"
	"github.com/jkj-dev/ergo-loom/internal/storage/sqlitecli"
	"github.com/jkj-dev/ergo-loom/internal/toolruntime"
)

type Server struct {
	store     sqlitecli.Store
	approvals *approvalBroker
	filters   chatfilter.Chain
	drivers   provider.DriverRegistry
}

type chatSelection struct {
	Route   sqlitecli.AccessRoute
	Model   sqlitecli.ProviderModel
	Profile sqlitecli.ProviderProfile
}

type authStatus struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	AccountLabel string `json:"accountLabel"`
	Connected    bool   `json:"connected"`
	Status       string `json:"status"`
	Detail       string `json:"detail"`
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
		store:     store,
		approvals: newApprovalBroker(),
		filters:   chatfilter.NewChain(chatfilter.IdentityFilter{}),
		drivers: provider.NewDriverRegistry(
			provider.CodexAppServerDriver{},
			provider.UnavailableDriver{ProviderID: "openai", Reason: "ChatGPT handoff driver is not implemented yet"},
			provider.ClaudeCLIDriver{},
			provider.UnavailableDriver{ProviderID: "copilot", Reason: "Copilot bridge driver is not implemented yet"},
			provider.UnavailableDriver{ProviderID: "gemini", Reason: "Gemini bridge driver is not implemented yet"},
			provider.UnavailableDriver{ProviderID: "ollama", Reason: "Ollama local model driver is not implemented yet"},
		),
	}
}

func (s Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/state", s.state)
	mux.HandleFunc("GET /api/auth/status", s.authStatus)
	mux.HandleFunc("POST /api/projects", s.createProject)
	mux.HandleFunc("PATCH /api/projects/", s.renameProject)
	mux.HandleFunc("POST /api/projects/", s.projectRoute)
	mux.HandleFunc("DELETE /api/projects/", s.projectRoute)
	mux.HandleFunc("POST /api/provider-profiles/connect", s.connectProviderProfile)
	mux.HandleFunc("GET /api/sessions/search", s.searchSessions)
	mux.HandleFunc("GET /api/sessions/", s.session)
	mux.HandleFunc("PATCH /api/sessions/", s.renameSession)
	mux.HandleFunc("POST /api/sessions/", s.sessionMessage)
	mux.HandleFunc("POST /api/sessions", s.createSession)
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
	})
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
		s.removeProjectRoute(w, r)
		return
	}
	writeError(w, errors.New("method not allowed"), http.StatusMethodNotAllowed)
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
	writeJSON(w, map[string]any{
		"session":          session,
		"messages":         messages,
		"context":          contextUsageOrZero(s.store, sessionID),
		"providerGroupIds": providerGroupIDs,
	})
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

	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")

	userMessage, err := s.store.AddMessage(sessionID, "user", content)
	if err != nil {
		writeStreamEvent(w, "error", map[string]string{"message": err.Error()})
		flusher.Flush()
		return
	}
	writeStreamEvent(w, "message", userMessage)
	flusher.Flush()

	writeStreamEvent(w, "assistant_start", map[string]string{"role": "assistant"})
	flusher.Flush()

	assistantContent, streamed, err := s.runAssistant(r.Context(), sessionID, content, input.ThinkingEffort, selection, func(event provider.Event) {
		switch event.Kind {
		case "delta":
			writeStreamEvent(w, "assistant_delta", map[string]string{"text": event.Text})
		case "status":
			writeStreamEvent(w, "assistant_status", map[string]string{"text": event.Text})
		case "tool_start", "tool_result", "approval_request", "tool_error", "turn_aborted":
			writeStreamEvent(w, event.Kind, toolEventPayloadForSession(sessionID, event))
		}
		flusher.Flush()
	})
	if err != nil {
		writeStreamEvent(w, "error", map[string]string{"message": err.Error()})
		flusher.Flush()
		_ = s.store.AddTokenUsage(sqlitecli.TokenUsageInput{
			ProviderPluginID:  selection.Route.ProviderPluginID,
			ProviderProfileID: selection.Profile.ID,
			SessionID:         sessionID,
			Model:             selection.Model.ModelRef,
			PromptTokens:      estimateTokens(content),
			Status:            "error",
		})
		return
	}
	if strings.TrimSpace(assistantContent) == "" {
		writeStreamEvent(w, "error", map[string]string{"message": "assistant returned an empty response"})
		flusher.Flush()
		return
	}

	if !streamed {
		streamTextChunks(r.Context(), w, flusher, assistantContent)
	}

	assistantMessage, err := s.store.AddMessage(sessionID, "assistant", assistantContent)
	if err != nil {
		writeStreamEvent(w, "error", map[string]string{"message": err.Error()})
		flusher.Flush()
		return
	}
	_ = s.store.AddTokenUsage(sqlitecli.TokenUsageInput{
		ProviderPluginID:  selection.Route.ProviderPluginID,
		ProviderProfileID: selection.Profile.ID,
		SessionID:         sessionID,
		Model:             selection.Model.ModelRef,
		PromptTokens:      estimateTokens(content),
		CompletionTokens:  estimateTokens(assistantContent),
		Status:            "success",
	})
	writeStreamEvent(w, "assistant_done", assistantMessage)
	flusher.Flush()
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

func (s Server) resolveChatSelection(sessionID string, routeID string, modelID string) (chatSelection, error) {
	routeID = strings.TrimSpace(routeID)
	modelID = strings.TrimSpace(modelID)
	if routeID == "" {
		return chatSelection{}, errors.New("chat route is required")
	}
	if modelID == "" {
		return chatSelection{}, errors.New("chat model is required")
	}

	project, err := s.store.DefaultProject()
	if err != nil {
		return chatSelection{}, err
	}
	if sessionID != sessionIDForCapabilityCheck {
		if session, _, sessionErr := s.store.GetSession(sessionID); sessionErr == nil && strings.TrimSpace(session.ProjectID) != "" {
			project.ID = session.ProjectID
		}
	}
	projectRoutes, err := s.store.ListProjectAccessRoutes(project.ID)
	if err != nil {
		return chatSelection{}, err
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

	models, err := s.store.ListProviderModels()
	if err != nil {
		return chatSelection{}, err
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
		return chatSelection{}, fmt.Errorf("%s is not executable from Ergo Loom chat yet", selectedRoute.DisplayName)
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
			return authStatus{ID: "claude", Label: "Claude", AccountLabel: "", Connected: false, Status: "signed_out", Detail: "Claude CLI is installed but not logged in"}
		}
		if detail == "" {
			detail = runErr.Error()
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
	path, err := exec.LookPath("codex")
	if err != nil && strings.TrimSpace(os.Getenv("CODEX_EXEC")) != "" {
		path = strings.TrimSpace(os.Getenv("CODEX_EXEC"))
		err = nil
	}
	if err != nil {
		path = "/Applications/Codex.app/Contents/Resources/codex"
		if _, statErr := os.Stat(path); statErr != nil {
			return authStatus{ID: "codex", Label: "Codex/ChatGPT", AccountLabel: "", Connected: false, Status: "missing", Detail: "Codex local runtime was not found"}
		}
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
