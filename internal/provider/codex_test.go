package provider

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Floodnut/ergo-loom/internal/toolruntime"
)

func TestCodexRunnerRespondStreamsJSONDeltas(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test")
	}

	dir := t.TempDir()
	command := filepath.Join(dir, "fake-codex")
	if err := os.WriteFile(command, []byte(`#!/bin/sh
printf '{"delta":"hello "}\n'
printf '{"payload":{"text":"world"}}\n'
`), 0o755); err != nil {
		t.Fatal(err)
	}

	var streamed strings.Builder
	runner := CodexRunner{
		Command: command,
		WorkDir: dir,
	}
	response, err := runner.Respond(context.Background(), "Say hello", func(event Event) {
		if event.Kind == EventKindDelta {
			streamed.WriteString(event.Text)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if response != "hello world" {
		t.Fatalf("response = %q, want %q", response, "hello world")
	}
	if streamed.String() != "hello world" {
		t.Fatalf("streamed = %q, want %q", streamed.String(), "hello world")
	}
}

func TestCodexRunnerRespondReadsCompletedAgentMessage(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test")
	}

	dir := t.TempDir()
	command := filepath.Join(dir, "fake-codex")
	if err := os.WriteFile(command, []byte(`#!/bin/sh
printf '{"type":"thread.started","thread_id":"test"}\n'
printf '{"type":"item.completed","item":{"id":"item_0","type":"agent_message","text":"final answer"}}\n'
`), 0o755); err != nil {
		t.Fatal(err)
	}

	var streamed strings.Builder
	runner := CodexRunner{
		Command: command,
		WorkDir: dir,
	}
	response, err := runner.Respond(context.Background(), "Say hello", func(event Event) {
		if event.Kind == EventKindDelta {
			streamed.WriteString(event.Text)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if response != "final answer" {
		t.Fatalf("response = %q, want %q", response, "final answer")
	}
	if streamed.String() != "" {
		t.Fatalf("streamed = %q, want no delta streaming", streamed.String())
	}
}

func TestAppServerEventParsesAgentMessageDelta(t *testing.T) {
	event := appServerEvent(map[string]any{
		"method": "item/agentMessage/delta",
		"params": map[string]any{
			"delta": "hello",
		},
	})
	if event.Kind != EventKindDelta || event.Text != "hello" {
		t.Fatalf("event = %#v, want delta hello", event)
	}
}

func TestAppServerEventDoesNotExposeReasoningText(t *testing.T) {
	event := appServerEvent(map[string]any{
		"method": "item/started",
		"params": map[string]any{
			"item": map[string]any{"type": "reasoning"},
		},
	})
	if event.Kind != EventKindStatus || event.Text != "Started reasoning summary" {
		t.Fatalf("event = %#v, want reasoning summary status", event)
	}
}

func TestAppServerEventParsesCommandToolLifecycle(t *testing.T) {
	started := appServerEvent(map[string]any{
		"method": "item/started",
		"params": map[string]any{
			"item": map[string]any{
				"type":    "command",
				"command": "curl www.example.com",
			},
		},
	})
	if started.Kind != EventKindToolStart || started.Tool == nil || started.Tool.ToolName != "command" || started.Tool.Command != "curl www.example.com" {
		t.Fatalf("started = %#v, want command tool_start", started)
	}

	completed := appServerEvent(map[string]any{
		"method": "item/completed",
		"params": map[string]any{
			"item": map[string]any{
				"type":   "command",
				"stdout": "<html>example</html>",
			},
		},
	})
	if completed.Kind != EventKindToolResult || completed.Tool == nil || completed.Tool.Text != "<html>example</html>" {
		t.Fatalf("completed = %#v, want command tool_result", completed)
	}
}

func TestAppServerEventParsesApprovalRequest(t *testing.T) {
	event := appServerEvent(map[string]any{
		"method": "exec/approval/requested",
		"params": map[string]any{
			"id":      "approval_1",
			"command": "curl www.example.com",
			"reason":  "Network command requested",
		},
	})
	if event.Kind != EventKindApprovalRequest || event.Tool == nil || event.Tool.ApprovalID != "approval_1" || event.Tool.Command != "curl www.example.com" {
		t.Fatalf("event = %#v, want approval_request", event)
	}
}

func TestCodexAppServerRunnerHandlesCommandApprovalRequest(t *testing.T) {
	runner := CodexAppServerRunner{
		ApprovalHandler: func(_ context.Context, event toolruntime.Event) (string, error) {
			if event.Command != "curl www.example.com" {
				t.Fatalf("event.Command = %q", event.Command)
			}
			return "accept", nil
		},
	}
	var sent map[string]any
	var emitted Event
	handled, err := runner.handleServerRequest(context.Background(), map[string]any{
		"id":     float64(7),
		"method": "item/commandExecution/requestApproval",
		"params": map[string]any{
			"command": "curl www.example.com",
			"reason":  "Network command requested",
		},
	}, func(message map[string]any) error {
		sent = message
		return nil
	}, func(event Event) {
		emitted = event
	})
	if err != nil {
		t.Fatal(err)
	}
	if !handled {
		t.Fatal("handled = false, want true")
	}
	result, _ := sent["result"].(map[string]any)
	if sent["id"] != float64(7) || result["decision"] != "accept" {
		t.Fatalf("sent = %#v, want accept response", sent)
	}
	if emitted.Kind != EventKindApprovalRequest || emitted.Tool == nil || emitted.Tool.Command != "curl www.example.com" || emitted.Tool.Status != "pending" {
		t.Fatalf("emitted = %#v, want pending approval event", emitted)
	}
}

func TestAppServerEventParsesTurnAbortAndError(t *testing.T) {
	aborted := appServerEvent(map[string]any{
		"method": "turn/aborted",
		"params": map[string]any{
			"reason": "stopped after 0s",
		},
	})
	if aborted.Kind != EventKindTurnAborted || aborted.Tool == nil || aborted.Tool.Text != "stopped after 0s" {
		t.Fatalf("aborted = %#v, want aborted event", aborted)
	}

	failed := appServerEvent(map[string]any{
		"error": map[string]any{
			"message": "network approval denied",
		},
	})
	if failed.Kind != EventKindToolError || failed.Tool == nil || failed.Tool.Text != "network approval denied" {
		t.Fatalf("failed = %#v, want provider error", failed)
	}
}
