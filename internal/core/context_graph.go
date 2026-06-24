package core

import "time"

type EventType string

const (
	EventMessageUser          EventType = "message.user"
	EventMessageAssistant     EventType = "message.assistant"
	EventProviderRunStarted   EventType = "provider.run.started"
	EventProviderRunCompleted EventType = "provider.run.completed"
	EventToolRequested        EventType = "tool.requested"
	EventToolApproved         EventType = "tool.approved"
	EventToolRejected         EventType = "tool.rejected"
	EventToolCompleted        EventType = "tool.completed"
	EventToolFailed           EventType = "tool.failed"
	EventTurnAborted          EventType = "turn.aborted"
	EventFileReferenced       EventType = "file.referenced"
	EventSummaryCreated       EventType = "summary.created"
	EventBranchCreated        EventType = "branch.created"
	EventMergeCreated         EventType = "merge.created"
	EventKnowledgePromoted    EventType = "knowledge.promoted"
	EventModeratorHandoff     EventType = "moderator.handoff"
	EventQueueItemCreated     EventType = "queue.item.created"
	EventQueueItemReordered   EventType = "queue.item.reordered"
	EventSteeringAdded        EventType = "steering.added"
	EventParallelRunQueued    EventType = "parallel.run.queued"
)

type Event struct {
	ID             string
	Type           EventType
	ProjectID      string
	SessionID      string
	BranchID       string
	ParentEventIDs []string
	PayloadRef     string
	CreatedAt      time.Time
}

type Head struct {
	ID        string
	ProjectID string
	SessionID string
	BranchID  string
	EventID   string
	UpdatedAt time.Time
}

type GraphBranch struct {
	ID          string
	ProjectID   string
	SessionID   string
	FromEventID string
	HeadEventID string
	CreatedAt   time.Time
}

type ChatRunRole string

const (
	ChatRunRoleMain     ChatRunRole = "main"
	ChatRunRoleParallel ChatRunRole = "parallel"
)

type ChatRunStatus string

const (
	ChatRunQueued          ChatRunStatus = "queued"
	ChatRunRunning         ChatRunStatus = "running"
	ChatRunWaitingApproval ChatRunStatus = "waiting_approval"
	ChatRunCompleted       ChatRunStatus = "completed"
	ChatRunFailed          ChatRunStatus = "failed"
	ChatRunCancelled       ChatRunStatus = "cancelled"
)

