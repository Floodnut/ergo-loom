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

	"github.com/Floodnut/ergo-loom/internal/toolruntime"
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

type DriverErrorKind string

const (
	ErrKindTransient   DriverErrorKind = "transient"
	ErrKindAuthFailure DriverErrorKind = "auth_failure"
	ErrKindRateLimit   DriverErrorKind = "rate_limit"
	ErrKindSessionEnd  DriverErrorKind = "session_end"
	ErrKindUnavailable DriverErrorKind = "unavailable"
	ErrKindFatal       DriverErrorKind = "fatal"
)

type DriverError struct {
	Kind      DriverErrorKind
	Message   string
	Retryable bool
}

func (e *DriverError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

type ChatDriver interface {
	ProviderPluginID() string
	CanExecute(ChatRequest) bool
	Respond(ctx context.Context, request ChatRequest, onEvent func(Event)) (ChatResponse, error)
	Ping(ctx context.Context) error
}

// DriverRegistry maps provider requests to chat drivers. Multiple drivers may
// share the same provider plugin ID (e.g. ClaudeCLIDriver and
// ClaudeSDKBridgeDriver are both "anthropic"). GetForRequest finds the first
// driver whose CanExecute returns true for the specific request transport.
type DriverRegistry struct {
	drivers []ChatDriver
}

func NewDriverRegistry(drivers ...ChatDriver) DriverRegistry {
	var list []ChatDriver
	for _, d := range drivers {
		if d == nil || strings.TrimSpace(d.ProviderPluginID()) == "" {
			continue
		}
		list = append(list, d)
	}
	return DriverRegistry{drivers: list}
}

// GetForRequest returns the first driver that can handle this specific request.
func (r DriverRegistry) GetForRequest(request ChatRequest) (ChatDriver, bool) {
	for _, d := range r.drivers {
		if d.ProviderPluginID() == request.ProviderPluginID && d.CanExecute(request) {
			return d, true
		}
	}
	return nil, false
}

// Get returns the first driver registered for providerPluginID regardless of
// transport. Used for ping/diagnostics where no specific request is available.
func (r DriverRegistry) Get(providerPluginID string) (ChatDriver, bool) {
	providerPluginID = strings.TrimSpace(providerPluginID)
	for _, d := range r.drivers {
		if d.ProviderPluginID() == providerPluginID {
			return d, true
		}
	}
	return nil, false
}

func (r DriverRegistry) CanExecute(request ChatRequest) bool {
	_, ok := r.GetForRequest(request)
	return ok
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
		return ChatResponse{}, driverError(ErrKindUnavailable, "%s is not executable by the Codex app-server driver", request.RouteDisplayName)
	}
	runner := NewCodexAppServerRunner(d.WorkDir)
	runner.Command = firstNonEmpty(d.Command, runner.Command)
	runner.Model = request.ModelRef
	runner.Effort = request.ThinkingEffort
	runner.ApprovalHandler = request.ApprovalHandler

	response, err := runner.RespondInThread(ctx, request.ExternalThreadID, request.Input, onEvent)
	if err != nil {
		return ChatResponse{
			Text:             response.Text,
			ExternalThreadID: response.ThreadID,
			Streamed:         response.Streamed,
		}, classifyCodexDriverError(err)
	}
	return ChatResponse{
		Text:             response.Text,
		ExternalThreadID: response.ThreadID,
		Streamed:         response.Streamed,
	}, nil
}

func (d CodexAppServerDriver) Ping(context.Context) error { return nil }

type ClaudeCLIDriver struct {
	Command string
	WorkDir string
}

func (d ClaudeCLIDriver) ProviderPluginID() string {
	return "anthropic"
}

func (d ClaudeCLIDriver) CanExecute(request ChatRequest) bool {
	return request.ProviderPluginID == "anthropic" && request.RouteTransport == "claude_cli"
}

