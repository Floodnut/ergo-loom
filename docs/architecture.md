# Ergo Loom Architecture

Ergo Loom is a local workspace for AI work context. It preserves original session exports from tools like Codex, VSCode Copilot, Cursor, Claude Code, and Gemini CLI, converts them into a shared internal model, and lets users inspect, branch, merge, and reuse that context across tools, accounts, and providers.

The long-term context model is a local event graph: Ergo Loom owns the local context, while provider sessions and CLI processes are execution channels. See `docs/context-graph.md`.

## Product Shape

- Product name: Ergo Loom
- Repository and package name: `ergo-loom`
- CLI command: `ergo`
- Primary runtime stack: Go for core and CLI, TypeScript for UI
- Initial persistence: local SQLite
- Long-term context direction: structured local event files with SQLite projections
- Provider and agent integrations should be plugin-shaped from the start
- The default installed experience is a lightweight chat app, with the CLI installed beside it

## MVP Scope

The first useful version should behave like a local session graph manager:

1. Import sessions from supported tools while preserving the original source payload.
2. List stored sessions.
3. Show a session and its messages.
4. Create a branch from a specific message in a session.
5. Merge sessions with a simple deterministic strategy first, then add manual merge controls later.
6. Index local files for keyword search before adding vector search.
7. Track provider profiles and token usage across accounts.

## Repository Layout

```text
ergo-loom/
  apps/
    desktop-or-web/        # TypeScript UI, added when the UI work starts
    cli/                   # Go CLI entrypoint
  internal/
    core/                  # Session, message, branch, merge, KB domain types
    storage/               # SQLite-backed persistence
  docs/
    architecture.md
  data/
    local.db               # Local SQLite database, ignored by git
```

The `packages/ui` workspace should be added only when shared TypeScript UI components become useful.

## Core Model

The internal model is intentionally small:

- `source_tool`: a known upstream tool such as `codex`, `copilot`, `claude`, or `gemini`
- `session`: a conversation or work thread imported from one source tool
- `message`: a single message within a session
- `branch`: a derived session created from another session at a specific message
- `raw_import`: the original source payload and metadata needed for reimport
- `project`: a local workspace configuration that chooses which access routes are available while chatting
- `provider_plugin`: an AI provider integration such as OpenAI, Anthropic, Gemini, Copilot, Cursor, or a local model
- `provider_profile`: a configured account or credential boundary for one provider
- `access_route`: a specific way to use a provider, including license-native, free, handoff, bridge, or local routes
- `agent_plugin`: a runnable behavior such as chat, review, summarize, merge, or import
- `capability`: a local non-AI operation such as DB storage, file indexing, search, HTTP, git, or export
- `token_ledger`: usage records by provider profile, model, session, and agent run

Original tool formats should never be overwritten. Every import adapter writes raw source data first, then writes normalized records derived from that raw data.

## Account and Provider Boundaries

Ergo Loom should not assume one account per tool. A user may have multiple OpenAI, Anthropic, Gemini, Copilot, Cursor, work, personal, or local profiles. Sessions belong to Ergo Loom first. Provider profiles are execution targets and accounting boundaries.

```text
session context -> agent run -> provider profile -> provider plugin -> model
```

This lets the same normalized session be reused with a different account, provider, or model without rewriting the original session.

## Access Routes

Ergo Loom should avoid API-billed routes by default. For every provider, it should model practical access paths that use an existing license, a free tier, manual handoff, a local bridge, or local compute:

- `subscription_native`: uses an existing paid license through an official local app, CLI, app-server, or IDE bridge
- `licensed_handoff`: uses an existing paid license through manual prompt handoff and transcript import
- `free_native`: uses an official free tier through a CLI, local app, or bridge
- `free_handoff`: uses a free web chat manually
- `local`: uses local compute or non-AI capabilities
- `unavailable`: no stable route for the user's current account, region, or product state

The route selection rule is:

```text
prefer stable license-native routes
then free native routes
then handoff routes
then local routes when the task can run without a remote AI
```

Projects choose a subset of access routes. The chat app should default to the first enabled route for the current project, and users should be able to add or remove routes later without changing historical sessions.

Example routes:

- Codex Plus through local Codex CLI or app-server: `subscription_native`
- VS Code Copilot through a VS Code extension bridge: `subscription_native`
- Gemini CLI through personal free tier: `free_native`
- Claude web free chat: `free_handoff`
- Local model runtime: `local`

## Plugin Shape

Provider plugins answer "which AI can be called?" Agent plugins answer "what work should be done?" Capabilities answer "what can the local runtime do without AI?"

Provider plugins should eventually expose:

```text
id
list models
send request
stream response
estimate tokens
```

Agent plugins should eventually expose:

```text
id
required capabilities
optional provider profile
run
```

Local capabilities stay separate from AI providers. API calls, SQLite persistence, import/export, keyword search, file indexing, git inspection, and prompt rendering should be callable without consuming AI tokens.

## Token Ledger

Token and cost tracking is a first-class feature because Ergo Loom may route work across multiple providers, accounts, and agents.

The ledger should record:

- provider plugin
- provider profile
- model
- optional session
- optional agent
- prompt tokens
- completion tokens
- estimated cost
- actual cost
- request id
- success or failure
- timestamp

The first implementation can support manual usage records and empty summaries before actual provider calls exist.

## Import Strategy

Import adapters are narrow translators:

```text
source export -> raw_import -> normalized sessions/messages/branches
```

This allows Ergo Loom to re-run an adapter if a source format changes or if the internal schema evolves. Adapter failures should not destroy the original payload.

Initial adapters:

- `codex`
- `claude`
- `gemini`
- `copilot`
- `cursor`

## Merge Strategy

The first merge implementation can be time-ordered:

1. Select source sessions.
2. Collect messages.
3. Sort by message creation time, then stable message ID.
4. Write a merged session with references back to original messages.

Manual merge can come later with UI affordances for picking, dropping, and reordering messages.

## UI Shape

The TypeScript UI should start as a working app surface, not a landing page:

- Left: session list and filters
- Center: chat transcript
- Right: context, branches, and source metadata

The UI should operate on the same local model as the CLI.

The default product experience is the chat app itself. A user should be able to install Ergo Loom, open the app, and start a local AI-style chat immediately. The CLI is a companion surface for import, automation, scripting, and diagnostics.

The first implementation should stay lightweight:

- Go serves the local app and owns SQLite access.
- Static web assets are embedded into the Go binary for installable builds.
- A native shell can be added later if needed.
- Electron is allowed when it buys meaningful desktop integration, but it should not be the default weight unless the product needs it.

## Realtime Core Communication

The chat app should communicate with the local core over a realtime channel. Normal request/response APIs are not enough because assistant output, tool calls, usage updates, and cancellation need to stream.

The target protocol is protobuf envelopes over a socket transport:

- Browser-compatible installed UI: WebSocket with binary protobuf frames
- Native shell: Unix domain socket, named pipe, or localhost TCP with the same protobuf frames
- Bootstrap implementation: HTTP JSON plus chunked streaming until generated protobuf code is introduced

See `docs/realtime-protocol.md` and `proto/ergo/loom/v1/realtime.proto`.

## Local Knowledge Base

The first knowledge base version should index local files into SQLite with keyword search. Vector search is useful later, but it should not block the session graph MVP.

## Design Principles

- Preserve source formats.
- Normalize into a small common model.
- Keep import adapters replaceable.
- Keep provider profiles separate from sessions.
- Track token usage across accounts and agents.
- Keep AI-powered features separate from local capabilities.
- Start as a local-first session graph manager.
- Avoid speculative platform features until the core loop is useful.
