package provider

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jkj-dev/ergo-loom/internal/toolruntime"
)

type ChatRequest struct {
	SessionID         string
	RouteID           string
	RouteDisplayName  string
	RouteTransport    string
	ProviderPluginID  string
	ProviderProfileID string
	ModelID           string
	ModelDisplayName  string
	ModelRef          string
	Input             string
	ThinkingEffort    string
	ExternalThreadID  string
	ApprovalHandler   func(context.Context, toolruntime.Event) (string, error)
}

type ChatResponse struct {
	Text             string
	ExternalThreadID string
	Streamed         bool
}

type ChatDriver interface {
	ProviderPluginID() string
	CanExecute(ChatRequest) bool
	Respond(ctx context.Context, request ChatRequest, onEvent func(Event)) (ChatResponse, error)
}

type DriverRegistry struct {
	drivers map[string]ChatDriver
}

func NewDriverRegistry(drivers ...ChatDriver) DriverRegistry {
	registry := DriverRegistry{drivers: make(map[string]ChatDriver, len(drivers))}
	for _, driver := range drivers {
		if driver == nil {
			continue
		}
		providerID := strings.TrimSpace(driver.ProviderPluginID())
		if providerID == "" {
			continue
		}
		registry.drivers[providerID] = driver
	}
	return registry
}

func (r DriverRegistry) Get(providerPluginID string) (ChatDriver, bool) {
	driver, ok := r.drivers[strings.TrimSpace(providerPluginID)]
	return driver, ok
}

func (r DriverRegistry) CanExecute(request ChatRequest) bool {
	driver, ok := r.Get(request.ProviderPluginID)
	return ok && driver.CanExecute(request)
}

type CodexAppServerDriver struct {
	Command string
	WorkDir string
}

func (d CodexAppServerDriver) ProviderPluginID() string {
	return "codex"
}

func (d CodexAppServerDriver) CanExecute(request ChatRequest) bool {
	return request.ProviderPluginID == "codex" && request.RouteTransport == "app_server"
}

func (d CodexAppServerDriver) Respond(ctx context.Context, request ChatRequest, onEvent func(Event)) (ChatResponse, error) {
	if !d.CanExecute(request) {
		return ChatResponse{}, fmt.Errorf("%s is not executable by the Codex app-server driver", request.RouteDisplayName)
	}
	runner := NewCodexAppServerRunner(d.WorkDir)
	runner.Command = firstNonEmpty(d.Command, runner.Command)
	runner.Model = request.ModelRef
	runner.Effort = request.ThinkingEffort
	runner.ApprovalHandler = request.ApprovalHandler

	response, err := runner.RespondInThread(ctx, request.ExternalThreadID, request.Input, onEvent)
	return ChatResponse{
		Text:             response.Text,
		ExternalThreadID: response.ThreadID,
		Streamed:         response.Streamed,
	}, err
}

type ClaudeCLIDriver struct {
	Command string
	WorkDir string
}

func (d ClaudeCLIDriver) ProviderPluginID() string {
	return "anthropic"
}

func (d ClaudeCLIDriver) CanExecute(request ChatRequest) bool {
	return request.ProviderPluginID == "anthropic" && (request.RouteTransport == "claude_cli" || request.RouteTransport == "manual")
}

func (d ClaudeCLIDriver) Respond(ctx context.Context, request ChatRequest, onEvent func(Event)) (ChatResponse, error) {
	if !d.CanExecute(request) {
		return ChatResponse{}, fmt.Errorf("%s is not executable by the Claude CLI driver", request.RouteDisplayName)
	}
	if request.RouteTransport == "manual" {
		return d.respondWithBrowserHandoff(ctx, request, onEvent)
	}
	command, err := claudeCommand(d.Command)
	if err != nil {
		return ChatResponse{}, err
	}
	sessionID := request.ExternalThreadID
	if sessionID == "" {
		sessionID = newUUID()
	}

	args := []string{
		"--print",
		"--output-format", "stream-json",
		"--verbose",
		"--include-partial-messages",
		"--session-id", sessionID,
		"--system-prompt", ergoLoomIdentityInstructions(),
		"--permission-mode", "default",
		"--tools", "",
	}
	if model := claudeModel(request.ModelRef); model != "" {
		args = append(args, "--model", model)
	}
	if effort := claudeEffort(request.ThinkingEffort); effort != "" {
		args = append(args, "--effort", effort)
	}
	args = append(args, request.Input)

	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = firstNonEmpty(d.WorkDir, mustGetwd())
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return ChatResponse{}, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return ChatResponse{}, err
	}

	onEvent(Event{Kind: "status", Text: "Attached Claude CLI session"})
	var assistant strings.Builder
	var streamed bool
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		event := claudeStreamEvent(scanner.Bytes())
		if event.Kind == "" {
			continue
		}
		if event.Kind == "delta" {
			assistant.WriteString(event.Text)
			streamed = true
		}
		if event.Kind == "final" {
			assistant.Reset()
			assistant.WriteString(event.Text)
			continue
		}
		onEvent(event)
	}
	if err := scanner.Err(); err != nil {
		_ = cmd.Wait()
		return ChatResponse{ExternalThreadID: sessionID, Streamed: streamed}, err
	}
	if err := cmd.Wait(); err != nil {
		return ChatResponse{ExternalThreadID: sessionID, Streamed: streamed}, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	final := strings.TrimSpace(assistant.String())
	if final == "" {
		return ChatResponse{ExternalThreadID: sessionID, Streamed: streamed}, errors.New("claude CLI returned an empty response")
	}
	return ChatResponse{Text: final, ExternalThreadID: sessionID, Streamed: streamed}, nil
}

