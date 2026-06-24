package core

type SegmentEndReason string

const (
	ReasonCompleted   SegmentEndReason = "completed"
	ReasonError       SegmentEndReason = "error"
	ReasonTimeout     SegmentEndReason = "timeout"
	ReasonAuthFailure SegmentEndReason = "auth_failure"
	ReasonBudgetLimit SegmentEndReason = "budget_limit"
	ReasonSessionEnd  SegmentEndReason = "session_end"
	ReasonUserAbort   SegmentEndReason = "user_abort"
)

type ModerationAction string

const (
	ActionContinue  ModerationAction = "continue"
	ActionFailover  ModerationAction = "failover"
	ActionSuspend   ModerationAction = "suspend"
	ActionTerminate ModerationAction = "terminate"
)

type ModerationContext struct {
	Session       Session
	ActiveSegment ProviderSegment
	Reason        SegmentEndReason
	QueueDepth    int
}

type ModerationDecision struct {
	Action ModerationAction
}

// Moderator reacts to provider segment and budget events. It does not select
// routes, build context packets, approve tools, or choose candidate outputs.
type Moderator interface {
	OnSegmentEnd(ctx ModerationContext) ModerationDecision
	OnBudgetWarning(ctx ModerationContext) ModerationDecision
}

type DefaultModerator struct{}

func (DefaultModerator) OnSegmentEnd(ctx ModerationContext) ModerationDecision {
	switch ctx.Reason {
	case ReasonCompleted, ReasonUserAbort:
		return ModerationDecision{Action: ActionTerminate}
	case ReasonError, ReasonTimeout, ReasonAuthFailure, ReasonBudgetLimit, ReasonSessionEnd:
		return ModerationDecision{Action: ActionFailover}
	default:
		return ModerationDecision{Action: ActionSuspend}
	}
}

func (DefaultModerator) OnBudgetWarning(ctx ModerationContext) ModerationDecision {
	return ModerationDecision{Action: ActionContinue}
}
