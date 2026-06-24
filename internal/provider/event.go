package provider

import "github.com/Floodnut/ergo-loom/internal/toolruntime"

type EventKind string

const (
	EventKindDelta  EventKind = "delta"
	EventKindStatus EventKind = "status"

	EventKindToolStart       EventKind = "tool_start"
	EventKindToolResult      EventKind = "tool_result"
	EventKindToolError       EventKind = "tool_error"
	EventKindApprovalRequest EventKind = "approval_request"
	EventKindTurnAborted     EventKind = "turn_aborted"

	EventKindFinal EventKind = "final"

	eventKindDone EventKind = "done"
)

type Event struct {
	Kind EventKind
	Text string
	Tool *toolruntime.Event
}
