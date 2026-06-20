import { contextBridge } from "electron";

contextBridge.exposeInMainWorld("ergoLoom", {
  platform: process.platform,
});
