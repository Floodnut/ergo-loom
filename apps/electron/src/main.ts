import { app, BrowserWindow, Menu, dialog, ipcMain, shell, type MenuItemConstructorOptions, type OpenDialogOptions } from "electron";
import { spawn, type ChildProcessWithoutNullStreams } from "node:child_process";
import fs from "node:fs";
import http from "node:http";
import { createRequire } from "node:module";
import net from "node:net";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

let mainWindow: BrowserWindow | null = null;
let backend: ChildProcessWithoutNullStreams | null = null;
let handoffServer: http.Server | null = null;
let claudeWorkerWindow: BrowserWindow | null = null;

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const require = createRequire(import.meta.url);
const { autoUpdater } = require("electron-updater") as {
  autoUpdater: {
    autoDownload: boolean;
    autoInstallOnAppQuit: boolean;
    on: (event: string, listener: (...args: any[]) => void) => void;
    checkForUpdatesAndNotify: () => Promise<unknown>;
    quitAndInstall: (isSilent?: boolean, isForceRunAfter?: boolean) => void;
  };
};

function appRoot(): string {
  if (app.isPackaged) {
    return process.resourcesPath;
  }
  return path.resolve(__dirname, "../..");
}

function ergoExecutable(root: string): string {
  const binary = process.platform === "win32" ? "ergo.exe" : "ergo";
  return path.join(root, "bin", binary);
}

function appIcon(root: string, fileName = "icon.png"): string {
  return path.join(root, "build", fileName);
}

async function freePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const server = net.createServer();
    server.on("error", reject);
    server.listen(0, "127.0.0.1", () => {
      const address = server.address();
      if (!address || typeof address === "string") {
        server.close(() => reject(new Error("could not allocate a local port")));
        return;
      }
      server.close(() => resolve(address.port));
    });
  });
}

function backendCommand(root: string): { command: string; args: string[] } {
  const binary = ergoExecutable(root);
  if (fs.existsSync(binary)) {
    return { command: binary, args: [] };
  }
  return { command: "go", args: ["run", "./apps/cli/cmd/ergo"] };
}

function startBackend(root: string, dataDir: string, port: number, handoffBridgeURL: string): void {
  const addr = `127.0.0.1:${port}`;
  const backendCmd = backendCommand(root);
  backend = spawn(backendCmd.command, [...backendCmd.args, "app", "--addr", addr], {
    cwd: root,
    env: {
      ...process.env,
      ERGO_LOOM_APP_ROOT: root,
      ERGO_LOOM_DATA_DIR: dataDir,
      ERGO_LOOM_DESKTOP: "1",
      ERGO_LOOM_HANDOFF_BRIDGE_URL: handoffBridgeURL,
    },
  });

  backend.stdout.on("data", (chunk) => {
    console.log(`[ergo] ${chunk.toString().trimEnd()}`);
  });
  backend.stderr.on("data", (chunk) => {
    console.error(`[ergo] ${chunk.toString().trimEnd()}`);
  });
  backend.on("exit", (code, signal) => {
    if (mainWindow && !mainWindow.isDestroyed()) {
      mainWindow.webContents.send("ergo-backend-exit", { code, signal });
    }
    backend = null;
  });
}

type ClaudeHandoffPayload = {
  sessionId?: string;
  externalThreadId?: string;
  input?: string;
  modelRef?: string;
  modelDisplayName?: string;
  thinkingEffort?: string;
};

async function readRequestBody(request: http.IncomingMessage): Promise<unknown> {
  return new Promise((resolve, reject) => {
    const chunks: Buffer[] = [];
    request.on("data", (chunk) => chunks.push(Buffer.from(chunk)));
    request.on("error", reject);
    request.on("end", () => {
      try {
        const body = Buffer.concat(chunks).toString("utf8");
        resolve(body ? JSON.parse(body) : {});
      } catch (error) {
        reject(error);
      }
    });
  });
}

function writeJSON(response: http.ServerResponse, statusCode: number, payload: unknown): void {
  response.writeHead(statusCode, {
    "content-type": "application/json; charset=utf-8",
    "cache-control": "no-cache",
  });
  response.end(JSON.stringify(payload));
}

