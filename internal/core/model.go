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
	ID           string
	ProjectID    string
	SourceTool   SourceTool
	SourceID     string
	Title        string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	ParentID     string
	BranchFromID string
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
