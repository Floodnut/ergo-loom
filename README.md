<p align="center">
  <img src="apps/desktop-or-web/static/icon.svg" alt="Ergo Loom" width="96" height="96">
</p>

# Ergo Loom

Ergo Loom is a local AI work context manager. It is designed to collect chat and task context spread across tools like Codex, Claude, VS Code Copilot, Cursor, and Gemini, then let you branch, merge, inspect, and reuse that context from one local workspace.

The product idea is simple: AI conversations are not disposable chat logs. They are working context. Ergo Loom keeps that context local, structured, branchable, and portable across providers and accounts.

## Current Status

Ergo Loom is an early local-first prototype. The current app already includes:

- A lightweight installed desktop shell powered by Electron
- A Go local core/server with embedded web UI
- Local SQLite persistence at `~/.ergo-loom/local.db`
- A TypeScript chat UI with left project/session navigation
- Provider/model route scaffolding for Codex/ChatGPT, Claude, Copilot, Gemini, Cursor, and local models
- Codex app-server integration scaffolding
- Tool approval UI and local terminal execution
- Right-side workspace tabs for tool activity, terminal tabs, and file viewer tabs
- macOS `.app` and `.dmg` packaging
- GitHub Releases update feed wiring through `electron-updater`

## Product Direction

Ergo Loom aims to become a local workspace for:

- Managing AI chat sessions across multiple tools
- Sharing sessions across different accounts, not just one account
- Branching from a message inside a chat session
- Merging multiple sessions into one context
- Routing work across different AI providers and models
- Tracking token/quota usage by provider, account, model, session, and agent
- Running local non-AI capabilities such as shell commands, file reads, API calls, SQLite storage, and search
- Keeping provider integrations plugin-shaped so new AI tools can be added later

API-billed provider routes are intentionally not the default focus. The project currently prioritizes existing subscriptions, free tiers, handoff routes, local bridges, and local compute.

## Repository Layout

```text
apps/
  cli/                 Go CLI entrypoint
  desktop-or-web/      TypeScript web UI served by the Go app
  electron/            Desktop shell that launches the Go backend
internal/
  chatfilter/          Input filter chain for future policy/tool routing
  core/                Shared domain model
  provider/            AI provider integration scaffolding
  storage/sqlitecli/   SQLite schema and store
  toolruntime/         Local tool execution interfaces
  web/                 Local HTTP/streaming server
proto/
  ergo/loom/v1/        Realtime protocol draft
docs/                  Architecture and design notes
```

## Local Data

Installed desktop builds keep local state outside the app bundle:

```text
~/.ergo-loom/local.db
```

Replacing or updating `Ergo Loom.app` should not remove the local database. The app also supports:

- `ERGO_LOOM_DB_PATH` to override the exact SQLite file
- `ERGO_LOOM_DATA_DIR` to override the local data directory
- `ERGO_LOOM_APP_ROOT` for packaged resource lookup

## Development

Requirements:

- Go
- Node.js
- npm
- macOS for the current packaging flow

Install dependencies:

```bash
npm install
```

Run type checks:

```bash
npm run check:desktop
```

Run Go tests:

```bash
GOWORK=off GOCACHE="$PWD/.cache/go-build" go test ./...
```

Start the local web app:

```bash
GOWORK=off GOCACHE="$PWD/.cache/go-build" go run ./apps/cli/cmd/ergo app --addr 127.0.0.1:3763
```

Start the desktop app in development:

```bash
npm run start:desktop
```

## Packaging

Build the web UI, Go CLI, Electron main process, and icons:

```bash
npm run build:desktop
```

Create a macOS app and installable DMG:

```bash
npm run package:mac
```

Outputs are written to:

```text
dist-packaged/mac-arm64/Ergo Loom.app
dist-packaged/Ergo-Loom-0.1.0-arm64.dmg
dist-packaged/latest-mac.yml
```

The GitHub/README brand icon is kept at:

```text
apps/desktop-or-web/static/icon.svg
```

The installable macOS app icon is generated from:

```text
apps/desktop-or-web/static/icon-app.svg
```

into:

```text
build/icon.png
build/icon.icns
```

## Updates

Packaged builds are configured to check GitHub Releases for updates:

```text
Floodnut/ergo-loom
```

To publish an update, create a new release and upload the generated DMG, blockmap, and `latest-mac.yml`. Code signing and notarization are not yet configured and should be added before broader distribution.

## License

MIT. See [LICENSE](LICENSE).