func (d ClaudeCLIDriver) Respond(ctx context.Context, request ChatRequest, onEvent func(Event)) (ChatResponse, error) {
	if !d.CanExecute(request) {
		return ChatResponse{}, driverError(ErrKindUnavailable, "%s is not executable by the Claude CLI driver", request.RouteDisplayName)
	}
	if request.RouteTransport == "manual" {
		return d.respondWithBrowserHandoff(ctx, request, onEvent)
	}
	command, err := claudeCommand(d.Command)
	if err != nil {
		return ChatResponse{}, classifyClaudeDriverError(err.Error())
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
		"--system-prompt", ergoLoomIdentityInstructions(),
		"--permission-mode", "default",
		"--tools", "",
	}
	if request.ExternalThreadID != "" {
		args = append(args, "--resume", request.ExternalThreadID)
	} else {
		args = append(args, "--session-id", sessionID)
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
		return ChatResponse{}, driverError(ErrKindFatal, err.Error())
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return ChatResponse{}, classifyClaudeDriverError(err.Error())
	}

	var assistant strings.Builder
	var streamed bool
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if parsedSessionID := claudeSessionID(line); parsedSessionID != "" {
			sessionID = parsedSessionID
		}
		event := claudeStreamEvent(line)
		if event.Kind == "" {
			continue
		}
		if event.Kind == EventKindDelta {
			assistant.WriteString(event.Text)
			streamed = true
		}
		if event.Kind == eventKindFinal {
			assistant.Reset()
			assistant.WriteString(event.Text)
			continue
		}
		onEvent(event)
	}
	if err := scanner.Err(); err != nil {
		_ = cmd.Wait()
		return ChatResponse{ExternalThreadID: sessionID, Streamed: streamed}, driverError(ErrKindTransient, err.Error())
	}
	if err := cmd.Wait(); err != nil {
		return ChatResponse{ExternalThreadID: sessionID, Streamed: streamed}, classifyClaudeDriverError(claudeCLIErrorDetail(firstNonEmpty(stderr.String(), assistant.String())))
	}
	final := strings.TrimSpace(assistant.String())
	if final == "" {
		return ChatResponse{ExternalThreadID: sessionID, Streamed: streamed}, driverError(ErrKindTransient, "claude CLI returned an empty response")
	}
	return ChatResponse{Text: final, ExternalThreadID: sessionID, Streamed: streamed}, nil
}

func (d ClaudeCLIDriver) Ping(ctx context.Context) error {
	command, err := claudeCommand(d.Command)
	if err != nil {
		return classifyClaudeDriverError(err.Error())
	}
	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(pingCtx, command, "--version")
	if err := cmd.Run(); err != nil {
		return classifyClaudeDriverError(err.Error())
	}
	return nil
}

// ClaudeSDKBridgeDriver calls the Ergo Loom Python bridge that wraps
// claude_code_sdk. The bridge streams SSE events so the driver can emit
// token deltas in real time.
//
// Set ERGO_CLAUDE_SDK_BRIDGE_URL (default http://127.0.0.1:3764) and start
// the bridge with: python tools/claude-sdk-bridge/main.py
type ClaudeSDKBridgeDriver struct {
	BridgeURL string
}

func (d ClaudeSDKBridgeDriver) ProviderPluginID() string { return "anthropic" }

func (d ClaudeSDKBridgeDriver) CanExecute(request ChatRequest) bool {
	return request.ProviderPluginID == "anthropic" &&
		request.RouteTransport == "claude_sdk_bridge" &&
		strings.TrimSpace(d.explicitBridgeURL()) != ""
}

// explicitBridgeURL returns the bridge URL only when explicitly configured via
// BridgeURL field or ERGO_CLAUDE_SDK_BRIDGE_URL env. Falls back to empty string
// (not the default port) so CanExecute=false when the bridge is not set up,
// avoiding an unnecessary connection failure before falling through to ClaudeCLIDriver.
func (d ClaudeSDKBridgeDriver) explicitBridgeURL() string {
	return strings.TrimRight(strings.TrimSpace(firstNonEmpty(d.BridgeURL, os.Getenv("ERGO_CLAUDE_SDK_BRIDGE_URL"))), "/")
}

func (d ClaudeSDKBridgeDriver) bridgeURL() string {
	return strings.TrimRight(
		strings.TrimSpace(firstNonEmpty(d.BridgeURL, os.Getenv("ERGO_CLAUDE_SDK_BRIDGE_URL"), "http://127.0.0.1:3764")),
		"/",
	)
}

