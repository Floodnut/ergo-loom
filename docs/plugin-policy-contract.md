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

Approval decisions should be recorded as events:

- `tool.requested`
- `tool.approved`
- `tool.rejected`
- `tool.completed`
- `tool.failed`

Planned approval policy names:

- `ask-per-command`
- `allow-similar-after-approval`
- `deny-by-default`
- `project-trusted`

## Candidate Output Lifecycle

Candidate output belongs to a parallel `ChatRun`. It is not part of the main
transcript until accepted.

States:

```text
pending -> ready -> accepted
pending -> ready -> rejected
pending -> rejected
```

Future states may include:

```text
merged
superseded
expired
```

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