async function startHandoffBridge(port: number): Promise<string> {
  const server = http.createServer((request, response) => {
    void (async () => {
      if (request.method !== "POST" || request.url !== "/v1/claude/chat") {
        writeJSON(response, 404, { error: "not found" });
        return;
      }
      const payload = await readRequestBody(request) as ClaudeHandoffPayload;
      const result = await runClaudeBrowserHandoff(payload);
      writeJSON(response, 200, result);
    })().catch((error) => {
      writeJSON(response, 500, { error: error instanceof Error ? error.message : String(error) });
    });
  });

  await new Promise<void>((resolve, reject) => {
    server.on("error", reject);
    server.listen(port, "127.0.0.1", () => resolve());
  });
  handoffServer = server;
  return `http://127.0.0.1:${port}`;
}

async function ensureClaudeWorkerWindow(): Promise<BrowserWindow> {
  if (claudeWorkerWindow && !claudeWorkerWindow.isDestroyed()) {
    return claudeWorkerWindow;
  }
  claudeWorkerWindow = new BrowserWindow({
    width: 1120,
    height: 860,
    title: "Ergo Loom · Claude",
    parent: mainWindow ?? undefined,
    show: false,
    backgroundColor: "#f7f4ec",
    webPreferences: {
      contextIsolation: true,
      nodeIntegration: false,
      partition: "persist:ergo-loom-claude",
    },
  });
  claudeWorkerWindow.on("closed", () => {
    claudeWorkerWindow = null;
  });
  claudeWorkerWindow.webContents.setWindowOpenHandler(({ url: nextURL }) => {
    void claudeWorkerWindow?.loadURL(nextURL);
    return { action: "deny" };
  });
  await claudeWorkerWindow.loadURL("https://claude.ai/new");
  return claudeWorkerWindow;
}

async function showClaudeWorkerWindow(): Promise<void> {
  const worker = await ensureClaudeWorkerWindow();
  if (worker.isMinimized()) {
    worker.restore();
  }
  worker.show();
  worker.focus();
}

async function runClaudeBrowserHandoff(payload: ClaudeHandoffPayload): Promise<{ text: string; externalThreadId: string }> {
  const input = String(payload.input || "").trim();
  if (!input) {
    throw new Error("Claude handoff input is required");
  }
  const worker = await ensureClaudeWorkerWindow();
  await worker.webContents.executeJavaScript("document.readyState", true);

  const prompt = [
    "You are Ergo Loom, a local AI work context manager.",
    "Your product-level identity is Ergo Loom. Do not identify as Claude or Anthropic.",
    payload.modelDisplayName ? `Selected model route: ${payload.modelDisplayName}.` : "",
    "",
    input,
  ].filter(Boolean).join("\n");

  const submitResult = await worker.webContents.executeJavaScript(`
    (() => {
      const prompt = ${JSON.stringify(prompt)};
      const visible = (node) => {
        const rect = node.getBoundingClientRect();
        const style = window.getComputedStyle(node);
        return rect.width > 0 && rect.height > 0 && style.visibility !== "hidden" && style.display !== "none";
      };
      const inputs = [
        ...document.querySelectorAll('textarea'),
        ...document.querySelectorAll('[contenteditable="true"]')
      ].filter(visible);
      const input = inputs[inputs.length - 1];
      if (!input) {
        return { ok: false, reason: "Claude input was not found. The worker window may need sign-in." };
      }
      input.focus();
      if (input.tagName === "TEXTAREA") {
        input.value = prompt;
        input.dispatchEvent(new Event("input", { bubbles: true }));
      } else {
        input.textContent = prompt;
        input.dispatchEvent(new InputEvent("input", { bubbles: true, inputType: "insertText", data: prompt }));
      }
      const buttons = [...document.querySelectorAll('button')].filter(visible);
      const send = buttons.find((button) => /send|submit|arrow|보내|전송/i.test(button.getAttribute("aria-label") || button.textContent || ""))
        || buttons.reverse().find((button) => !button.disabled);
      if (!send) {
        return { ok: false, reason: "Claude send button was not found." };
      }
      send.click();
      return { ok: true };
    })()
  `, true) as { ok: boolean; reason?: string };

  if (!submitResult.ok) {
    worker.show();
    throw new Error(`${submitResult.reason || "Claude web worker could not submit the prompt"} The Claude worker opened inside Ergo Loom; sign in there and retry.`);
  }

  const started = Date.now();
  let lastText = "";
  let stableCount = 0;
  while (Date.now() - started < 120000) {
    await new Promise((resolve) => setTimeout(resolve, 1000));
    const text = await worker.webContents.executeJavaScript(`
      (() => {
        const visible = (node) => {
          const rect = node.getBoundingClientRect();
          const style = window.getComputedStyle(node);
          return rect.width > 0 && rect.height > 0 && style.visibility !== "hidden" && style.display !== "none";
        };
        const selectors = [
          '[data-testid*="message"]',
          '[class*="font-claude-message"]',
          '[class*="prose"]',
          'main article',
          'main [role="listitem"]'
        ];
        const values = [...document.querySelectorAll(selectors.join(','))]
          .filter(visible)
          .map((node) => (node.innerText || node.textContent || "").trim())
          .filter((text) => text.length > 0)
          .filter((text) => !text.includes(${JSON.stringify(input)}));
        return values[values.length - 1] || "";
      })()
    `, true) as string;
    const trimmed = text.trim();
    if (!trimmed) continue;
    if (trimmed === lastText) {
      stableCount += 1;
    } else {
      lastText = trimmed;
      stableCount = 0;
    }
    if (stableCount >= 3) {
      return {
        text: lastText,
        externalThreadId: payload.externalThreadId || `claude-web-${payload.sessionId || Date.now()}`,
      };
    }
  }

  worker.show();
  throw new Error("Claude web worker timed out while waiting for a stable response. The worker window is open inside Ergo Loom for inspection.");
}

