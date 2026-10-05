// This process has no Coder API/CLI calls. Only OpenAI can trigger the receiver.

import {
	appendFileSync,
	mkdirSync,
	readFileSync,
	writeFileSync,
} from "node:fs";
import { homedir } from "node:os";
import { join } from "node:path";
import OpenAI from "openai";

process.umask(0o077);
const root = process.env.STATE_DIR ?? join(import.meta.dirname, "../state");
const config = JSON.parse(readFileSync(join(root, "config.json"), "utf8"));
const credentials = JSON.parse(
	readFileSync(join(homedir(), ".config/codex-spike/credentials.json"), "utf8"),
);
const apiKey = credentials.OPENAI_API_KEY;
// Drop unrelated credential references before starting the input-only client.
for (const name of Object.keys(credentials)) delete credentials[name];
for (const name of Object.keys(process.env))
	if (
		name.startsWith("CODER_") ||
		name.includes("EXECUTOR") ||
		name.includes("WEBHOOK")
	)
		delete process.env[name];
const client = new OpenAI({
	apiKey,
	baseURL: "https://api.openai.com/v1",
	maxRetries: 0,
	timeout: 360000,
});
const session = await client.beta.agents.sessions.create({
	agent_id: config.agentId,
	environment: { type: "self_hosted", workspace_directory: "/home/coder/demo" },
});
const evidence = join(root, session.id);
mkdirSync(evidence, { recursive: true });
writeFileSync(
	join(evidence, "session.json"),
	`${JSON.stringify(session, null, 2)}\n`,
);
console.info(
	JSON.stringify({
		kind: "client_session_created",
		session: session.id,
		time: new Date().toISOString(),
	}),
);
const stream = await client.beta.agents.sessions.events.stream(session.id);
let completed = false;
let terminalType = "";
const reading = (async () => {
	for await (const e of stream) {
		appendFileSync(join(evidence, "events.jsonl"), `${JSON.stringify(e)}\n`);
		if (!e.type.endsWith(".delta"))
			console.info(
				JSON.stringify({
					time: new Date().toISOString(),
					type: e.type,
					session: session.id,
				}),
			);
		if (
			[
				"agent.session.turn.completed",
				"agent.session.turn.failed",
				"agent.session.turn.cancelled",
				"agent.session.failed",
			].includes(e.type)
		) {
			completed = true;
			terminalType = e.type;
			stream.controller.abort();
			break;
		}
	}
})();
const timeout = setTimeout(() => {
	stream.controller.abort();
}, 370000);
try {
	await client.beta.agents.sessions.events.create(session.id, {
		events: [
			{
				type: "agent.session.input.message",
				input: [
					{
						role: "user",
						content: [
							{
								type: "input_text",
								text: `Read ${config.provision === "create" ? "worker-marker.txt" : "workspace-marker.txt"} in /home/coder/demo, then write webhook-proof.txt containing that marker and the words OpenAI requested this Coder execution. Report the exact marker and confirm the file was written. Do not access secrets or anything outside this demo directory.`,
							},
						],
					},
				],
			},
		],
	});
	await reading;
	const items = await client.beta.agents.sessions.items.list(session.id, {
		order: "asc",
		limit: 100,
	});
	writeFileSync(
		join(evidence, "items.json"),
		`${JSON.stringify(items.data, null, 2)}\n`,
	);
	console.info(
		JSON.stringify({
			kind: "client_done",
			session: session.id,
			terminal_event: terminalType,
			terminal_event_observed: completed,
		}),
	);
	if (terminalType !== "agent.session.turn.completed") process.exitCode = 1;
} catch (e) {
	console.error(
		JSON.stringify({
			kind: "client_failure",
			session: session.id,
			error: e instanceof Error ? e.message : "unknown",
		}),
	);
	process.exitCode = 1;
} finally {
	clearTimeout(timeout);
	stream.controller.abort();
	await reading.catch(() => {});
	console.info(
		"Session preserved for independent inspection and explicit cleanup.",
	);
}
