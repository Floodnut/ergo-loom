# Plugin And Policy Contract

Ergo Loom is a local AI work context manager. This document defines the shared
contract that backend, frontend, and provider/tool work should follow.

## Product Invariants

1. Ergo Loom owns the local context.
   Provider sessions, CLI sessions, browser chats, and IDE bridges are execution
   channels. They are not the source of truth.

2. Providers are plugins and drivers.
   A provider can disappear, expire, fail auth, or reset its remote context.
   Ergo Loom must still be able to reconstruct the next request from local
   context.

3. One Ergo Loom chat can have many provider runs.
   The relationship is one chat to many provider segments, not one chat to one
   provider session.

4. The main active run is a chat-level slot.
   It is not a provider, model, CLI process, or remote thread.

5. Queue items are queue items.
   A queue item becomes a normal, steering, or parallel action only when it is
   consumed.

6. Steering targets the main active run.
   The current provider segment receives the steering input, but the graph
   records steering as a chat-level concern.

7. Parallel work creates candidate output.
   It is a sibling/sub run until accepted or rejected.

8. Moderator is not an orchestrator.
   It only mediates provider flow, ordering, token expiry, and fallback handoff.

## Registry Categories

`GET /api/plugins` exposes registry data for UI and tooling.

Registry categories:

- `providers`
- `agents`
- `capabilities`
- `tools`
- `routes`
- `models`
- `policies.contextPackets`
- `policies.handoffs`
- `policies.routeSelection`

Provider, access route, model, and driver are distinct.

```text
ProviderPlugin -> AccessRoute -> ProviderModel -> Driver execution
```

Example:

```text
anthropic -> claude-code-cli -> anthropic-claude-sonnet-4-6 -> ClaudeCLIDriver
```

## ContextPacketPolicy

Responsibility:

- select local context
- respect provider/project budget
- include source references
- include summaries and knowledge when policy allows

It must not:

- call providers directly
- mutate session state
- execute tools
- decide provider routing

## HandoffPolicy

Responsibility:

- detect whether provider route/model transition needs a handoff summary
- produce a summary payload for the next context packet

It may call a provider through the supplied callback for summarization, but it
must have a non-AI fallback.

It must not:

- choose the next route
- execute user tools
- alter the main transcript directly

## RouteSelectionPolicy

Responsibility:

- select an access route/model from project-enabled candidates
- respect explicit one-shot overrides
- support failover or manual selection modes

It must not:

- build context packets
- execute provider requests
- approve tools
- merge candidate outputs

## Moderator

Moderator is reactive, not an orchestrator. It responds to provider events and
state changes. It does not plan work on its own.

Moderator can decide:

- continue
- failover
- suspend
- terminate

Moderator may react to:

- provider segment completed
- provider segment error
- timeout
- auth failure
- token budget limit
- user abort

Moderator must not:

- select the next route or model
- build context packets
- generate handoff summaries
- approve or reject tools
- choose candidate outputs
- infer user intent

When a moderator returns `failover`, the server calls `RouteSelectionPolicy`.
The selected route is outside the moderator contract.

```go
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

type Moderator interface {
    OnSegmentEnd(ctx ModerationContext) ModerationDecision
    OnBudgetWarning(ctx ModerationContext) ModerationDecision
}
```

Route/model hints are intentionally absent. If future requirements need them,
add optional soft hints that `RouteSelectionPolicy` may ignore.

## Tool Runtime And Approval Policy

Tool execution is global to Ergo Loom, not owned by AI providers.

Tool categories should include:

- shell command
- HTTP request
- file read/write
- git
- docker
- kubectl
- OpenStack CLI/API

### Go Interface

```go
// internal/core/tool_approval.go

type ToolCallRequest struct {
    ID          string
    SessionID   string
    ToolName    string
    Parameters  map[string]any
    RequestedAt time.Time
}

type ApprovalVerdict string

const (
    VerdictAutoApprove ApprovalVerdict = "auto"
    VerdictAskUser     ApprovalVerdict = "ask_user"
    VerdictDeny        ApprovalVerdict = "deny"
)

type ApprovalResult struct {
    Verdict ApprovalVerdict
    Reason  string
}

type ToolApprovalPolicy interface {
    Name() string
    Evaluate(req ToolCallRequest) ApprovalResult
}
```

### DB Schema

```sql
CREATE TABLE tool_call_requests (
    id           TEXT PRIMARY KEY,
    session_id   TEXT NOT NULL,
    tool_name    TEXT NOT NULL,
    parameters   TEXT NOT NULL,   -- JSON
    status       TEXT NOT NULL DEFAULT 'pending',
    -- pending | approved | rejected | executed | failed
    verdict      TEXT,            -- auto | ask_user | deny
    reason       TEXT,
    requested_at DATETIME NOT NULL,
    resolved_at  DATETIME
);
```

