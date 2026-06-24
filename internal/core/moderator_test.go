package core

import "testing"

func TestDefaultModeratorDecisions(t *testing.T) {
	moderator := DefaultModerator{}

	cases := []struct {
		name   string
		reason SegmentEndReason
		want   ModerationAction
	}{
		{name: "completed", reason: ReasonCompleted, want: ActionTerminate},
		{name: "user abort", reason: ReasonUserAbort, want: ActionTerminate},
		{name: "error", reason: ReasonError, want: ActionFailover},
		{name: "timeout", reason: ReasonTimeout, want: ActionFailover},
		{name: "auth failure", reason: ReasonAuthFailure, want: ActionFailover},
		{name: "budget limit", reason: ReasonBudgetLimit, want: ActionFailover},
		{name: "session end", reason: ReasonSessionEnd, want: ActionFailover},
		{name: "unknown", reason: SegmentEndReason("unknown"), want: ActionSuspend},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := moderator.OnSegmentEnd(ModerationContext{Reason: tc.reason})
			if got.Action != tc.want {
				t.Fatalf("expected %s, got %s", tc.want, got.Action)
			}
		})
	}
}

func TestDefaultModeratorBudgetWarningContinues(t *testing.T) {
	got := (DefaultModerator{}).OnBudgetWarning(ModerationContext{})
	if got.Action != ActionContinue {
		t.Fatalf("expected continue, got %s", got.Action)
	}
}
