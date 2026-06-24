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

	eventKindFinal EventKind = "final"
	eventKindDone  EventKind = "done"
)

type Event struct {
	Kind EventKind
	Text string
	Tool *toolruntime.Event
}

func eventKindForToolEventType(eventType toolruntime.EventType) EventKind {
	switch eventType {
	case toolruntime.EventToolStart:
		return EventKindToolStart
	case toolruntime.EventToolResult:
		return EventKindToolResult
	case toolruntime.EventToolError:
		return EventKindToolError
	case toolruntime.EventApprovalRequest:
		return EventKindApprovalRequest
	case toolruntime.EventTurnAborted:
		return EventKindTurnAborted
	default:
		return ""
	}
}

func eventFromToolRuntime(toolEvent toolruntime.Event) Event {
	return Event{
		Kind: eventKindForToolEventType(toolEvent.Type),
		Text: toolEvent.Text,
		Tool: &toolEvent,
	}
}