async function waitForBackend(url: string): Promise<void> {
  const started = Date.now();
  let lastError: unknown;
  while (Date.now() - started < 8000) {
    try {
      const response = await fetch(url);
      if (response.ok) return;
    } catch (error) {
      lastError = error;
    }
    await new Promise((resolve) => setTimeout(resolve, 120));
  }
  throw new Error(`Ergo Loom backend did not start: ${String(lastError)}`);
}

async function createWindow(): Promise<void> {
  const root = appRoot();
  const dataDir = path.join(os.homedir(), ".ergo-loom");
  const port = await freePort();
  const handoffPort = await freePort();
  const url = `http://127.0.0.1:${port}/?desktop=1`;

  fs.mkdirSync(dataDir, { recursive: true });
  if (process.platform === "darwin" && fs.existsSync(appIcon(root))) {
    app.dock?.setIcon(appIcon(root));
  }
  const handoffBridgeURL = await startHandoffBridge(handoffPort);
  startBackend(root, dataDir, port, handoffBridgeURL);
  await waitForBackend(url);

  mainWindow = new BrowserWindow({
    width: 1440,
    height: 920,
    minWidth: 980,
    minHeight: 680,
    title: "Ergo Loom",
    backgroundColor: "#f7f4ec",
    icon: appIcon(root),
    show: false,
    titleBarStyle: process.platform === "darwin" ? "hiddenInset" : "default",
    webPreferences: {
      contextIsolation: true,
      nodeIntegration: false,
      preload: path.join(__dirname, "preload.js"),
    },
  });

  mainWindow.once("ready-to-show", () => {
    mainWindow?.show();
  });
  mainWindow.webContents.setWindowOpenHandler(({ url: nextURL }) => {
    void shell.openExternal(nextURL);
    return { action: "deny" };
  });
  mainWindow.on("closed", () => {
    mainWindow = null;
  });

  await mainWindow.loadURL(url);
}

function stopBackend(): void {
  if (!backend || backend.killed) return;
  backend.kill();
  backend = null;
}

function stopHandoffBridge(): void {
  handoffServer?.close();
  handoffServer = null;
  if (claudeWorkerWindow && !claudeWorkerWindow.isDestroyed()) {
    claudeWorkerWindow.close();
  }
  claudeWorkerWindow = null;
}

