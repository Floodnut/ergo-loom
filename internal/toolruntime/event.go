package toolruntime

import "context"

type EventType string

const (
	EventToolStart       EventType = "tool_start"
	EventToolResult      EventType = "tool_result"
	EventApprovalRequest EventType = "approval_request"
	EventToolError       EventType = "tool_error"
	EventTurnAborted     EventType = "turn_aborted"
)

type Event struct {
	Type         EventType      `json:"type"`
	ToolID       string         `json:"toolId"`
	ToolName     string         `json:"toolName"`
	InvocationID string         `json:"invocationId"`
	ApprovalID   string         `json:"approvalId"`
	Command      string         `json:"command"`
	Text         string         `json:"text"`
	Status       string         `json:"status"`
	Payload      map[string]any `json:"raw,omitempty"`
}

type Request struct {
	ToolID       string         `json:"toolId"`
	InvocationID string         `json:"invocationId"`
	Command      string         `json:"command"`
	Args         map[string]any `json:"args,omitempty"`
	RequiresAuth bool           `json:"requiresAuth"`
}

type Result struct {
	InvocationID string `json:"invocationId"`
	Status       string `json:"status"`
	Stdout       string `json:"stdout"`
	Stderr       string `json:"stderr"`
	ExitCode     int    `json:"exitCode"`
}

type Executor interface {
	Execute(ctx context.Context, request Request, emit func(Event)) (Result, error)
}
