# Chat Input Filter

Ergo Loom routes every user chat input through a filter layer before storing it
or sending it to any AI provider.

The initial chain is intentionally a no-op. Its job is to reserve the boundary
where later policy can live:

- product identity enforcement
- security policy checks
- tool-call preflight classification
- command rewrite or user confirmation hints
- provider-specific routing guards
- context trimming or metadata tagging

Current flow:

1. Web handler receives chat input.
2. `internal/chatfilter.Chain` evaluates the input.
3. The result can allow, block, or rewrite content.
4. Only filtered content is stored and sent to the assistant runner.

Filters should stay provider-neutral. Provider adapters can consume metadata
from filtered input later, but should not own global Ergo Loom policy.