function configureAutoUpdater(): void {
  autoUpdater.autoDownload = true;
  autoUpdater.autoInstallOnAppQuit = true;

  autoUpdater.on("update-available", (info) => {
    mainWindow?.webContents.send("ergo-update-status", {
      status: "available",
      version: info.version,
    });
  });
  autoUpdater.on("update-not-available", (info) => {
    mainWindow?.webContents.send("ergo-update-status", {
      status: "not-available",
      version: info.version,
    });
  });
  autoUpdater.on("error", (error) => {
    mainWindow?.webContents.send("ergo-update-status", {
      status: "error",
      message: error.message,
    });
  });
  autoUpdater.on("update-downloaded", (info) => {
    mainWindow?.webContents.send("ergo-update-status", {
      status: "downloaded",
      version: info.version,
    });
    void dialog.showMessageBox({
      type: "info",
      buttons: ["Install and restart", "Later"],
      defaultId: 0,
      cancelId: 1,
      message: "A new Ergo Loom update is ready.",
      detail: `Version ${info.version} has been downloaded. Your local data in ~/.ergo-loom will be preserved.`,
    }).then((result) => {
      if (result.response === 0) {
        stopBackend();
        autoUpdater.quitAndInstall(false, true);
      }
    });
  });
}

function checkForUpdates(silent = true): void {
  if (!app.isPackaged) {
    if (!silent) {
      void dialog.showMessageBox({
        type: "info",
        message: "Updates are checked only in packaged builds.",
        detail: "Run npm run package:mac and launch the packaged app to test update checks.",
      });
    }
    return;
  }
  autoUpdater.checkForUpdatesAndNotify().catch((error) => {
    if (!silent) {
      void dialog.showErrorBox("Update check failed", error.message);
    }
  });
}

function installAppMenu(): void {
  const template: MenuItemConstructorOptions[] = [
    {
      label: "Ergo Loom",
      submenu: [
        { role: "about" },
        {
          label: "Check for Updates...",
          click: () => checkForUpdates(false),
        },
        { type: "separator" },
        { role: "quit" },
      ],
    },
    {
      label: "Edit",
      submenu: [
        { role: "undo" },
        { role: "redo" },
        { type: "separator" },
        { role: "cut" },
        { role: "copy" },
        { role: "paste" },
        { role: "selectAll" },
      ],
    },
    {
      label: "View",
      submenu: [
        { role: "reload" },
        { role: "toggleDevTools" },
        { type: "separator" },
        { role: "resetZoom" },
        { role: "zoomIn" },
        { role: "zoomOut" },
        { type: "separator" },
        { role: "togglefullscreen" },
      ],
    },
    {
      label: "Window",
      submenu: [{ role: "minimize" }, { role: "close" }],
    },
  ];
  Menu.setApplicationMenu(Menu.buildFromTemplate(template));
}

app.setName("Ergo Loom");

ipcMain.handle("ergo:open-claude-worker", async () => {
  await showClaudeWorkerWindow();
  return { ok: true };
});

ipcMain.handle("ergo:choose-directory", async () => {
  const options: OpenDialogOptions = {
    properties: ["openDirectory", "createDirectory"],
    title: "Choose Ergo Loom project folder",
  };
  const result = mainWindow
    ? await dialog.showOpenDialog(mainWindow, options)
    : await dialog.showOpenDialog(options);
  if (result.canceled || result.filePaths.length === 0) {
    return { canceled: true, path: "" };
  }
  return { canceled: false, path: result.filePaths[0] };
});

app.whenReady().then(() => {
  installAppMenu();
  configureAutoUpdater();
  void createWindow().catch((error) => {
    console.error(error);
    app.quit();
  });
  setTimeout(() => checkForUpdates(true), 2500);
});

app.on("activate", () => {
  if (BrowserWindow.getAllWindows().length === 0) {
    void createWindow();
  }
});

app.on("before-quit", () => {
  stopBackend();
  stopHandoffBridge();
});

app.on("window-all-closed", () => {
  if (process.platform !== "darwin") {
    app.quit();
  }
});