### API

```text
POST   /api/sessions/{sessionID}/tool-calls           AI requests tool execution
GET    /api/sessions/{sessionID}/tool-calls/pending   pending approvals for UI
PATCH  /api/tool-calls/{id}                           user approves or rejects
```

`PATCH` body: `{ "status": "approved" | "rejected" }`

### Approval Decisions as Events

- `tool.requested`
- `tool.approved`
- `tool.rejected`
- `tool.completed`
- `tool.failed`

### Policy Names

| Name | Behavior |
|---|---|
| `safe-only` | read/search tools auto-approve; write/exec/git ask_user (recommended default) |
| `ask-per-command` | every tool call goes to the user |
| `allow-similar-after-approval` | once a tool+params pattern is approved, repeat runs auto-approve |
| `deny-by-default` | all tool calls rejected unless explicitly allow-listed |
| `project-trusted` | all tool calls auto-approved for this project |

`safe-only` is the recommended default. It lets read-only exploration proceed
without interruption while requiring explicit approval for any action that
modifies state.

### Package Location

Implement in `internal/toolpolicy/policy.go` following the same Registry
pattern as `internal/packetpolicy`, `internal/handoffpolicy`, and
`internal/routepolicy`:

```go
func NewRegistry() Registry { ... }
func (r Registry) Register(p core.ToolApprovalPolicy) { ... }
func (r Registry) GetOrDefault(name string) core.ToolApprovalPolicy { ... }
func (r Registry) List() []string { ... }
```

Register at server startup alongside the other policy registries.

## Candidate Output Lifecycle

Candidate output belongs to a parallel `ChatRun`. It is not part of the main
transcript until accepted.

States:

```text
pending -> ready -> accepted -> merged
pending -> ready -> rejected
pending -> ready -> superseded  (another candidate in the same turn was accepted first)
pending -> rejected
pending -> failed -> rejected
```

Terminal states: `merged`, `rejected`, `superseded`.

`failed` stays visible long enough for the UI to show the provider error. It can
then be cleaned up as `rejected`.

### superseded Scope

superseded applies only to candidates that share the same user turn, not the
entire session. The scoping field is `trigger_event_id`: the event ID of the
user message that caused this candidate run to start.

```sql
UPDATE candidate_outputs
SET status = 'superseded'
WHERE session_id     = ?
  AND trigger_event_id = ?   -- same user turn only
  AND status         = 'ready'
  AND id             != ?    -- exclude the just-accepted candidate
```

`candidate_outputs` must include a `trigger_event_id TEXT` column. It is set
when the parallel run is created and must not be null.

### Accepted Candidate Semantics

Use the hybrid merge/materialization model:

1. Create a `merge.created` event on the main branch.
   - Parent 1: current main head.
   - Parent 2: candidate run output event or candidate output reference.
   - Payload references the accepted candidate ID and source chat run ID.

2. Materialize the accepted content as a new `message.assistant` event after the
   merge event.
   - This keeps the visible transcript simple.
   - The merge event preserves provenance.

Do not splice all candidate run events into the main branch. That makes
parallel work indistinguishable from main work and will complicate branch/merge
reasoning.

Do not use pointer-only acceptance as the only representation. It makes replay
and provider context packet construction harder because accepted answers would
not look like transcript content.

## Provider Expiry And Resume Contract

### Error Classification

Drivers must classify failures using `DriverError`. The server converts
`DriverError.Kind` into `SegmentEndReason` before calling the Moderator.
Unclassified errors map to `ReasonError`.

```go
// internal/provider/driver.go

type DriverErrorKind string

const (
    ErrKindTransient   DriverErrorKind = "transient"    // temporary; retry is safe
    ErrKindAuthFailure DriverErrorKind = "auth_failure" // token expired; user action needed
    ErrKindRateLimit   DriverErrorKind = "rate_limit"   // provider throttling; back off
    ErrKindSessionEnd  DriverErrorKind = "session_end"  // provider session ended; resume as new segment
    ErrKindUnavailable DriverErrorKind = "unavailable"  // provider is down
    ErrKindFatal       DriverErrorKind = "fatal"        // unrecoverable
)

type DriverError struct {
    Kind      DriverErrorKind
    Message   string
    Retryable bool
}

func (e *DriverError) Error() string { return e.Message }
```

### Ping

Drivers should implement `Ping` to let the Moderator or health checks verify
that a provider session is still alive before starting a new segment.

