import { spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";

const root = process.cwd();
const cacheDir = path.join(root, ".cache", "go-build");
fs.mkdirSync(cacheDir, { recursive: true });

const result = spawnSync("go", ["build", "-o", "bin/ergo", "./apps/cli/cmd/ergo"], {
  cwd: root,
  env: {
    ...process.env,
    GOWORK: "off",
    GOCACHE: cacheDir,
  },
  stdio: "inherit",
});

process.exit(result.status ?? 1);
