import { spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import sharp from "sharp";

const root = process.cwd();
const source = path.join(root, "apps", "desktop-or-web", "static", "icon-app.svg");
const buildDir = path.join(root, "build");
const assetDir = path.join(buildDir, "assets");
const iconsetDir = path.join(buildDir, "icon.iconset");
const rendered = path.join(assetDir, "icon.svg.png");
const png = path.join(buildDir, "icon.png");
const icns = path.join(buildDir, "icon.icns");

fs.mkdirSync(assetDir, { recursive: true });
fs.rmSync(rendered, { force: true });
fs.rmSync(png, { force: true });
fs.rmSync(icns, { force: true });
fs.rmSync(iconsetDir, { force: true, recursive: true });
fs.mkdirSync(iconsetDir, { recursive: true });

function run(command, args) {
  const result = spawnSync(command, args, { cwd: root, stdio: "inherit" });
  if (result.status !== 0) {
    process.exit(result.status ?? 1);
  }
}

await sharp(source).resize(1024, 1024).png().toFile(rendered);
fs.copyFileSync(rendered, png);

const sizes = [
  ["16", "16", "icon_16x16.png"],
  ["32", "32", "icon_16x16@2x.png"],
  ["32", "32", "icon_32x32.png"],
  ["64", "64", "icon_32x32@2x.png"],
  ["128", "128", "icon_128x128.png"],
  ["256", "256", "icon_128x128@2x.png"],
  ["256", "256", "icon_256x256.png"],
  ["512", "512", "icon_256x256@2x.png"],
  ["512", "512", "icon_512x512.png"],
];

for (const [height, width, fileName] of sizes) {
  run("sips", ["-z", height, width, rendered, "--out", path.join(iconsetDir, fileName)]);
}
fs.copyFileSync(rendered, path.join(iconsetDir, "icon_512x512@2x.png"));

const icnsChunks = [
  ["icp4", "icon_16x16.png"],
  ["icp5", "icon_32x32.png"],
  ["icp6", "icon_32x32@2x.png"],
  ["ic07", "icon_128x128.png"],
  ["ic08", "icon_256x256.png"],
  ["ic09", "icon_512x512.png"],
  ["ic10", "icon_512x512@2x.png"],
];

const chunks = icnsChunks.map(([type, fileName]) => {
  const data = fs.readFileSync(path.join(iconsetDir, fileName));
  const header = Buffer.alloc(8);
  header.write(type, 0, 4, "ascii");
  header.writeUInt32BE(data.length + 8, 4);
  return Buffer.concat([header, data]);
});
const totalLength = 8 + chunks.reduce((sum, chunk) => sum + chunk.length, 0);
const header = Buffer.alloc(8);
header.write("icns", 0, 4, "ascii");
header.writeUInt32BE(totalLength, 4);
fs.writeFileSync(icns, Buffer.concat([header, ...chunks]));
