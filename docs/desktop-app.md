# Desktop App Notes

Ergo Loom's desktop shell is a thin installed runtime around the same local Go core.

- The desktop process starts `bin/ergo app --addr 127.0.0.1:<free-port>`.
- The web UI is loaded from that local backend with `?desktop=1`, which enables macOS titlebar-safe layout.
- Local state defaults to `~/.ergo-loom/local.db`.
- App icons are generated from `apps/desktop-or-web/static/icon.svg` into `build/icon.png` and `build/icon.icns`.
- `ERGO_LOOM_DB_PATH` can override the exact SQLite file.
- `ERGO_LOOM_DATA_DIR` can override the local data directory.
- `ERGO_LOOM_APP_ROOT` lets the Go binary find bundled schema/static resources when launched by a desktop shell.
- `npm run package:mac` builds `dist-packaged/mac-arm64/Ergo Loom.app` with the custom icon.

Packaging still needs a follow-up pass for `.app`/`.dmg` creation and resource copying, but the runtime boundary is already:

```text
Electron or native shell
  -> Go backend / core
    -> SQLite at ~/.ergo-loom/local.db
    -> local tool/provider bridges
  -> embedded web UI
```