func (d ClaudeCLIDriver) respondWithBrowserHandoff(ctx context.Context, request ChatRequest, onEvent func(Event)) (ChatResponse, error) {
	bridgeURL := strings.TrimRight(strings.TrimSpace(os.Getenv("ERGO_LOOM_HANDOFF_BRIDGE_URL")), "/")
	if bridgeURL == "" {
		onEvent(Event{Kind: "status", Text: "Claude web handoff requires the Ergo Loom desktop browser worker"})
		text := strings.Join([]string{
			"Claude Code CLI를 사용할 수 없는 상태라 Claude 무료/웹 계정 handoff 경로로 내려왔습니다.",
			"",
			"이 경로는 Ergo Loom 데스크톱 앱의 내장 브라우저 worker가 필요합니다.",
			"현재 브라우저 개발 서버에서는 Claude 웹 세션을 앱 내부에서 조작할 수 없어서, 설치형 앱/Electron 런타임에서 다시 시도해야 합니다.",
		}, "\n")
		return ChatResponse{Text: text, Streamed: false}, nil
	}

	onEvent(Event{Kind: "status", Text: "Sending request to Ergo Loom Claude web worker"})
	payload := map[string]string{
		"provider":         request.ProviderPluginID,
		"sessionId":        request.SessionID,
		"externalThreadId": request.ExternalThreadID,
		"input":            request.Input,
		"modelRef":         request.ModelRef,
		"modelDisplayName": request.ModelDisplayName,
		"thinkingEffort":   request.ThinkingEffort,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return ChatResponse{}, err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, bridgeURL+"/v1/claude/chat", bytes.NewReader(body))
	if err != nil {
		return ChatResponse{}, err
	}
	httpRequest.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 2 * time.Minute}
	response, err := client.Do(httpRequest)
	if err != nil {
		return ChatResponse{}, err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 2*1024*1024))
	if err != nil {
		return ChatResponse{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return ChatResponse{}, fmt.Errorf("Claude web handoff worker failed: %s", strings.TrimSpace(string(responseBody)))
	}

	var result struct {
		Text             string `json:"text"`
		ExternalThreadID string `json:"externalThreadId"`
	}
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return ChatResponse{}, err
	}
	if strings.TrimSpace(result.Text) == "" {
		return ChatResponse{}, errors.New("Claude web handoff worker returned an empty response")
	}
	onEvent(Event{Kind: "status", Text: "Received Claude web response inside Ergo Loom"})
	return ChatResponse{
		Text:             result.Text,
		ExternalThreadID: firstNonEmpty(result.ExternalThreadID, request.ExternalThreadID),
		Streamed:         false,
	}, nil
}

func claudeCommand(override string) (string, error) {
	candidates := []string{
		override,
		os.Getenv("ERGO_CLAUDE_COMMAND"),
		"claude",
		filepath.Join(os.Getenv("HOME"), ".local", "bin", "claude"),
		"/opt/homebrew/bin/claude",
		"/usr/local/bin/claude",
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
	return "", errors.New("Claude CLI was not found; set ERGO_CLAUDE_COMMAND or add claude to PATH")
}

func claudeEffort(effort string) string {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "low", "medium", "high":
		return strings.ToLower(strings.TrimSpace(effort))
	case "very_high", "very-high", "very high":
		return "xhigh"
	default:
		return "medium"
	}
}

func claudeModel(modelRef string) string {
	switch strings.TrimSpace(modelRef) {
	case "claude-sonnet-4.6", "claude-sonnet-4-6":
		return "sonnet"
	case "claude-haiku-4.5", "claude-haiku-4-5":
		return "haiku"
	case "claude-opus-4.8", "claude-opus-4-8":
		return "opus"
	default:
		return strings.TrimSpace(modelRef)
	}
}

func claudeStreamEvent(line []byte) Event {
	var message map[string]any
	if err := json.Unmarshal(line, &message); err != nil {
		return Event{}
	}
	eventType := firstString(message, "type")
	text := firstNonEmpty(
		firstString(message, "delta.text", "delta.partial_json", "message.content.0.text", "content.0.text", "result", "text"),
		claudeContentText(message["message"]),
		claudeContentText(message["content"]),
	)
	switch eventType {
	case "assistant", "result":
		if text != "" {
			return Event{Kind: "final", Text: text}
		}
	case "content_block_delta", "message_delta", "partial":
		if text != "" {
			return Event{Kind: "delta", Text: text}
		}
	case "system", "user":
		return Event{}
	}
	if text != "" {
		return Event{Kind: "delta", Text: text}
	}
	return Event{}
}

func claudeContentText(value any) string {
	switch typed := value.(type) {
	case []any:
		var out strings.Builder
		for _, item := range typed {
			if object, ok := item.(map[string]any); ok {
				out.WriteString(firstString(object, "text"))
			}
		}
		return out.String()
	case map[string]any:
		return claudeContentText(typed["content"])
	default:
		return ""
	}
}

func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]),
		hex.EncodeToString(b[4:6]),
		hex.EncodeToString(b[6:8]),
		hex.EncodeToString(b[8:10]),
		hex.EncodeToString(b[10:16]),
	)
}

func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

type UnavailableDriver struct {
	ProviderID string
	Reason     string
}

func (d UnavailableDriver) ProviderPluginID() string {
	return d.ProviderID
}

func (d UnavailableDriver) CanExecute(ChatRequest) bool {
	return false
}

func (d UnavailableDriver) Respond(context.Context, ChatRequest, func(Event)) (ChatResponse, error) {
	if strings.TrimSpace(d.Reason) != "" {
		return ChatResponse{}, errors.New(d.Reason)
	}
	return ChatResponse{}, errors.New("provider driver is not implemented yet")
}