```go
type ChatDriver interface {
    ProviderPluginID() string
    CanExecute(ChatRequest) bool
    Respond(ctx context.Context, request ChatRequest, onEvent func(Event)) (ChatResponse, error)
    Ping(ctx context.Context) error  // return nil if alive; DriverError otherwise
}
```

Drivers that cannot implement a lightweight ping should return `nil`
immediately (no-op). The server must not treat a nil-returning ping as a
health guarantee.

### SegmentEndReason Mapping

```text
ErrKindAuthFailure  → ReasonAuthFailure
ErrKindRateLimit    → ReasonTimeout
ErrKindTransient    → ReasonTimeout
ErrKindSessionEnd   → ReasonSessionEnd  (new reason, added to core/moderator.go)
ErrKindUnavailable  → ReasonError
ErrKindFatal        → ReasonError
unclassified error  → ReasonError
```

### Resume Semantics

Resume does not restore a provider session. It starts a new `ProviderSegment`
on the same or a different route, using Ergo Loom's local context packet as
the provider's memory replacement.

```text
provider session expired (ErrKindSessionEnd)
        ↓
Moderator.OnSegmentEnd(reason=session_end) → ActionFailover
        ↓
RouteSelectionPolicy: may re-select the same route
        ↓
HandoffPolicy: treats session_end the same as a route change
        ↓
new ProviderSegment starts (ExternalThreadID is fresh)
```

`ExternalThreadID` from the expired segment is not reused. The new context
packet reconstructs provider memory from local events.

### Driver Implementation Checklist

Each driver is responsible for classifying its own errors:

- `ClaudeCLIDriver`: parse stderr for auth patterns; map exit codes to kinds
- `CodexAppServerDriver`: HTTP 401/403 → auth_failure; 429 → rate_limit; 5xx → transient
- `CopilotBridgeDriver`: same HTTP pattern as above

## Active Chat Run And Queue Consumption

### Definitions

`GetActiveChatRun` returns the main run that is currently executing or waiting
for user approval. `queued` runs are not active — they have not started yet.

```go
// active = running or waiting_approval only
func (s Store) GetActiveChatRun(sessionID string) (core.ChatRun, error)

// next item waiting to be consumed
func (s Store) NextQueuedChatRun(sessionID string) (core.ChatRun, error)

// transition queued → running before execution
func (s Store) UpdateChatRunStatus(id string, status core.ChatRunStatus) error
```

The existing `ActiveMainChatRun` includes `queued` in its status filter. It
must be narrowed to exclude `queued`, or replaced with `GetActiveChatRun`.

### Incoming Message Dispatch

When a new message arrives, the server checks for an active run before
deciding how to proceed:

```text
new message arrives
        ↓
GetActiveChatRun()
        ↓
not found                      found (running / waiting_approval)
    ↓                                    ↓
execute immediately              mode?
                          ┌──────────────┴──────────────┐
                        normal                       steering
                          ↓                              ↓
                    AddQueueItem                  injectSteering(run.ID)
                    (waits for active to finish)  (delivered to current run)
```

`parallel` is a separate endpoint and is not part of this dispatch.

### Queue Consumption

`maybeConsumeQueue` runs after every `CompleteChatRun` and after a Moderator
`ActionTerminate` decision:

```go
func (s Server) maybeConsumeQueue(sessionID string) {
    if _, err := s.store.GetActiveChatRun(sessionID); err == nil {
        return // active run still present
    }
    run, err := s.store.NextQueuedChatRun(sessionID)
    if err != nil {
        return // queue empty
    }
    s.store.UpdateChatRunStatus(run.ID, core.ChatRunRunning)
    go s.executeChatRun(run)
}
```

### DB Index

Add an index to support efficient active-run lookups:

```sql
CREATE INDEX IF NOT EXISTS idx_chat_runs_session_active
ON chat_runs(session_id, branch_id, role, status);
```

### Implementation Checklist

- Narrow `ActiveMainChatRun` status filter to exclude `queued`, or add
  `GetActiveChatRun` as a separate store method
- Add `NextQueuedChatRun` store method
- Add `UpdateChatRunStatus` store method
- Add `maybeConsumeQueue` to server, call after `CompleteChatRun`
- Update `streamSessionMessage` to check active run before executing:
  normal → queue, steering → inject, no active run → execute immediately

## Knowledge Boundary

Knowledge is reusable context, not a transcript dump.

Initial scopes:

- project knowledge
- global knowledge

Promotion should be explicit or policy-driven. Context packet policies decide
how much retrieved knowledge is included in provider input.

## Team Rule

When adding a new plugin or policy, first answer:

1. Which registry category owns it?
2. Which project setting selects it, if any?
3. Which event type records its durable effect?
4. Is the effect source-of-truth state or just a UI projection?
