package packetpolicy

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Floodnut/ergo-loom/internal/core"
)

const defaultBudget = 12000

// Registry maps policy names to ContextPacketPolicy implementations.
type Registry struct {
	policies map[string]core.ContextPacketPolicy
}

func NewRegistry() Registry {
	r := Registry{policies: make(map[string]core.ContextPacketPolicy)}
	r.Register(FlatTrimPolicy{})
	r.Register(SegmentChainPolicy{})
	return r
}

func (r Registry) Register(p core.ContextPacketPolicy) {
	r.policies[p.Name()] = p
}

func (r Registry) Get(name string) (core.ContextPacketPolicy, bool) {
	p, ok := r.policies[name]
	return p, ok
}

func (r Registry) List() []string {
	names := make([]string, 0, len(r.policies))
	for name := range r.policies {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (r Registry) GetOrDefault(name string) core.ContextPacketPolicy {
	if p, ok := r.policies[name]; ok {
		return p
	}
	return r.policies["flat-trim"]
}

// FlatTrimPolicy is the baseline policy: recent messages, fixed 12000-char budget.
// Provider budget is ignored; behavior matches the original buildContextPacket.
type FlatTrimPolicy struct{}

func (FlatTrimPolicy) Name() string { return "flat-trim" }

func (FlatTrimPolicy) Build(ctx core.PacketBuildContext) core.ContextPacket {
	packet := newPacket(ctx)
	latest := strings.TrimSpace(ctx.UserInput)
	contextLines := messageLines(ctx.Ancestors, ctx.Messages, latest)

	assembled := systemLines(ctx)
	assembled = append(assembled, "", "Conversation context:")
	assembled = append(assembled, trimLines(contextLines, defaultBudget/2)...)
	assembled = append(assembled, "", "Latest user message:", ctx.UserInput)
	packet.Content = trimPacket(strings.Join(assembled, "\n"), defaultBudget)
	return packet
}

// SegmentChainPolicy uses the provider-specific context budget and is designed
// to incorporate segment-level handoff summaries as they become available.
// Without summaries it behaves like FlatTrimPolicy but respects ContextBudget.
type SegmentChainPolicy struct{}

func (SegmentChainPolicy) Name() string { return "segment-chain" }

func (SegmentChainPolicy) Build(ctx core.PacketBuildContext) core.ContextPacket {
	budget := ctx.ContextBudget
	if budget <= 0 {
		budget = defaultBudget
	}
	packet := newPacket(ctx)
	latest := strings.TrimSpace(ctx.UserInput)

	// Section: summaries from summary.created events.
	var summaryLines []string
	coveredMessageIDs := map[string]bool{}
	if ctx.LoadSummary != nil {
		for _, event := range ctx.Ancestors {
			if event.Type != core.EventSummaryCreated {
				continue
			}
			summaryID := strings.TrimPrefix(event.PayloadRef, "summary:")
			if summaryID == "" {
				continue
			}
			payload, err := ctx.LoadSummary(summaryID)
			if err != nil {
				continue
			}
			if strings.TrimSpace(payload.Text) != "" {
				summaryLines = append(summaryLines, payload.Text)
			}
			for _, msgID := range payload.CoveredMessageIDs {
				coveredMessageIDs[msgID] = true
			}
		}
	}

	// Section: messages — skip messages already covered by a summary.
	contextLines := messageLinesFilteredByMsgID(ctx.Ancestors, ctx.Messages, latest, coveredMessageIDs)

	assembled := systemLines(ctx)
	if len(summaryLines) > 0 {
		assembled = append(assembled, "", "Previous provider context:")
		assembled = append(assembled, summaryLines...)
	}
	assembled = append(assembled, "", "Conversation context:")
	assembled = append(assembled, trimLines(contextLines, budget/2)...)
	assembled = append(assembled, "", "Latest user message:", ctx.UserInput)
	packet.Content = trimPacket(strings.Join(assembled, "\n"), budget)
	return packet
}

// --- shared helpers ---

func newPacket(ctx core.PacketBuildContext) core.ContextPacket {
	return core.ContextPacket{
		ID:          fmt.Sprintf("context_packet_%d", time.Now().UTC().UnixNano()),
		ProjectID:   ctx.Session.ProjectID,
		SessionID:   ctx.Session.ID,
		BranchID:    "main",
		HeadEventID: ctx.HeadEventID,
		UserInput:   ctx.UserInput,
		CreatedAt:   time.Now().UTC(),
	}
}

func systemLines(ctx core.PacketBuildContext) []string {
	lines := []string{
		"You are Ergo Loom, a local AI work context manager.",
		"Use Ergo Loom's local context as the authoritative conversation state.",
		"Provider-owned CLI, app, browser, or remote sessions are execution channels and may be stale or unavailable.",
	}
	if ctx.RouteLabel != "" {
		lines = append(lines, "Selected route: "+ctx.RouteLabel+".")
	}
	if strings.TrimSpace(ctx.Note) != "" {
		lines = append(lines, "Context note: "+strings.TrimSpace(ctx.Note))
	}
	return lines
}

func messageLines(ancestors []core.Event, messages []core.Message, skipContent string) []string {
	return messageLinesFiltered(ancestors, messages, skipContent, nil)
}

func messageLinesFilteredByMsgID(ancestors []core.Event, messages []core.Message, skipContent string, skipMsgIDs map[string]bool) []string {
	msgByID := make(map[string]core.Message, len(messages))
	for _, m := range messages {
		msgByID[m.ID] = m
	}
	lines := make([]string, 0, len(ancestors))
	for _, event := range ancestors {
		if event.Type != core.EventMessageUser && event.Type != core.EventMessageAssistant {
			continue
		}
		messageID := strings.TrimPrefix(event.PayloadRef, "message:")
		if skipMsgIDs[messageID] {
			continue
		}
		msg, ok := msgByID[messageID]
		if !ok {
			continue
		}
		content := strings.TrimSpace(msg.Content)
		if content == "" || (msg.Role == "user" && content == skipContent) {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s: %s", msg.Role, content))
	}
	return lines
}

func messageLinesFiltered(ancestors []core.Event, messages []core.Message, skipContent string, skipEventIDs map[string]bool) []string {
	msgByID := make(map[string]core.Message, len(messages))
	for _, m := range messages {
		msgByID[m.ID] = m
	}
	lines := make([]string, 0, len(ancestors))
	for _, event := range ancestors {
		if skipEventIDs[event.ID] {
			continue
		}
		if event.Type != core.EventMessageUser && event.Type != core.EventMessageAssistant {
			continue
		}
		messageID := strings.TrimPrefix(event.PayloadRef, "message:")
		msg, ok := msgByID[messageID]
		if !ok {
			continue
		}
		content := strings.TrimSpace(msg.Content)
		if content == "" || (msg.Role == "user" && content == skipContent) {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s: %s", msg.Role, content))
	}
	return lines
}

func trimLines(lines []string, maxChars int) []string {
	if maxChars <= 0 {
		return nil
	}
	selected := []string{}
	used := 0
	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]
		lineLen := len([]rune(line)) + 1
		if used+lineLen > maxChars && len(selected) > 0 {
			break
		}
		selected = append([]string{line}, selected...)
		used += lineLen
	}
	return selected
}

func trimPacket(content string, maxChars int) string {
	runes := []rune(content)
	if len(runes) <= maxChars {
		return content
	}
	return string(runes[len(runes)-maxChars:])
}
