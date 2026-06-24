package core

import "time"

type SourceTool string

const (
	SourceToolCodex   SourceTool = "codex"
	SourceToolCopilot SourceTool = "copilot"
	SourceToolClaude  SourceTool = "claude"
	SourceToolGemini  SourceTool = "gemini"
)

type Session struct {
	ID            string
	ProjectID     string
	SourceTool    SourceTool
	SourceID      string
	Title         string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	ParentID      string
	BranchFromID  string
	ActiveRouteID string
	ActiveModelID string
}

// RouteCandidate is a route/model pair with a priority ordering.
type RouteCandidate struct {
	RouteID  string
	ModelID  string
	Priority int
}

// RouteSelectionContext is the input to a RouteSelectionPolicy.
type RouteSelectionContext struct {
	Session          Session
	Candidates       []RouteCandidate // sorted by priority ascending
	RequestedRouteID string           // explicit per-message override; may be empty
	RequestedModelID string
}

// RouteSelectionPolicy decides which route and model to use for a message.
// Implementations can encode manual selection, failover, round-robin, etc.
type RouteSelectionPolicy interface {
	Name() string
	Select(ctx RouteSelectionContext) (routeID string, modelID string, err error)
}

type Message struct {
	ID        string
	SessionID string
	Role      string
	Content   string
	CreatedAt time.Time
	SourceID  string
}

type Branch struct {
	ID            string
	ParentID      string
	SessionID     string
	FromMessageID string
	CreatedAt     time.Time
}

type SessionStore interface {
	Init() error
	ListSessions() ([]Session, error)
	GetSession(id string) (Session, []Message, error)
	CreateBranch(sessionID string, fromMessageID string) (Session, error)
}
