# Ergo Loom Visual Direction

Ergo Loom should feel like a local craft tool for AI context: warm, precise, quiet, and branch-aware without looking like git.

## Core Motif

- World-tree topology: sessions branch, converge, and keep a living root.
- Loom threads: separate AI contexts are woven into a usable workspace.
- Local craft: macOS-first, installable, restrained, tactile, and trustworthy.

## Icon Concept

The primary mark is a rounded macOS-ready tile containing a woven world-tree symbol.

- Center trunk: the current working context.
- Upper split: branching sessions or agents.
- Lower merge: context recombination.
- Horizontal threads: loom structure, kept subtle so the mark does not become a textile illustration.

Source asset: `apps/desktop-or-web/static/icon.svg`

## UI Tone

- Palette: warm paper, flax, cedar, muted moss, ink.
- Surfaces: low-contrast panels with almost no hard borders.
- Texture: very light woven linear gradients only on chrome areas.
- Typography: compact task-app scale, with calm hierarchy rather than big marketing type.
- Interaction: folding, branching, and usage states should feel quick and physical.

## macOS Packaging Notes

- Keep the SVG source as the canonical icon.
- Generate `.icns` from the same symbol when Electron/Tauri packaging starts.
- The icon should remain legible at 16px, so avoid adding text or thin decorative strands.
