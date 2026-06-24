# Ergo Loom Context Graph

Ergo Loom is a local context manager for AI work. Provider sessions, CLI
processes, browser handoffs, and IDE bridges are execution channels. They are
not the source of truth for the user's work context.

This document defines the direction for storing, branching, merging, retrieving,
and replaying local context as Ergo Loom grows beyond a single linear chat.

## Core Assumptions

1. Ergo Loom owns the local context.
   - Chat messages, tool requests, approvals, file references, summaries,
     provider runs, branches, merges, and knowledge references should be
     recoverable from Ergo Loom-managed local state.
   - A provider may maintain its own remote or CLI session, but that session is
     an optimization. It may expire, reset, lose auth, or disappear after a
     reboot.

2. Providers and tools are plugins.
   - AI providers such as Codex/ChatGPT, Claude, Copilot, Gemini, and Ollama are
     execution plugins.
   - Non-AI capabilities such as shell, git, HTTP, Kubernetes, OpenStack, file
     search, and database access are tool plugins.
   - Global policy, approvals, routing, and context selection remain provider
     neutral.

3. Ergo Loom has both a backend and a frontend.
   - The backend owns local persistence, indexing, provider/tool drivers,
     approvals, routing, and context packet construction.
   - The frontend is a realtime workspace UI over that local backend.

## Recommended Shape

Ergo Loom should model context as a Git-like event graph with searchable
projections.

```text
Structured local files  -> source of truth
SQLite projections      -> fast lists, search, usage, UI state
Context retriever       -> selects relevant context
Context packet builder  -> prepares provider/tool input
Provider/tool drivers   -> execute work through plugins
```

The exact storage engine can evolve, but the domain model should not depend on
provider-owned chat sessions.

## Event Graph

Every meaningful change in a chat or project should become an event.

Examples:

- `message.user`
- `message.assistant`
- `provider.run.started`
- `provider.run.completed`
- `tool.requested`
- `tool.approved`
- `tool.rejected`
- `tool.completed`
- `file.referenced`
- `summary.created`
- `branch.created`
- `merge.created`
- `knowledge.promoted`
- `moderator.handoff`

Each event has stable identity and graph links:

```json
{
  "id": "evt_123",
  "type": "message.user",
  "project_id": "project_default",
  "session_id": "session_abc",
  "branch_id": "branch_main",
  "parent_event_ids": ["evt_122"],
  "created_at": "2026-06-24T10:00:00Z",
  "payload_ref": "objects/messages/msg_123.json"
}
```

Linear chat is a chain of events. Parallel provider replies are sibling events.
Merges are events with multiple parents.

```text
user request
  ├─ codex reply
  ├─ claude reply
  └─ copilot reply

branch A head
branch B head
  └─ merge event
```

## Sessions, Branches, And Heads

Sessions should be treated as views over the graph, not as the only structure
that owns context.

- A session is a user-facing work thread.
- A branch is a named line of work inside or derived from a session.
- A head is the current event pointer for a branch.
- A merge records which parent heads were combined and why.

This allows steering, queueing, parallel work, and later merge decisions without
copying entire transcripts.

## Chat Runs And Provider Segments

Provider execution is split into two layers:

- `ChatRun` is Ergo Loom's chat-level execution slot. It can be `main` or
  `parallel`.
- `ProviderSegment` is the provider/model execution piece inside a `ChatRun`.

The main active run is a chat-level concept, not a provider/model identity. A
main run may start with one provider and continue with another if moderator
handoff, quota exhaustion, retry, or provider replacement happens.

```text
Ergo Loom event graph
  -> context packet
  -> chat run
  -> provider segment
  -> result event
```

Provider state should record external identifiers, but those identifiers should
not be required to reconstruct context.

```json
{
  "provider_id": "anthropic",
  "route_id": "claude_cli",
  "external_thread_id": "optional-provider-id",
  "last_synced_event_id": "evt_123",
  "status": "fresh"
}
```

If a provider thread expires, Ergo Loom should build a new context packet from
local context and continue through the same or another provider.

Steering targets the main `ChatRun` by default. The currently active provider
segment receives the steering input, but the graph records steering as a
chat-run concern so provider/model handoff does not change the meaning of the
run.

Parallel work creates a parallel `ChatRun` from the current context snapshot.
Its result remains a candidate until explicitly merged into the main run or
branch.

## Context Retrieval

Ergo Loom should not send the whole project to a provider by default. It should
retrieve only the useful context for the current task.

Retrieval priority:

1. Current branch ancestry.
2. Recent events in the active session.
3. Active summaries and decisions.
4. Relevant tool/file events.
5. Sibling branches and merged sessions when useful.
6. Project knowledge.
7. Global knowledge.

Initial retrieval can use SQLite FTS and simple scoring. Vector search can be
added later without changing the event graph model.

## Context Packets

A context packet is the bounded input prepared for a provider or tool.

It should include:

- current user request
- selected project and branch
- relevant recent messages
- summaries/checkpoints
- relevant tool events and approval state
- relevant file or artifact references
- provider constraints
- source references for what was included

Context packets should be recorded for debugging and reproducibility, but large
payloads should be stored by reference when needed.

## Project And Global Knowledge

Project context and global knowledge are related but separate.

Project context is the living work graph for a project. Global knowledge is
reusable information that should prevent repeating expensive work.

Examples of global knowledge:

- provider authentication rules
- recurring setup procedures
- known error resolutions
- durable design decisions
- expensive analysis results worth reusing

Promotion to project or global knowledge should be explicit or policy-driven.
Not every chat message should become long-term knowledge.

## Storage Direction

Preferred long-term direction:

```text
~/.ergo-loom/
  objects/
    events/
    messages/
    artifacts/
  projects/
    <project-id>/
      project.json
      heads.json
      sessions/
  knowledge/
    global/
    projects/
  indexes/
    ergo.db
```

Structured files are the durable source. SQLite is a projection that can be
rebuilt from source files.

The current SQLite-first implementation can evolve toward this shape gradually.
The first step is to introduce event graph concepts behind interfaces, then
change persistence without rewriting UI/provider logic.

## Implementation Order

1. Define core graph types and interfaces:
   - `Event`
   - `Head`
   - `Branch`
   - `ChatRun`
   - `ProviderSegment`
   - `ContextPacket`
   - `KnowledgeItem`

2. Add an `EventStore` interface:
   - append event
   - get event
   - list ancestry
   - move head
   - create branch
   - create merge

3. Add a SQLite-backed event projection first.

4. Record existing chat messages and tool activity as events.

5. Add a context packet builder that reads through the event graph and indexes.

6. Move provider calls to consume context packets instead of raw latest text.

7. Connect queueing, steering, and parallel provider replies to event graph
   semantics.

8. Add structured file source storage and make SQLite rebuildable.

9. Add project/global knowledge promotion and retrieval.

## Non-Goals For The First Pass

- Do not build a full vector database immediately.
- Do not replace all existing session/message tables at once.
- Do not make provider-specific context policy the source of truth.
- Do not store every transient UI detail as a graph event.

The goal is to make Ergo Loom's local context model stable before adding more
provider-specific behavior.
