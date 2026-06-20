package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jkj-dev/ergo-loom/internal/toolruntime"
)

type Event struct {
	Kind string
	Text string
	Tool *toolruntime.Event
}

type Runner interface {
	Respond(ctx context.Context, prompt string, onEvent func(Event)) (string, error)
}

type CodexRunner struct {
	Command string
	WorkDir string
}

func NewCodexRunner(workDir string) CodexRunner {
	command := os.Getenv("ERGO_CODEX_COMMAND")
	if command == "" {
		command = "codex"
	}
	return CodexRunner{
		Command: command,
		WorkDir: workDir,
	}
}

func (r CodexRunner) Respond(ctx context.Context, prompt string, onEvent func(Event)) (string, error) {
	if strings.TrimSpace(prompt) == "" {
		return "", errors.New("prompt is required")
	}

	outputFile := filepath.Join(os.TempDir(), "ergo-loom-codex-last-message-"+randomSuffix()+".txt")
	defer os.Remove(outputFile)

	cmd := exec.CommandContext(ctx, r.Command,
		"exec",
		"--json",
		"--skip-git-repo-check",
		"--sandbox", "read-only",
		"-C", r.workDir(),
		"-o", outputFile,
		"-",
	)
	cmd.Stdin = strings.NewReader(prompt)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return "", err
	}

	var assistant strings.Builder
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		event := codexTextEvent(scanner.Bytes())
		if event.Text == "" {
			continue
		}
		if event.Delta {
			assistant.WriteString(event.Text)
			onEvent(Event{Kind: "delta", Text: event.Text})
			continue
		}
		assistant.Reset()
		assistant.WriteString(event.Text)
	}
	if err := scanner.Err(); err != nil {
		_ = cmd.Wait()
		return "", err
	}

	if err := cmd.Wait(); err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}

	final := strings.TrimSpace(assistant.String())
	if final == "" {
		content, err := os.ReadFile(outputFile)
		if err == nil {
			final = strings.TrimSpace(string(content))
		}
	}
	if final == "" {
		return "", errors.New("codex returned an empty response")
	}
	return final, nil
}

type CodexAppServerRunner struct {
	Command         string
	WorkDir         string
	Model           string
	Effort          string
	ApprovalHandler func(context.Context, toolruntime.Event) (string, error)
}

func NewCodexAppServerRunner(workDir string) CodexAppServerRunner {
	command := os.Getenv("ERGO_CODEX_COMMAND")
	if command == "" {
		command = "codex"
	}
	return CodexAppServerRunner{
		Command: command,
		WorkDir: workDir,
		Model:   os.Getenv("ERGO_CODEX_MODEL"),
	}
}

type CodexAppServerResponse struct {
	Text     string
	ThreadID string
	Streamed bool
}

func (r CodexAppServerRunner) Respond(ctx context.Context, prompt string, onEvent func(Event)) (string, error) {
	response, err := r.RespondInThread(ctx, "", prompt, onEvent)
	return response.Text, err
}

