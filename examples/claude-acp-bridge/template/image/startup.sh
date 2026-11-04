#!/bin/sh
set -eu
umask 077

# Native CLIs are pinned in the image. The init script has already downloaded
# this deployment's Coder binary into BINARY_DIR, which is on PATH.
# Configure only prepares project/auth files and records the literal prompt.
# The agent restores sessions and starts the initial job after scripts finish.
# Codex 0.164.0 reads this endpoint from config.toml, not OPENAI_BASE_URL.
if [ -n "${OPENAI_BASE_URL:-}" ]; then
  node <<'JS'
const fs = require("node:fs");
const path = require("node:path");
const root = process.env.CODEX_HOME || path.join(process.env.HOME, ".codex");
const endpoint = new URL(process.env.OPENAI_BASE_URL);
if (endpoint.protocol !== "https:" || endpoint.username || endpoint.password || endpoint.search || endpoint.hash) {
  throw new Error("Codex gateway must be HTTPS without embedded credentials");
}
fs.mkdirSync(root, { recursive: true, mode: 0o700 });
const dir = fs.lstatSync(root);
if (!dir.isDirectory() || dir.uid !== process.getuid() || (dir.mode & 0o077)) {
  throw new Error("Codex home must be a private directory owned by the workspace user");
}
const config = path.join(root, "config.toml");
let content = "";
let exists = false;
try {
  const fd = fs.openSync(config, fs.constants.O_RDONLY | fs.constants.O_NOFOLLOW | fs.constants.O_NONBLOCK);
  try {
    const info = fs.fstatSync(fd);
    if (!info.isFile() || info.uid !== process.getuid() || (info.mode & 0o077) || info.size > 1048576) {
      throw new Error("Existing Codex configuration is not private; it was not changed");
    }
    content = fs.readFileSync(fd, "utf8");
    exists = true;
  } finally {
    fs.closeSync(fd);
  }
} catch (error) {
  if (error.code !== "ENOENT") throw error;
}
const setting = "openai_base_url = " + JSON.stringify(process.env.OPENAI_BASE_URL);
if (/^\s*openai_base_url\s*=/m.test(content)) {
  if (!content.split("\n").some(line => line.trim() === setting)) {
    throw new Error("Existing Codex gateway differs; reconcile config.toml before setup");
  }
} else {
  if (!exists && process.env.CODER_NATIVE_JOB_HARNESS === "codex" && process.env.CODER_NATIVE_JOB_MODEL) {
    content = "model = " + JSON.stringify(process.env.CODER_NATIVE_JOB_MODEL) + "\n";
  }
  const temporary = config + "." + process.pid + ".tmp";
  fs.writeFileSync(temporary, setting + "\n" + content, { mode: 0o600, flag: "wx" });
  fs.renameSync(temporary, config);
}
// Prepare terminal Codex too, even when another harness owns the initial job.
const authFile = path.join(root, "auth.json");
try {
  const auth = fs.lstatSync(authFile);
  if (!auth.isFile() || auth.uid !== process.getuid() || (auth.mode & 0o077)) {
    throw new Error("Existing Codex authentication is not private; it was not changed");
  }
} catch (error) {
  if (error.code !== "ENOENT") throw error;
  const mount = "/run/secrets/claude-auth.json";
  const info = fs.lstatSync(mount);
  if (!info.isFile() || info.uid !== process.getuid() || (info.mode & 0o077) || info.size > 1048576) {
    throw new Error("Mounted Codex authentication is not private");
  }
  const key = JSON.parse(fs.readFileSync(mount, "utf8")).OPENAI_API_KEY;
  if (typeof key !== "string" || !key || /\s|\0/.test(key)) {
    throw new Error("Mounted authentication requires OPENAI_API_KEY");
  }
  const result = require("node:child_process").spawnSync("codex", ["login", "--with-api-key"], {
    env: { ...process.env, CODEX_HOME: root },
    input: key + "\n",
    stdio: ["pipe", "ignore", "ignore"],
    timeout: 15000,
  });
  if (result.status !== 0) throw new Error("Codex native login failed");
}
JS
fi
exec coder native-agent configure --setup