func (d ClaudeSDKBridgeDriver) Respond(ctx context.Context, request ChatRequest, onEvent func(Event)) (ChatResponse, error) {
	if !d.CanExecute(request) {
		return ChatResponse{}, driverError(ErrKindUnavailable, "Claude SDK bridge is not available; start tools/claude-sdk-bridge/main.py and set ERGO_CLAUDE_SDK_BRIDGE_URL")
	}

	payload := map[string]string{
		"prompt":         request.Input,
		"sessionId":      request.ExternalThreadID,
		"modelRef":       request.ModelRef,
		"thinkingEffort": request.ThinkingEffort,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return ChatResponse{}, driverError(ErrKindFatal, err.Error())
	}

	onEvent(Event{Kind: EventKindStatus, Text: "Connecting to Claude SDK bridge"})

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, d.bridgeURL()+"/v1/claude/chat", bytes.NewReader(body))
	if err != nil {
		return ChatResponse{}, driverError(ErrKindFatal, err.Error())
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := (&http.Client{Timeout: 10 * time.Minute}).Do(httpReq)
	if err != nil {
		return ChatResponse{}, driverError(ErrKindTransient, "Claude SDK bridge unreachable: "+err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return ChatResponse{}, httpDriverError(resp.StatusCode, "Claude SDK bridge: "+strings.TrimSpace(string(msg)))
	}

	var fullText strings.Builder
	var sessionID = request.ExternalThreadID
	var streamed bool

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		var ev struct {
			Type    string          `json:"type"`
			Payload json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			continue
		}
		switch ev.Type {
		case "delta":
			var p struct{ Text string `json:"text"` }
			if err := json.Unmarshal(ev.Payload, &p); err == nil && p.Text != "" {
				fullText.WriteString(p.Text)
				streamed = true
				onEvent(Event{Kind: EventKindDelta, Text: p.Text})
			}
		case "tool_start":
			var p struct {
				ToolName string `json:"toolName"`
				ToolID   string `json:"toolId"`
				Command  string `json:"command"`
			}
			if err := json.Unmarshal(ev.Payload, &p); err == nil {
				onEvent(Event{Kind: EventKindToolStart, Tool: &toolruntime.Event{
					Type:     "sdk_tool",
					ToolID:   firstNonEmpty(p.ToolID, p.ToolName),
					ToolName: p.ToolName,
					Command:  p.Command,
					Text:     "Claude requested " + p.ToolName,
					Status:   "started",
				}})
			}
		case "done":
			var p struct {
				Text      string `json:"text"`
				SessionID string `json:"sessionId"`
			}
			if err := json.Unmarshal(ev.Payload, &p); err == nil {
				if p.SessionID != "" {
					sessionID = p.SessionID
				}
				if !streamed && strings.TrimSpace(p.Text) != "" {
					fullText.Reset()
					fullText.WriteString(p.Text)
				}
			}
		case "error":
			var p struct{ Message string `json:"message"` }
			if err := json.Unmarshal(ev.Payload, &p); err == nil {
				return ChatResponse{ExternalThreadID: sessionID, Streamed: streamed},
					driverError(ErrKindTransient, "Claude SDK bridge error: "+p.Message)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return ChatResponse{ExternalThreadID: sessionID, Streamed: streamed}, driverError(ErrKindTransient, err.Error())
	}

	final := strings.TrimSpace(fullText.String())
	if final == "" {
		return ChatResponse{ExternalThreadID: sessionID, Streamed: streamed},
			driverError(ErrKindTransient, "Claude SDK bridge returned an empty response")
	}
	return ChatResponse{Text: final, ExternalThreadID: sessionID, Streamed: streamed}, nil
}

func (d ClaudeSDKBridgeDriver) Ping(ctx context.Context) error {
	url := d.bridgeURL()
	if url == "" {
		return driverError(ErrKindUnavailable, "Claude SDK bridge URL is not configured")
	}
	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(pingCtx, http.MethodGet, url+"/healthz", nil)
	if err != nil {
		return driverError(ErrKindFatal, err.Error())
	}
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return driverError(ErrKindTransient, "Claude SDK bridge unreachable: "+err.Error())
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return driverError(ErrKindTransient, fmt.Sprintf("Claude SDK bridge health check failed: %d", resp.StatusCode))
	}
	return nil
}

type CopilotBridgeDriver struct {
	BridgeURL string
}

func (d CopilotBridgeDriver) ProviderPluginID() string {
	return "copilot"
}

func (d CopilotBridgeDriver) CanExecute(request ChatRequest) bool {
	if request.ProviderPluginID != "copilot" {
		return false
	}
	if request.RouteTransport != "copilot_sdk_jsonrpc" && request.RouteTransport != "ide_bridge" {
		return false
	}
	return strings.TrimSpace(firstNonEmpty(d.BridgeURL, os.Getenv("ERGO_COPILOT_BRIDGE_URL"))) != ""
}

func (d CopilotBridgeDriver) Respond(ctx context.Context, request ChatRequest, onEvent func(Event)) (ChatResponse, error) {
	if !d.CanExecute(request) {
		return ChatResponse{}, driverError(ErrKindUnavailable, "Copilot bridge is not configured. Start an Ergo Loom Copilot bridge worker and set ERGO_COPILOT_BRIDGE_URL.")
	}
	bridgeURL := strings.TrimRight(strings.TrimSpace(firstNonEmpty(d.BridgeURL, os.Getenv("ERGO_COPILOT_BRIDGE_URL"))), "/")
	onEvent(Event{Kind: EventKindStatus, Text: "Sending request to Ergo Loom Copilot bridge"})
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
		return ChatResponse{}, driverError(ErrKindFatal, err.Error())
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, bridgeURL+"/v1/copilot/chat", bytes.NewReader(body))
	if err != nil {
		return ChatResponse{}, driverError(ErrKindFatal, err.Error())
	}
	httpRequest.Header.Set("Content-Type", "application/json")

	response, err := (&http.Client{Timeout: 2 * time.Minute}).Do(httpRequest)
	if err != nil {
		return ChatResponse{}, driverError(ErrKindTransient, err.Error())
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 2*1024*1024))
	if err != nil {
		return ChatResponse{}, driverError(ErrKindTransient, err.Error())
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return ChatResponse{}, httpDriverError(response.StatusCode, "Copilot bridge failed: "+strings.TrimSpace(string(responseBody)))
	}
	var result struct {
		Text             string `json:"text"`
		ExternalThreadID string `json:"externalThreadId"`
	}
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return ChatResponse{}, driverError(ErrKindFatal, err.Error())
	}
	if strings.TrimSpace(result.Text) == "" {
		return ChatResponse{}, driverError(ErrKindTransient, "Copilot bridge returned an empty response")
	}
	onEvent(Event{Kind: EventKindStatus, Text: "Received Copilot bridge response"})
	return ChatResponse{Text: result.Text, ExternalThreadID: result.ExternalThreadID, Streamed: false}, nil
}

