# Project Settings Contract

Ergo Loom projects are context boundaries. A project is not just a folder label;
it defines which local path, provider routes, policies, tools, and knowledge
scope are valid for chats under that project.

This contract is intentionally small. UI, backend, and plugin work should use
these names unless the team explicitly changes them.

## Project Fields

Persisted project settings:

```text
id
display_name
root_path
context_policy
handoff_policy
route_policy
tool_approval_policy
kb_scope_policy
```

Current implementation already stores:

- `context_policy`
- `handoff_policy`
- `route_policy`

Planned fields:

- `tool_approval_policy`
- `kb_scope_policy`

## Field Semantics

`root_path`
: Local project boundary. If a chat changes to a different root path, it should
  be treated as moving to another project unless an explicit project/chat merge
  is performed.

`context_policy`
: Selects the `ContextPacketPolicy` used to build bounded provider input.

`handoff_policy`
: Selects how Ergo Loom summarizes and transfers context when provider routes
  change.

`route_policy`
: Selects how a chat chooses an access route/model when the user has not made a
  one-shot override.

`tool_approval_policy`
: Controls approval behavior for tool execution. This is project-level because
  command risk depends on the project path and team context.

`kb_scope_policy`
: Controls which knowledge sources are eligible for retrieval. Initial values
  should be `project-only`, `project-and-global`, and `disabled`.

## Related Tables

Project access is split from project settings:

```text
projects
project_access_routes
moderator_preferences
```

`project_access_routes` controls which provider access routes are enabled and
their priority. Provider, route, and model must stay separate:

- Provider: who owns the capability, such as `anthropic`.
- Access route: how Ergo Loom reaches it, such as `claude-code-cli`.
- Model: which model is selected, such as `Claude Sonnet 4.6`.

## API Shape

Existing:

```text
GET /api/state
GET /api/plugins
POST /api/projects
PATCH /api/projects/{projectID}
POST /api/projects/{projectID}/routes
DELETE /api/projects/{projectID}/routes
POST /api/projects/{projectID}/moderator
```

Recommended next endpoint:

```text
PATCH /api/projects/{projectID}/settings
```

Request body:

```json
{
  "contextPolicy": "segment-chain",
  "handoffPolicy": "route-change",
  "routePolicy": "manual",
  "toolApprovalPolicy": "ask-per-command",
  "kbScopePolicy": "project-and-global"
}
```

The backend should validate each policy name against `/api/plugins`.

## Collaboration Rule

Any feature that needs project-level behavior should add a project setting before
adding hidden defaults in UI or provider code. Hidden project assumptions are the
main way parallel work will drift.
