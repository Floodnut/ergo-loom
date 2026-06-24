package handoffpolicy

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Floodnut/ergo-loom/internal/core"
)

// Registry maps policy names to HandoffPolicy implementations.
type Registry struct {
	policies map[string]core.HandoffPolicy
}

func NewRegistry() Registry {
	r := Registry{policies: make(map[string]core.HandoffPolicy)}
	r.Register(RouteChangePolicy{})
	r.Register(AIHandoffPolicy{})
	r.Register(NoOpPolicy{})
	return r
}

func (r Registry) Register(p core.HandoffPolicy) {
	r.policies[p.Name()] = p
}

func (r Registry) List() []string {
	names := make([]string, 0, len(r.policies))
	for name := range r.policies {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (r Registry) GetOrDefault(name string) core.HandoffPolicy {
	if p, ok := r.policies[name]; ok {
		return p
	}
	return r.policies["route-change"]
}

// RouteChangePolicy detects a switch when the incoming route differs from the
// last completed segment. Summary is rule-based (no AI call required).
type RouteChangePolicy struct{}

func (RouteChangePolicy) Name() string { return "route-change" }

func (RouteChangePolicy) DetectSwitch(last core.ProviderSegment, incoming core.HandoffCandidate) bool {
	return last.RouteID != "" && last.RouteID != incoming.RouteID
}

func (RouteChangePolicy) Summarize(ctx core.HandoffContext) (core.SummaryPayload, error) {
	var coveredIDs []string
	var userRequests []string

	for _, msg := range ctx.Messages {
		coveredIDs = append(coveredIDs, msg.ID)
		if msg.Role == "user" {
			line := strings.TrimSpace(msg.Content)
			if len([]rune(line)) > 120 {
				line = string([]rune(line)[:120]) + "…"
			}
			userRequests = append(userRequests, "- "+line)
		}
	}

	providerLabel := ctx.Segment.ProviderID
	if ctx.Segment.RouteID != "" {
		providerLabel = ctx.Segment.RouteID
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Previous provider (%s) completed %d message(s).", providerLabel, len(ctx.Messages))
	if len(userRequests) > 0 {
		sb.WriteString("\n\nRequests handled:\n")
		sb.WriteString(strings.Join(userRequests, "\n"))
	}

	return core.SummaryPayload{
		ProviderSegmentID: ctx.Segment.ID,
		CoveredMessageIDs: coveredIDs,
		Text:              sb.String(),
	}, nil
}

// AIHandoffPolicy detects a route change and generates a structured summary
// using the previous provider via CallProvider. Falls back to rule-based
// summary if CallProvider is nil or returns an error.
type AIHandoffPolicy struct{}

func (AIHandoffPolicy) Name() string { return "ai-summarize" }

func (AIHandoffPolicy) DetectSwitch(last core.ProviderSegment, incoming core.HandoffCandidate) bool {
	return last.RouteID != "" && last.RouteID != incoming.RouteID
}

func (AIHandoffPolicy) Summarize(ctx core.HandoffContext) (core.SummaryPayload, error) {
	coveredIDs := make([]string, 0, len(ctx.Messages))
	for _, m := range ctx.Messages {
		coveredIDs = append(coveredIDs, m.ID)
	}

	if ctx.CallProvider != nil {
		prompt := buildSummaryPrompt(ctx.Messages, ctx.Segment.RouteID)
		if text, err := ctx.CallProvider(prompt); err == nil && strings.TrimSpace(text) != "" {
			return core.SummaryPayload{
				ProviderSegmentID: ctx.Segment.ID,
				CoveredMessageIDs: coveredIDs,
				Text:              strings.TrimSpace(text),
			}, nil
		}
	}

	// Fallback: rule-based
	return (RouteChangePolicy{}).Summarize(ctx)
}

func buildSummaryPrompt(messages []core.Message, routeLabel string) string {
	var sb strings.Builder
	sb.WriteString("Summarize the following conversation segment from provider ")
	sb.WriteString(routeLabel)
	sb.WriteString(".\n\n")
	sb.WriteString("Output three short sections:\n")
	sb.WriteString("DONE: what was completed\n")
	sb.WriteString("STATE: current working state (files, tech, context)\n")
	sb.WriteString("NEXT: what remains or was left open\n\n")
	sb.WriteString("Conversation:\n")
	for _, m := range messages {
		content := strings.TrimSpace(m.Content)
		if len([]rune(content)) > 300 {
			content = string([]rune(content)[:300]) + "…"
		}
		fmt.Fprintf(&sb, "%s: %s\n", m.Role, content)
	}
	sb.WriteString("\nSummary:")
	return sb.String()
}

// NoOpPolicy never detects a switch. Use when handoff summarization is disabled.
type NoOpPolicy struct{}

func (NoOpPolicy) Name() string { return "noop" }

func (NoOpPolicy) DetectSwitch(_ core.ProviderSegment, _ core.HandoffCandidate) bool {
	return false
}

func (NoOpPolicy) Summarize(_ core.HandoffContext) (core.SummaryPayload, error) {
	return core.SummaryPayload{}, nil
}