func (d CopilotBridgeDriver) Ping(ctx context.Context) error {
	bridgeURL := strings.TrimRight(strings.TrimSpace(firstNonEmpty(d.BridgeURL, os.Getenv("ERGO_COPILOT_BRIDGE_URL"))), "/")
	if bridgeURL == "" {
		return driverError(ErrKindUnavailable, "Copilot bridge is not configured")
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, bridgeURL+"/healthz", nil)
	if err != nil {
		return driverError(ErrKindFatal, err.Error())
	}
	response, err := (&http.Client{Timeout: 3 * time.Second}).Do(httpRequest)
	if err != nil {
		return driverError(ErrKindTransient, err.Error())
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return httpDriverError(response.StatusCode, "Copilot bridge health check failed")
	}
	return nil
}

func (d ClaudeCLIDriver) respondWithBrowserHandoff(ctx context.Context, request ChatRequest, onEvent func(Event)) (ChatResponse, error) {
	bridgeURL := strings.TrimRight(strings.TrimSpace(os.Getenv("ERGO_LOOM_HANDOFF_BRIDGE_URL")), "/")
	if bridgeURL == "" {
		onEvent(Event{Kind: EventKindStatus, Text: "Claude web handoff requires the Ergo Loom desktop browser worker"})
		text := strings.Join([]string{
			"Claude Code CLI를 사용할 수 없는 상태라 Claude 무료/웹 계정 handoff 경로로 내려왔습니다.",
			"",
			"이 경로는 Ergo Loom 데스크톱 앱의 내장 브라우저 worker가 필요합니다.",
			"현재 브라우저 개발 서버에서는 Claude 웹 세션을 앱 내부에서 조작할 수 없어서, 설치형 앱/Electron 런타임에서 다시 시도해야 합니다.",
		}, "\n")
		return ChatResponse{Text: text, Streamed: false}, nil
	}

	onEvent(Event{Kind: EventKindStatus, Text: "Sending request to Ergo Loom Claude web worker"})
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
		return ChatResponse{}, driverError(ErrKindFatal, err.Error())
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, bridgeURL+"/v1/claude/chat", bytes.NewReader(body))
	if err != nil {
		return ChatResponse{}, driverError(ErrKindFatal, err.Error())
	}
	httpRequest.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 2 * time.Minute}
	response, err := client.Do(httpRequest)
	if err != nil {
		return ChatResponse{}, driverError(ErrKindTransient, err.Error())
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 2*1024*1024))
	if err != nil {
		return ChatResponse{}, driverError(ErrKindTransient, err.Error())
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return ChatResponse{}, httpDriverError(response.StatusCode, "Claude web handoff worker failed: "+strings.TrimSpace(string(responseBody)))
	}

	var result struct {
		Text             string `json:"text"`
		ExternalThreadID string `json:"externalThreadId"`
	}
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return ChatResponse{}, driverError(ErrKindFatal, err.Error())
	}
	if strings.TrimSpace(result.Text) == "" {
		return ChatResponse{}, driverError(ErrKindTransient, "Claude web handoff worker returned an empty response")
	}
	onEvent(Event{Kind: EventKindStatus, Text: "Received Claude web response inside Ergo Loom"})
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

