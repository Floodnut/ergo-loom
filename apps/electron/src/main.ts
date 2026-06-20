import { app, BrowserWindow, shell } from "electron";
import { spawn, type ChildProcessWithoutNullStreams } from "node:child_process";
import fs from "node:fs";
import net from "node:net";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

let mainWindow: BrowserWindow | null = null;
let backend: ChildProcessWithoutNullStreams | null = null;

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

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

function startBackend(root: string, dataDir: string, port: number): void {
  const addr = `127.0.0.1:${port}`;
  const backendCmd = backendCommand(root);
  backend = spawn(backendCmd.command, [...backendCmd.args, "app", "--addr", addr], {
    cwd: root,
    env: {
      ...process.env,
      ERGO_LOOM_APP_ROOT: root,
      ERGO_LOOM_DATA_DIR: dataDir,
      ERGO_LOOM_DESKTOP: "1",
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
  const url = `http://127.0.0.1:${port}/?desktop=1`;

  fs.mkdirSync(dataDir, { recursive: true });
  if (process.platform === "darwin" && fs.existsSync(appIcon(root))) {
    app.dock?.setIcon(appIcon(root));
  }
  startBackend(root, dataDir, port);
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

app.setName("Ergo Loom");

app.whenReady().then(() => {
  void createWindow().catch((error) => {
    console.error(error);
    app.quit();
  });
});

app.on("activate", () => {
  if (BrowserWindow.getAllWindows().length === 0) {
    void createWindow();
  }
});

app.on("before-quit", stopBackend);

app.on("window-all-closed", () => {
  if (process.platform !== "darwin") {
    app.quit();
  }
});
