// Explicit cleanup; never exposed on the public webhook route.

import { execFile } from "node:child_process";
import { readFileSync } from "node:fs";
import { homedir } from "node:os";
import { join } from "node:path";
import { promisify } from "node:util";
import OpenAI from "openai";
import { coderEnv } from "./coder.ts";
import { Store } from "./core.ts";
import { stopDockerWorkspaceExecutor } from "./docker.ts";

process.umask(0o077);
const id = process.argv[2];
if (!id || !/^sess_[A-Za-z0-9_-]+$/.test(id))
	throw Error("Pass the session ID to end");
const root = process.env.STATE_DIR ?? join(import.meta.dirname, "../state");
const store = new Store(join(root, "controller.sqlite"));
try {
	const allocation = store.allocation(id);
	if (!allocation) throw Error("No owned allocation for this session");
	// Run with the receiver stopped so it cannot race cleanup. Tombstone first prevents later wakeups.
	store.end(id);
	await stopDockerWorkspaceExecutor(allocation.workspace, id);
	const keys = JSON.parse(
		readFileSync(
			join(homedir(), ".config/codex-spike/credentials.json"),
			"utf8",
		),
	);
	const client = new OpenAI({
		apiKey: keys.OPENAI_API_KEY,
		baseURL: "https://api.openai.com/v1",
		maxRetries: 0,
	});
	try {
		await client.beta.agents.sessions.delete(id);
	} catch (e) {
		if ((e as { status?: number }).status !== 404) throw e;
	}
	await promisify(execFile)("coder", ["stop", allocation.workspace, "--yes"], {
		env: coderEnv(),
		timeout: 180000,
		maxBuffer: 1024 * 1024,
	});
	console.info(
		JSON.stringify({
			session: id,
			workspace: allocation.workspace,
			status: "ended",
			filesPreserved: true,
		}),
	);
} finally {
	store.close();
}