func claudeSessionID(line []byte) string {
	var message map[string]any
	if err := json.Unmarshal(line, &message); err != nil {
		return ""
	}
	return firstString(message, "session_id", "sessionId", "message.session_id", "message.sessionId")
}

func claudeStreamEvent(line []byte) Event {
	var message map[string]any
	if err := json.Unmarshal(line, &message); err != nil {
		return Event{}
	}
	eventType := firstString(message, "type")
	subtype := firstString(message, "subtype")
	if eventType == "system" {
		if subtype == "init" {
			return Event{Kind: EventKindStatus, Text: "Attached Claude CLI session"}
		}
		return Event{}
	}
	if toolEvent := claudeToolEvent(message); toolEvent.Kind != "" {
		return toolEvent
	}
	text := firstNonEmpty(
		firstString(message,
			"delta.text",
			"delta.partial_json",
			"message.content.0.text",
			"message.delta.text",
			"content.0.text",
			"result",
			"text",
		),
		claudeContentText(message["message"]),
		claudeContentText(message["content"]),
	)
	switch eventType {
	case "assistant", "result":
		if text != "" {
			return Event{Kind: eventKindFinal, Text: text}
		}
	case "content_block_delta", "message_delta", "partial":
		if text != "" {
			return Event{Kind: EventKindDelta, Text: text}
		}
	case "system", "user":
		return Event{}
	}
	if text != "" {
		return Event{Kind: EventKindDelta, Text: text}
	}
	return Event{}
}

func claudeToolEvent(message map[string]any) Event {
	eventType := firstString(message, "type")
	for _, block := range claudeContentBlocks(message) {
		blockType := firstString(block, "type")
		switch blockType {
		case "tool_use":
			name := firstNonEmpty(firstString(block, "name"), "tool")
			return Event{
				Kind: EventKindToolStart,
				Tool: &toolruntime.Event{
					Type:         "claude_tool",
					ToolID:       firstNonEmpty(firstString(block, "id"), name),
					ToolName:     name,
					InvocationID: firstString(block, "id"),
					Command:      claudeToolInputSummary(block["input"]),
					Text:         "Claude requested " + name,
					Status:       "started",
					Payload:      block,
				},
			}
		case "tool_result":
			toolID := firstString(block, "tool_use_id")
			return Event{
				Kind: EventKindToolResult,
				Tool: &toolruntime.Event{
					Type:         "claude_tool",
					ToolID:       firstNonEmpty(toolID, "tool"),
					ToolName:     firstNonEmpty(firstString(block, "name"), "tool"),
					InvocationID: toolID,
					Text:         firstNonEmpty(claudeContentText(block["content"]), firstString(block, "content")),
					Status:       "completed",
					Payload:      block,
				},
			}
		}
	}
	if eventType == "error" {
		return Event{Kind: EventKindToolError, Text: firstNonEmpty(firstString(message, "error.message", "message"), "Claude CLI stream error")}
	}
	return Event{}
}