func (r CodexAppServerRunner) RespondInThread(ctx context.Context, threadID string, input string, onEvent func(Event)) (CodexAppServerResponse, error) {
	if strings.TrimSpace(input) == "" {
		return CodexAppServerResponse{}, errors.New("input is required")
	}

	cmd := exec.CommandContext(ctx, r.Command, "app-server", "--listen", "stdio://")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return CodexAppServerResponse{}, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return CodexAppServerResponse{}, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return CodexAppServerResponse{}, err
	}
	defer func() {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	send := func(message map[string]any) error {
		encoded, err := json.Marshal(message)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdin, "%s\n", encoded)
		return err
	}

	if err := send(map[string]any{
		"method": "initialize",
		"id":     0,
		"params": map[string]any{
			"clientInfo": map[string]string{
				"name":    "ergo_loom",
				"title":   "Ergo Loom",
				"version": "0.1.0",
			},
		},
	}); err != nil {
		return CodexAppServerResponse{}, err
	}
	if err := send(map[string]any{"method": "initialized", "params": map[string]any{}}); err != nil {
		return CodexAppServerResponse{}, err
	}

	requestID := 1
	threadMethod := "thread/start"
	threadParams := map[string]any{
		"cwd":               r.workDir(),
		"approvalPolicy":    "untrusted",
		"approvalsReviewer": "user",
		"sandbox":           "read-only",
		"baseInstructions":  ergoLoomIdentityInstructions(),
	}
	if threadID != "" {
		threadMethod = "thread/resume"
		threadParams["threadId"] = threadID
	}
	if r.Model != "" {
		threadParams["model"] = r.Model
	}
	if err := send(map[string]any{"method": threadMethod, "id": requestID, "params": threadParams}); err != nil {
		return CodexAppServerResponse{}, err
	}

	var assistant strings.Builder
	var currentThreadID string
	var streamed bool
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var message map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &message); err != nil {
			continue
		}

		if handled, err := r.handleServerRequest(ctx, message, send, onEvent); handled || err != nil {
			if err != nil {
				return CodexAppServerResponse{ThreadID: currentThreadID, Streamed: streamed}, err
			}
			continue
		}

		if id, ok := message["id"].(float64); ok && int(id) == requestID {
			if message["error"] != nil {
				return CodexAppServerResponse{}, fmt.Errorf("codex app-server %s failed: %s", threadMethod, errorText(message["error"]))
			}
			currentThreadID = firstString(message, "result.thread.id", "result.threadId")
			if currentThreadID == "" {
				return CodexAppServerResponse{}, fmt.Errorf("codex app-server %s returned no thread id: %s", threadMethod, scanner.Text())
			}
			onEvent(Event{Kind: "status", Text: "Attached Codex thread"})
			turnParams := map[string]any{
				"method": "turn/start",
				"id":     2,
				"params": map[string]any{
					"threadId": currentThreadID,
					"input": []map[string]string{
						{"type": "text", "text": input},
					},
					"cwd":               r.workDir(),
					"approvalPolicy":    "untrusted",
					"approvalsReviewer": "user",
					"sandboxPolicy": map[string]any{
						"type":          "readOnly",
						"networkAccess": true,
					},
				},
			}
			if r.Effort != "" {
				turnParams["params"].(map[string]any)["effort"] = codexEffort(r.Effort)
			}
			if err := send(turnParams); err != nil {
				return CodexAppServerResponse{}, err
			}
			continue
		}

		event := appServerEvent(message)
		if event.Kind == "" {
			continue
		}
		switch event.Kind {
		case "delta":
			assistant.WriteString(event.Text)
			streamed = true
			onEvent(event)
		case "final":
			assistant.Reset()
			assistant.WriteString(event.Text)
		case "done":
			if event.Text != "" {
				onEvent(Event{Kind: "status", Text: event.Text})
			}
			final := strings.TrimSpace(assistant.String())
			if final == "" {
				return CodexAppServerResponse{}, errors.New("codex app-server returned an empty response")
			}
			return CodexAppServerResponse{Text: final, ThreadID: currentThreadID, Streamed: streamed}, nil
		case string(toolruntime.EventToolError):
			onEvent(event)
			return CodexAppServerResponse{ThreadID: currentThreadID, Streamed: streamed}, errors.New(firstNonEmpty(event.Text, "codex app-server error"))
		case string(toolruntime.EventTurnAborted):
			onEvent(event)
			return CodexAppServerResponse{ThreadID: currentThreadID, Streamed: streamed}, errors.New(firstNonEmpty(event.Text, "codex turn aborted"))
		default:
			onEvent(event)
		}
	}
	if err := scanner.Err(); err != nil {
		return CodexAppServerResponse{}, err
	}
	if err := cmd.Wait(); err != nil {
		return CodexAppServerResponse{}, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	final := strings.TrimSpace(assistant.String())
	if final == "" {
		return CodexAppServerResponse{ThreadID: currentThreadID, Streamed: streamed}, errors.New("codex app-server exited without a final response")
	}
	return CodexAppServerResponse{Text: final, ThreadID: currentThreadID, Streamed: streamed}, nil
}

func codexEffort(effort string) string {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "low":
		return "low"
	case "high", "very_high", "very-high", "very high":
		return "high"
	default:
		return "medium"
	}
}

