// Fails before install or build when node or pnpm differ from package.json.
import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

export function toolchainProblems(manifest, { node, pnpm }) {
  const problems = [];
  const minimum = /^>=(\d+)$/.exec(manifest.engines?.node ?? "");
  if (!minimum) {
    problems.push(`package.json engines.node must look like >=22, is ${manifest.engines?.node}`);
  } else if (Number(node.replace(/^v/, "").split(".")[0]) < Number(minimum[1])) {
    problems.push(`node ${minimum[1]} or later is required, found ${node}`);
  }
  const pinned = /^pnpm@(\d+\.\d+\.\d+)$/.exec(manifest.packageManager ?? "");
  if (!pinned) {
    problems.push(`package.json packageManager must be pnpm@X.Y.Z, is ${manifest.packageManager}`);
  } else if (pnpm.trim() !== pinned[1]) {
    problems.push(`pnpm ${pinned[1]} is required, found ${pnpm.trim()}`);
  }
  return problems;
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const appDir = fileURLToPath(new URL("..", import.meta.url));
  const manifest = JSON.parse(readFileSync(new URL("../package.json", import.meta.url), "utf8"));
  let pnpm;
  try {
    pnpm = execFileSync("pnpm", ["--version"], { cwd: appDir, encoding: "utf8" });
  } catch {
    pnpm = "none";
  }
  const problems = toolchainProblems(manifest, { node: process.version, pnpm });
  if (problems.length) {
    console.error(`offlinedocs needs a different toolchain:\n  ${problems.join("\n  ")}`);
    process.exit(1);
  }
}