func claudeContentBlocks(message map[string]any) []map[string]any {
	values := []any{message["content"]}
	if nested, ok := message["message"].(map[string]any); ok {
		values = append(values, nested["content"])
	}
	for _, value := range values {
		items, ok := value.([]any)
		if !ok {
			continue
		}
		blocks := make([]map[string]any, 0, len(items))
		for _, item := range items {
			if block, ok := item.(map[string]any); ok {
				blocks = append(blocks, block)
			}
		}
		if len(blocks) > 0 {
			return blocks
		}
	}
	return nil
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

func claudeToolInputSummary(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		if command := firstString(typed, "command"); command != "" {
			return command
		}
		encoded, err := json.Marshal(typed)
		if err == nil {
			return string(encoded)
		}
	case string:
		return typed
	}
	return ""
}

func claudeCLIErrorDetail(stderr string) string {
	detail := strings.TrimSpace(stderr)
	if strings.Contains(strings.ToLower(detail), "not logged in") {
		return "Claude CLI is not logged in with a subscription token. Run `claude setup-token` with your Pro/Max subscription account, then refresh provider status in Ergo Loom."
	}
	if strings.Contains(strings.ToLower(detail), "credit balance too low") {
		return "Claude CLI is using API billing and reports low credit balance. Run `claude setup-token` with your Pro/Max subscription account so Ergo Loom can use the subscription route instead of API billing."
	}
	if detail == "" {
		return "Claude CLI command failed"
	}
	return detail
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
		return ChatResponse{}, driverError(ErrKindUnavailable, d.Reason)
	}
	return ChatResponse{}, driverError(ErrKindUnavailable, "provider driver is not implemented yet")
}

func (d UnavailableDriver) Ping(context.Context) error {
	if strings.TrimSpace(d.Reason) != "" {
		return driverError(ErrKindUnavailable, d.Reason)
	}
	return driverError(ErrKindUnavailable, "provider driver is not implemented yet")
}

func driverError(kind DriverErrorKind, format string, args ...any) *DriverError {
	message := fmt.Sprintf(format, args...)
	if len(args) == 0 {
		message = format
	}
	return &DriverError{
		Kind:      kind,
		Message:   message,
		Retryable: kind == ErrKindTransient || kind == ErrKindRateLimit || kind == ErrKindSessionEnd,
	}
}

func httpDriverError(statusCode int, message string) *DriverError {
	switch {
	case statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden:
		return driverError(ErrKindAuthFailure, message)
	case statusCode == http.StatusTooManyRequests:
		return driverError(ErrKindRateLimit, message)
	case statusCode >= 500:
		return driverError(ErrKindTransient, message)
	default:
		return driverError(ErrKindFatal, message)
	}
}

func classifyCodexDriverError(err error) error {
	if err == nil {
		return nil
	}
	var driverErr *DriverError
	if errors.As(err, &driverErr) {
		return driverErr
	}
	message := err.Error()
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "401") || strings.Contains(lower, "403") || strings.Contains(lower, "auth") || strings.Contains(lower, "login"):
		return driverError(ErrKindAuthFailure, message)
	case strings.Contains(lower, "429") || strings.Contains(lower, "rate limit"):
		return driverError(ErrKindRateLimit, message)
	case strings.Contains(lower, "thread") && (strings.Contains(lower, "not found") || strings.Contains(lower, "expired") || strings.Contains(lower, "ended")):
		return driverError(ErrKindSessionEnd, message)
	case strings.Contains(lower, "timeout") || strings.Contains(lower, "temporary") || strings.Contains(lower, "connection"):
		return driverError(ErrKindTransient, message)
	default:
		return driverError(ErrKindFatal, message)
	}
}

func classifyClaudeDriverError(message string) error {
	message = strings.TrimSpace(message)
	if message == "" {
		return driverError(ErrKindFatal, "Claude CLI command failed")
	}
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "not logged in") || strings.Contains(lower, "login") || strings.Contains(lower, "setup-token") || strings.Contains(lower, "auth"):
		return driverError(ErrKindAuthFailure, message)
	case strings.Contains(lower, "credit balance too low") || strings.Contains(lower, "rate limit") || strings.Contains(lower, "too many requests"):
		return driverError(ErrKindRateLimit, message)
	case strings.Contains(lower, "session") && (strings.Contains(lower, "expired") || strings.Contains(lower, "not found") || strings.Contains(lower, "ended")):
		return driverError(ErrKindSessionEnd, message)
	case strings.Contains(lower, "timeout") || strings.Contains(lower, "temporar") || strings.Contains(lower, "connection"):
		return driverError(ErrKindTransient, message)
	default:
		return driverError(ErrKindFatal, message)
	}
}
