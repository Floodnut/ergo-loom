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