type ChatRun struct {
	ID              string
	ProjectID       string
	SessionID       string
	BranchID        string
	Role            ChatRunRole
	Status          ChatRunStatus
	InputEventID    string
	OutputEventID   string
	ContextPacketID string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type ProviderSegmentStatus string

const (
	ProviderSegmentRunning   ProviderSegmentStatus = "running"
	ProviderSegmentCompleted ProviderSegmentStatus = "completed"
	ProviderSegmentFailed    ProviderSegmentStatus = "failed"
	ProviderSegmentCancelled ProviderSegmentStatus = "cancelled"
)

type ProviderSegment struct {
	ID               string
	ChatRunID        string
	ProviderID       string
	RouteID          string
	ModelID          string
	ExternalThreadID string
	Status           ProviderSegmentStatus
	HandoffReason    string
	StartedAt        time.Time
	CompletedAt      *time.Time
}

type QueueItemStatus string

const (
	QueueItemPending  QueueItemStatus = "pending"
	QueueItemConsumed QueueItemStatus = "consumed"
	QueueItemRemoved  QueueItemStatus = "removed"
)

type QueueItemMode string

const (
	QueueItemNormal   QueueItemMode = "normal"
	QueueItemSteering QueueItemMode = "steering"
	QueueItemParallel QueueItemMode = "parallel"
)

type QueueItem struct {
	ID             string
	SessionID      string
	BranchID       string
	Content        string
	Mode           QueueItemMode
	Status         QueueItemStatus
	OrderIndex     int
	RouteID        string
	ModelID        string
	ThinkingEffort string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type CandidateOutputStatus string

const (
	CandidateOutputPending  CandidateOutputStatus = "pending"
	CandidateOutputReady    CandidateOutputStatus = "ready"
	CandidateOutputAccepted CandidateOutputStatus = "accepted"
	CandidateOutputRejected CandidateOutputStatus = "rejected"
)

type CandidateOutput struct {
	ID         string
	ChatRunID  string
	SessionID  string
	BranchID   string
	ContentRef string
	Status     CandidateOutputStatus
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type ProviderRun struct {
	ID                string
	EventID           string
	ProviderID        string
	RouteID           string
	ModelID           string
	ExternalThreadID  string
	LastSyncedEventID string
	Status            string
	StartedAt         time.Time
	CompletedAt       *time.Time
}

type ContextReference struct {
	Kind string
	ID   string
	Ref  string
}

type ContextPacket struct {
	ID          string
	ProjectID   string
	SessionID   string
	BranchID    string
	HeadEventID string
	UserInput   string
	Content     string
	References  []ContextReference
	CreatedAt   time.Time
}

type KnowledgeScope string

const (
	KnowledgeScopeProject KnowledgeScope = "project"
	KnowledgeScopeGlobal  KnowledgeScope = "global"
)

type KnowledgeItem struct {
	ID            string
	Scope         KnowledgeScope
	ProjectID     string
	Kind          string
	Title         string
	SourceEventID string
	ContentRef    string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// SummaryPayload is stored as objects/summaries/<id>.json and referenced
// by a summary.created event via PayloadRef = "summary:<id>".
type SummaryPayload struct {
	ProviderSegmentID string   `json:"provider_segment_id"`
	CoveredMessageIDs []string `json:"covered_message_ids"`
	Text              string   `json:"text"`
}

// HandoffCandidate describes the incoming provider selection for a new message.
type HandoffCandidate struct {
	RouteID string
	ModelID string
}

// HandoffContext is the input to HandoffPolicy.Summarize.
type HandoffContext struct {
	Session  Session
	Segment  ProviderSegment
	Messages []Message // messages recorded during the segment's lifetime
	// CallProvider performs a single-shot text generation for summary purposes.
	// May be nil; implementations must fall back gracefully.
	CallProvider func(prompt string) (string, error)
}

// HandoffPolicy detects provider switches and generates handoff summaries.
// New strategies (AI-generated, structured, rule-based) can be registered
// without changing core logic.
type HandoffPolicy interface {
	Name() string
	DetectSwitch(last ProviderSegment, incoming HandoffCandidate) bool
	Summarize(ctx HandoffContext) (SummaryPayload, error)
}

// PacketBuildContext is the input to a ContextPacketPolicy.
type PacketBuildContext struct {
	Session       Session
	Messages      []Message
	Ancestors     []Event
	HeadEventID   string
	UserInput     string
	Note          string
	ContextBudget int    // max chars; 0 = policy default
	RouteLabel    string // e.g. "Claude Code CLI / Claude Sonnet 4.6"
	// LoadSummary retrieves a SummaryPayload by ID (from a summary.created event PayloadRef).
	// May be nil if no summary loader is available.
	LoadSummary func(id string) (SummaryPayload, error)
}

// ContextPacketPolicy builds a ContextPacket from a PacketBuildContext.
// Implementations define how messages, summaries, and KB items are selected
// and assembled. New policies can be registered without changing core logic.
type ContextPacketPolicy interface {
	Name() string
	Build(ctx PacketBuildContext) ContextPacket
}

type EventInput struct {
	Type           EventType
	ProjectID      string
	SessionID      string
	BranchID       string
	ParentEventIDs []string
	PayloadRef     string
}

type EventStore interface {
	AppendEvent(input EventInput) (Event, error)
	GetEvent(id string) (Event, error)
	ListAncestors(headEventID string, limit int) ([]Event, error)
	MoveHead(projectID string, sessionID string, branchID string, eventID string) (Head, error)
	CreateGraphBranch(sessionID string, fromEventID string) (GraphBranch, error)
	CreateMerge(projectID string, sessionID string, branchID string, parentEventIDs []string) (Event, error)
}
