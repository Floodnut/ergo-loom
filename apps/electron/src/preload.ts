import { contextBridge, ipcRenderer } from "electron";

contextBridge.exposeInMainWorld("ergoLoom", {
  platform: process.platform,
  handoffBridge: true,
  openClaudeWorker: () => ipcRenderer.invoke("ergo:open-claude-worker"),
  chooseDirectory: () => ipcRenderer.invoke("ergo:choose-directory"),
  toggleMaximize: () => ipcRenderer.invoke("ergo:toggle-maximize"),
});
