<p align="center">
  <img src="apps/desktop-or-web/static/icon.svg" alt="Ergo Loom" width="96" height="96">
</p>

# Ergo Loom

Ergo Loom is a local workspace for AI work context.

It helps you keep chat sessions, task history, provider choices, tool approvals, local files, and project context together in one installed app. Instead of treating AI conversations as disposable chat logs, Ergo Loom treats them as working context that can be organized by project, reused, branched, merged, and routed across different AI providers.

Ergo Loom is designed for people who use more than one AI tool, more than one account, or more than one model while working on the same project.

## What Ergo Loom Does

- Organizes chats under local projects
- Keeps project context and local paths attached to chats
- Lets you choose which AI providers are available for a project
- Supports provider/model routing for tools such as Codex/ChatGPT, Claude, VS Code Copilot, Gemini, and local models
- Tracks local provider usage and account labels
- Shows tool activity, approvals, terminal activity, and file context beside the chat
- Supports moderator provider selection for multi-provider chats
- Stores local data outside the app bundle so updates do not erase your workspace

Ergo Loom focuses on local-first usage. API-billed routes are not the default installation path.

## Installation

Install Ergo Loom with the official macOS `.pkg` installer only.

1. Download the latest `Ergo Loom.pkg` from the project release page.
2. Open the package installer.
3. Follow the macOS installation steps.
4. Launch **Ergo Loom** from Applications.

Do not install Ergo Loom by manually copying app bundles from build folders. The package installer is the supported installation method because it installs the desktop app and companion local runtime together.

## Minimum Requirements

- macOS 13 Ventura or later
- Apple Silicon Mac is recommended
- Local disk space for the app and workspace database
- Existing accounts or local tools for the providers you want to use

Some provider routes may require their own desktop app, CLI, browser sign-in, subscription, or free-tier account. Ergo Loom does not replace provider licensing.

## First Run

When you open Ergo Loom, it creates a local workspace in:

```text
~/.ergo-loom
```

The default local database is:

```text
~/.ergo-loom/local.db
```

Updating Ergo Loom should preserve this directory.

## Basic Usage

1. Open Ergo Loom.
2. Create or select a project from the left sidebar.
3. Choose the project path for local context.
4. Select the AI providers you want available in the chat.
5. Choose a moderator provider mode if you plan to use multiple providers.
6. Start chatting from the composer at the bottom.

Chats are always stored under a project. If you do not create a project, Ergo Loom uses the default project.

## Projects

A project is the local boundary for chats, provider choices, paths, files, and future context merging.

Changing the local path means you are effectively moving to another project context. Ergo Loom treats project paths as project-level identity, not as a temporary chat option.

## AI Providers

Ergo Loom separates two ideas:

- **Provider**: the account or tool family, such as Codex/ChatGPT, Claude, Copilot, Gemini, or Ollama
- **Model**: the model available through that provider

A chat can be prepared with multiple providers. The visible model list is limited to models available through the providers selected for that chat.

## Moderator Provider

For multi-provider chats, Ergo Loom can mark one provider as the moderator.

The moderator is not an orchestrator. It is a lightweight coordination role used to keep multi-provider chat flow, ordering, and fallback behavior understandable.

Projects can use:

- **Auto**: use the registered provider order
- **Manual**: choose primary and secondary moderator providers

If a project does not define its own moderator preference, Ergo Loom follows the global preference.

## Tool Approvals

Some actions, such as shell commands or local tool calls, may require explicit approval.

When approval is requested, you can:

- Approve the requested action
- Approve similar actions for the current session
- Reject the action
- Send an alternative instruction into the chat
- Type your own replacement instruction

This keeps local execution visible and user-controlled.

## Local Data And Updates

Ergo Loom stores user data in `~/.ergo-loom`, outside the installed app.

That means app updates should replace the app itself while keeping:

- local database
- project records
- chat history
- provider account labels
- local workspace metadata

Back up `~/.ergo-loom` if you want to preserve your workspace manually.

## License

Ergo Loom is released under the MIT License. See [LICENSE](LICENSE).