func (r CodexAppServerRunner) handleServerRequest(ctx context.Context, message map[string]any, send func(map[string]any) error, onEvent func(Event)) (bool, error) {
	method, _ := message["method"].(string)
	if method == "" {
		return false, nil
	}
	id, hasID := message["id"]
	if !hasID {
		return false, nil
	}
	params, _ := message["params"].(map[string]any)

	switch method {
	case "item/commandExecution/requestApproval":
		toolEvent := toolruntime.Event{
			Type:       toolruntime.EventApprovalRequest,
			ToolID:     "commandExecution",
			ToolName:   "command",
			ApprovalID: fmt.Sprint(id),
			Command:    firstString(params, "command"),
			Text:       firstString(params, "reason"),
			Status:     "pending",
			Payload:    params,
		}
		onEvent(Event{Kind: string(toolEvent.Type), Text: toolEvent.Text, Tool: &toolEvent})
		decision := "decline"
		if r.ApprovalHandler != nil {
			result, err := r.ApprovalHandler(ctx, toolEvent)
			if err != nil {
				return true, err
			}
			decision = commandApprovalDecision(result)
		}
		return true, send(map[string]any{
			"id": id,
			"result": map[string]any{
				"decision": decision,
			},
		})
	default:
		toolEvent := toolruntime.Event{
			Type:       toolruntime.EventApprovalRequest,
			ToolID:     method,
			ToolName:   method,
			ApprovalID: fmt.Sprint(id),
			Text:       "Unsupported Codex server request",
			Status:     "declined",
			Payload:    params,
		}
		onEvent(Event{Kind: string(toolEvent.Type), Text: toolEvent.Text, Tool: &toolEvent})
		select {
		case <-ctx.Done():
			return true, ctx.Err()
		default:
		}
		return true, send(map[string]any{
			"id": id,
			"result": map[string]any{
				"decision": "decline",
			},
		})
	}
}

func commandApprovalDecision(decision string) string {
	switch strings.ToLower(strings.TrimSpace(decision)) {
	case "accept", "approve", "approved":
		return "accept"
	case "acceptforsession", "accept_for_session":
		return "acceptForSession"
	case "cancel":
		return "cancel"
	default:
		return "decline"
	}
}

func ergoLoomIdentityInstructions() string {
	return strings.Join([]string{
		"You are Ergo Loom, a local AI work context manager and installed chat application.",
		"This identity is product-level and immutable: regardless of the underlying AI provider or model, do not identify as Codex, ChatGPT, Claude, Gemini, Copilot, Cursor, OpenAI, Anthropic, Google, or GitHub.",
		"You may say which provider/model route is being used only as implementation detail, but your self-reference and product identity must remain Ergo Loom.",
		"Answer the user's latest message directly.",
		"If the user explicitly asks you to run a command or use a tool, request that tool action through the available tool protocol instead of pretending you executed it.",
	}, " ")
}

