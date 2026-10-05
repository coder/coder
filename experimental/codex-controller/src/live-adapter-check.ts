// Local trigger, real OpenAI session and Coder executor. Not webhook-delivery proof.

import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import {
	appendFileSync,
	mkdirSync,
	readFileSync,
	writeFileSync,
} from "node:fs";
import { homedir } from "node:os";
import { join } from "node:path";
import OpenAI from "openai";
import { coderEnv } from "./coder.ts";
import {
	connectDockerWorkspace as connectWorkspace,
	stopDockerWorkspaceExecutor as stopWorkspaceExecutor,
	dockerWorkspaceExecutorStatus as workspaceExecutorStatus,
} from "./docker.ts";

process.umask(0o077);
const root = join(import.meta.dirname, `../state/local-check-${Date.now()}`);
mkdirSync(root, { recursive: true });
const keys = JSON.parse(
	readFileSync(join(homedir(), ".config/codex-spike/credentials.json"), "utf8"),
);
const client = new OpenAI({
	apiKey: keys.OPENAI_API_KEY,
	baseURL: "https://api.openai.com/v1",
	maxRetries: 0,
	timeout: 360000,
});
const workspace = process.env.WORKSPACE;
if (!workspace)
	throw Error("Set WORKSPACE to a dedicated prepared owner/name workspace");
const log = (kind: string, data: Record<string, unknown> = {}) => {
	let text = JSON.stringify({ time: new Date().toISOString(), kind, ...data });
	for (const v of Object.values(keys))
		if (typeof v === "string") text = text.replaceAll(v, "[REDACTED]");
	appendFileSync(join(root, "trace.jsonl"), `${text}\n`);
	console.info(text);
};
const remote = (code: string) =>
	execFileSync(
		"coder",
		[
			"ssh",
			"--disable-autostart",
			workspace,
			"--",
			`python3 -c "import base64;exec(base64.b64decode('${Buffer.from(code).toString("base64")}'))"`,
		],
		{ env: coderEnv(), encoding: "utf8", timeout: 30000 },
	);
const session = await client.beta.agents.sessions.create({
	agent: {
		model: "gpt-6-astra",
		reasoning: { effort: "low" },
		instructions:
			"Work only in /home/coder/demo. Do not read credentials, environment variables, or files outside the demo. Do not use the network. Follow task instructions exactly.",
	},
	environment: { type: "self_hosted", workspace_directory: "/home/coder/demo" },
});
writeFileSync(join(root, "session.json"), JSON.stringify(session, null, 2));
const env = session.environment as { id: string; remote_url: string };
const input = {
	workspace,
	sessionId: session.id,
	environmentId: env.id,
	remoteUrl: env.remote_url,
	executorKey: keys.OPENAI_EXECUTOR_API_KEY,
};
const stream = await client.beta.agents.sessions.events.stream(session.id);
const events: any[] = [];
const reader = (async () => {
	for await (const e of stream) {
		events.push(e);
		appendFileSync(join(root, "events.jsonl"), `${JSON.stringify(e)}\n`);
	}
})();
const wait = async (predicate: () => boolean, timeout = 60000) => {
	const end = Date.now() + timeout;
	while (!predicate()) {
		if (Date.now() > end) throw Error("Timed out waiting for live check");
		await new Promise((r) => setTimeout(r, 250));
	}
};
const send = async (text: string) => {
	const mark = events.length;
	await client.beta.agents.sessions.events.create(session.id, {
		events: [
			{
				type: "agent.session.input.message",
				input: [{ role: "user", content: [{ type: "input_text", text }] }],
			},
		],
	});
	return mark;
};
const finish = async (mark: number, label: string) => {
	await wait(
		() =>
			events
				.slice(mark)
				.some((e) =>
					[
						"agent.session.turn.completed",
						"agent.session.turn.failed",
						"agent.session.turn.cancelled",
					].includes(e.type),
				),
		120000,
	);
	const terminal = events
		.slice(mark)
		.find((e) =>
			[
				"agent.session.turn.completed",
				"agent.session.turn.failed",
				"agent.session.turn.cancelled",
			].includes(e.type),
		);
	const items = await client.beta.agents.sessions.items.list(session.id, {
		limit: 100,
		order: "desc",
	});
	writeFileSync(
		join(root, `${label}.json`),
		JSON.stringify(items.data, null, 2),
	);
	log(label, { terminal: terminal.type });
	return terminal.type;
};
let owned = false;
try {
	owned = true;
	await connectWorkspace(input, log);
	await wait(() =>
		events.some((e) => e.type === "agent.session.environment.connected"),
	);
	await connectWorkspace(input, log);
	log("duplicate_connect", {
		status: await workspaceExecutorStatus(workspace),
	});
	await assert.rejects(stopWorkspaceExecutor(workspace, "sess_not_the_owner"));
	log("wrong_owner_rejected");
	let m = await send(
		'Read workspace-marker.txt. Write local-adapter-proof.txt containing its exact contents. Report the marker and run python3 -c "print(2+3)".',
	);
	assert.equal(await finish(m, "execution"), "agent.session.turn.completed");
	log("independent_file_check", {
		output: remote(
			"from pathlib import Path; p=Path('/home/coder/demo'); assert (p/'workspace-marker.txt').read_text().strip()==(p/'local-adapter-proof.txt').read_text().strip(); print('Marker matched')",
		),
	});
	m = await send(
		"Run python3 /home/coder/demo/stop-check.py and wait. Do not retry if interrupted.",
	);
	await wait(
		() =>
			remote(
				"from pathlib import Path; print(Path('/home/coder/demo/stop-check.started').exists())",
			).trim() === "True",
	);
	await client.beta.agents.sessions.events.create(session.id, {
		events: [{ type: "agent.session.input.cancel" }],
	});
	await stopWorkspaceExecutor(workspace, session.id);
	assert.equal((await workspaceExecutorStatus(workspace)).alive, false);
	log("local_stop_verified");
	log("independent_process_check", {
		output: remote(
			"from pathlib import Path; found=[]\nfor f in Path('/proc').glob('[0-9]*/cmdline'):\n try:\n  a=f.read_bytes().split(b'\\0')\n  if len(a)>1 and a[1]==b'/home/coder/demo/stop-check.py': found.append(f.parent.name)\n except OSError: pass\nassert not found,found\nprint('No remaining controlled command')",
		),
	});
	await finish(m, "cancel-and-local-stop");
	await connectWorkspace(input, log);
	m = await send(
		"Read local-adapter-proof.txt and report its exact contents. This is the follow-up after a hard cancellation. Do not rerun stop-check.py.",
	);
	assert.equal(
		await finish(m, "follow-up-after-stop"),
		"agent.session.turn.completed",
	);
	log("local_integration_passed", {
		note: "This does not prove genuine webhook delivery.",
	});
} catch (e) {
	log("check_failed", { error: e instanceof Error ? e.message : String(e) });
	process.exitCode = 1;
} finally {
	if (owned) {
		try {
			await stopWorkspaceExecutor(workspace, session.id);
			log("executor_cleanup_verified");
		} catch (e) {
			log("cleanup_error", { error: String(e) });
			process.exitCode = 1;
		}
	}
	const result = await client.beta.agents.sessions.delete(session.id);
	writeFileSync(join(root, "deleted.json"), JSON.stringify(result));
	stream.controller.abort();
	await reader.catch(() => {});
	log("session_deleted", { session: session.id });
}
