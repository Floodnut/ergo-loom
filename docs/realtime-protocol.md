# Realtime Protocol

Ergo Loom's default app is a chat client. The UI must receive assistant output as a stream, not as one completed response.

## Direction

The core runtime should expose a long-lived realtime channel between the installed UI and the local core:

```text
installed UI shell <-> realtime transport <-> Ergo Loom core <-> provider plugins
```

The message contract should be protobuf. The first browser-compatible transport should be WebSocket with binary protobuf frames. A native shell can later use the same protobuf frames over a Unix domain socket, named pipe, or localhost TCP socket.

## Why Protobuf

- Stable contract between UI, CLI, core, and future plugin processes.
- Efficient binary frames for high-frequency token deltas.
- Backward-compatible schema evolution.
- Shared event model for local agents and remote AI providers.

## Transport Plan

1. Current bootstrap: HTTP JSON plus chunked streaming for chat output.
2. Next: WebSocket endpoint carrying protobuf `ClientEnvelope` and `ServerEnvelope` frames.
3. Later: native app shell can swap WebSocket for local IPC without changing envelope messages.

The current HTTP streaming endpoint exists to validate responsiveness before introducing generated protobuf code.

## Event Model

Every interactive chat run should emit events like this:

```text
user message accepted
assistant message started
assistant token delta
tool call started
tool call delta
tool call finished
reasoning or progress status summary
assistant message finished
usage recorded
run failed
```

The UI should render deltas immediately and reconcile with the saved message once the run is finished.
Agent activity events should render in a bounded, collapsible panel below the chat transcript. Ergo Loom should show provider-exposed activity such as thinking states, tool progress, file activity, approvals, and concise reasoning summaries.

## Backpressure

The core should treat streaming as cancellable. The protocol needs `CancelRun` because users will stop generations, switch sessions, or close the app.

## Persistence

Streaming events are not the source of truth. The source of truth remains SQLite:

- User messages are written before a run starts.
- Assistant deltas are rendered live.
- The final assistant message is written after completion.
- Token usage is written after provider usage is known.