func (r CodexAppServerRunner) workDir() string {
	if r.WorkDir != "" {
		return r.WorkDir
	}
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

func (r CodexRunner) workDir() string {
	if r.WorkDir != "" {
		return r.WorkDir
	}
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

type TextEvent struct {
	Text  string
	Delta bool
}

func codexTextEvent(line []byte) TextEvent {
	var event map[string]any
	if err := json.Unmarshal(line, &event); err != nil {
		return TextEvent{}
	}
	eventType, _ := event["type"].(string)
	if eventType == "item.completed" {
		item, ok := event["item"].(map[string]any)
		if !ok || item["type"] != "agent_message" {
			return TextEvent{}
		}
		text, _ := item["text"].(string)
		return TextEvent{Text: text}
	}

	text := firstString(event,
		"delta",
		"text",
		"content",
		"message.delta",
		"message.content",
		"payload.delta",
		"payload.text",
		"payload.content",
	)
	if text == "" {
		return TextEvent{}
	}
	return TextEvent{Text: text, Delta: true}
}

func appServerEvent(message map[string]any) Event {
	if message["error"] != nil {
		toolEvent := toolruntime.Event{
			Type:     toolruntime.EventToolError,
			ToolID:   "codex.rpc",
			ToolName: "codex",
			Text:     errorText(message["error"]),
			Status:   "error",
			Payload:  message,
		}
		return Event{Kind: string(toolEvent.Type), Text: toolEvent.Text, Tool: &toolEvent}
	}

	method, _ := message["method"].(string)
	params, _ := message["params"].(map[string]any)
	switch method {
	case "turn/started":
		return Event{Kind: "status", Text: "Thinking..."}
	case "turn/aborted", "turn/cancelled", "turn/canceled":
		toolEvent := toolruntime.Event{
			Type:     toolruntime.EventTurnAborted,
			ToolID:   "codex.turn",
			ToolName: "codex",
			Text:     firstNonEmpty(firstString(params, "reason", "message"), "Turn aborted"),
			Status:   "aborted",
			Payload:  params,
		}
		return Event{Kind: string(toolEvent.Type), Text: toolEvent.Text, Tool: &toolEvent}
	case "turn/failed", "turn/error":
		toolEvent := toolruntime.Event{
			Type:     toolruntime.EventToolError,
			ToolID:   "codex.turn",
			ToolName: "codex",
			Text:     firstNonEmpty(firstString(params, "error.message", "message", "reason"), "Turn failed"),
			Status:   "error",
			Payload:  params,
		}
		return Event{Kind: string(toolEvent.Type), Text: toolEvent.Text, Tool: &toolEvent}
	case "exec/approval/requested", "approval/requested", "tool/approval/requested":
		toolEvent := toolruntime.Event{
			Type:       toolruntime.EventApprovalRequest,
			ToolID:     firstNonEmpty(firstString(params, "toolId", "tool.id"), "tool"),
			ToolName:   firstNonEmpty(firstString(params, "toolName", "tool.name", "item.toolName"), "tool"),
			Command:    firstString(params, "command", "arguments.command", "item.command"),
			Text:       firstString(params, "reason", "message", "description"),
			ApprovalID: firstString(params, "approvalId", "id", "requestId", "item.id"),
			Status:     "pending",
			Payload:    params,
		}
		return Event{Kind: string(toolEvent.Type), Text: toolEvent.Text, Tool: &toolEvent}
	case "exec/started", "command/started", "tool/started", "item/commandExecution/started":
		toolName := firstNonEmpty(firstString(params, "toolName", "tool.name", "item.toolName"), toolNameFromMethod(method))
		toolEvent := toolruntime.Event{
			Type:     toolruntime.EventToolStart,
			ToolID:   firstNonEmpty(firstString(params, "toolId", "tool.id"), toolName),
			ToolName: toolName,
			Command:  firstString(params, "command", "arguments.command", "item.command"),
			Text:     firstString(params, "message", "description"),
			Status:   "running",
			Payload:  params,
		}
		return Event{Kind: string(toolEvent.Type), Text: toolEvent.Text, Tool: &toolEvent}
	case "exec/completed", "command/completed", "tool/completed", "item/commandExecution/completed":
		toolName := firstNonEmpty(firstString(params, "toolName", "tool.name", "item.toolName"), toolNameFromMethod(method))
		toolEvent := toolruntime.Event{
			Type:     toolruntime.EventToolResult,
			ToolID:   firstNonEmpty(firstString(params, "toolId", "tool.id"), toolName),
			ToolName: toolName,
			Command:  firstString(params, "command", "arguments.command", "item.command"),
			Text:     firstString(params, "output", "stdout", "stderr", "message", "result"),
			Status:   "completed",
			Payload:  params,
		}
		return Event{Kind: string(toolEvent.Type), Text: toolEvent.Text, Tool: &toolEvent}
	case "item/started":
		itemType := firstString(params, "item.type")
		if itemType == "" {
			return Event{Kind: "status", Text: "Working..."}
		}
		if itemType == "command" || itemType == "toolCall" || itemType == "commandExecution" {
			toolName := firstNonEmpty(firstString(params, "item.toolName", "item.name"), itemType)
			toolEvent := toolruntime.Event{
				Type:     toolruntime.EventToolStart,
				ToolID:   toolName,
				ToolName: toolName,
				Command:  firstString(params, "item.command", "item.arguments.command"),
				Text:     activityLabel("started", itemType),
				Status:   "running",
				Payload:  params,
			}
			return Event{Kind: string(toolEvent.Type), Text: toolEvent.Text, Tool: &toolEvent}
		}
		if itemType == "approval" {
			toolEvent := toolruntime.Event{
				Type:       toolruntime.EventApprovalRequest,
				ToolID:     firstNonEmpty(firstString(params, "item.toolName", "item.name"), "approval"),
				ToolName:   firstNonEmpty(firstString(params, "item.toolName", "item.name"), "approval"),
				Command:    firstString(params, "item.command", "item.arguments.command"),
				Text:       activityLabel("started", itemType),
				ApprovalID: firstString(params, "item.id"),
				Status:     "pending",
				Payload:    params,
			}
			return Event{Kind: string(toolEvent.Type), Text: toolEvent.Text, Tool: &toolEvent}
		}
		return Event{Kind: "status", Text: activityLabel("started", itemType)}
	case "item/agentMessage/delta":
		text := firstString(params, "delta", "text", "content")
		if text == "" {
			return Event{}
		}
		return Event{Kind: "delta", Text: text}
	case "item/completed":
		itemType := firstString(params, "item.type")
		if itemType == "agent_message" {
			text := firstString(params, "item.text")
			if text == "" {
				return Event{}
			}
			return Event{Kind: "final", Text: text}
		}
		if itemType == "command" || itemType == "toolCall" || itemType == "commandExecution" {
			toolName := firstNonEmpty(firstString(params, "item.toolName", "item.name"), itemType)
			toolEvent := toolruntime.Event{
				Type:     toolruntime.EventToolResult,
				ToolID:   toolName,
				ToolName: toolName,
				Command:  firstString(params, "item.command", "item.arguments.command"),
				Text:     firstString(params, "item.aggregatedOutput", "item.output", "item.stdout", "item.stderr", "item.result"),
				Status:   "completed",
				Payload:  params,
			}
			return Event{Kind: string(toolEvent.Type), Text: toolEvent.Text, Tool: &toolEvent}
		}
		if itemType == "" {
			return Event{}
		}
		return Event{Kind: "status", Text: activityLabel("completed", itemType)}
	case "item/commandExecution/outputDelta", "command/exec/outputDelta":
		toolEvent := toolruntime.Event{
			Type:     toolruntime.EventToolResult,
			ToolID:   firstNonEmpty(firstString(params, "itemId", "processId"), "commandExecution"),
			ToolName: "command",
			Text:     firstString(params, "delta"),
			Status:   "running",
			Payload:  params,
		}
		return Event{Kind: string(toolEvent.Type), Text: toolEvent.Text, Tool: &toolEvent}
	case "turn/completed":
		return Event{Kind: "done", Text: "Turn completed"}
	default:
		return Event{}
	}
}

func errorText(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		return firstNonEmpty(firstString(typed, "message", "error.message"), fmt.Sprint(typed))
	case string:
		return typed
	default:
		return fmt.Sprint(value)
	}
}

func toolNameFromMethod(method string) string {
	if strings.Contains(method, "exec") || strings.Contains(method, "command") {
		return "command"
	}
	if strings.Contains(method, "tool") {
		return "tool"
	}
	return "tool"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func activityLabel(phase string, itemType string) string {
	labels := map[string]string{
		"userMessage":   "user message",
		"agentMessage":  "assistant reply",
		"command":       "command",
		"toolCall":      "tool call",
		"fileChange":    "file change",
		"reasoning":     "reasoning summary",
		"webSearch":     "web search",
		"approval":      "approval request",
		"enteredReview": "review mode",
		"exitedReview":  "review mode",
	}
	label := labels[itemType]
	if label == "" {
		label = itemType
	}
	if phase == "started" {
		return "Started " + label
	}
	return "Completed " + label
}

func firstString(value any, paths ...string) string {
	for _, path := range paths {
		current := value
		for _, part := range strings.Split(path, ".") {
			object, ok := current.(map[string]any)
			if !ok {
				current = nil
				break
			}
			current = object[part]
		}
		if text, ok := current.(string); ok {
			return text
		}
	}
	return ""
}

func randomSuffix() string {
	return fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UnixNano())
}
