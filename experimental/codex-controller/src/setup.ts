// Explicit administrative operations. Never called by the submitter or webhook worker.

import {
	existsSync,
	mkdirSync,
	readFileSync,
	renameSync,
	writeFileSync,
} from "node:fs";
import { homedir } from "node:os";
import { join } from "node:path";
import OpenAI from "openai";
import { validateWorkspace } from "./coder.ts";

process.umask(0o077);
const root = process.env.STATE_DIR ?? join(import.meta.dirname, "../state");
mkdirSync(root, { recursive: true, mode: 0o700 });
const credentialPath = join(homedir(), ".config/codex-spike/credentials.json");
const credentials = JSON.parse(readFileSync(credentialPath, "utf8"));
const client = new OpenAI({
	apiKey: credentials.OPENAI_API_KEY,
	baseURL: "https://api.openai.com/v1",
	maxRetries: 0,
});
const action = process.argv[2];
if (action === "agent") {
	const path = join(root, "config.json");
	if (existsSync(path))
		throw Error(
			"Configuration already exists; refusing to create another agent",
		);
	const workspace = validateWorkspace(process.env.WORKSPACE ?? "");
	const agent = await client.beta.agents.create({
		name: "Coder webhook proof of concept",
		model: "gpt-6-astra",
		instructions:
			"Work only in /home/coder/demo. Never access credentials, environment variables, /proc, or files outside that directory. Do not use the network or install packages. Report actual command results.",
	});
	writeFileSync(
		path,
		`${JSON.stringify(
			{ mode: "audit", agentId: agent.id, workspace, provision: "create" },
			null,
			2,
		)}\n`,
	);
	console.info(JSON.stringify({ agentId: agent.id, mode: "audit" }));
} else if (action === "register") {
	if (process.env.APPROVE_PUBLIC_WEBHOOK !== "yes")
		throw Error("Explicit public webhook authorization is required");
	const path = join(root, "endpoint.json");
	if (existsSync(path))
		throw Error(
			"Endpoint record already exists; reconcile before creating another",
		);
	const url = process.env.WEBHOOK_URL;
	if (!url || new URL(url).protocol !== "https:")
		throw Error("HTTPS WEBHOOK_URL required");
	// The SDK's endpoint-management enum omits the documented Agents events.
	// Send the documented names explicitly; a live API rejection must remain visible.
	const endpoint = await client.post<{ id: string; signing_secret: string }>(
		"/webhook_endpoints",
		{
			body: {
				name: "Coder Agents generated compute proof",
				url,
				event_types: ["agent.session.action_required", "agent.session.failed"],
			},
		},
	);
	// Store the endpoint ID before its secret, so interrupted setup can clean up.
	writeFileSync(path, `${JSON.stringify({ id: endpoint.id, url }, null, 2)}\n`);
	credentials.OPENAI_WEBHOOK_SECRET = endpoint.signing_secret;
	const tmp = `${credentialPath}.new`;
	writeFileSync(tmp, `${JSON.stringify(credentials)}\n`, {
		mode: 0o600,
		flag: "wx",
	});
	renameSync(tmp, credentialPath);
	console.info(JSON.stringify({ endpointId: endpoint.id, secretSaved: true }));
} else if (action === "unregister") {
	const { id } = JSON.parse(readFileSync(join(root, "endpoint.json"), "utf8"));
	await client.webhooks.delete(id);
	writeFileSync(
		join(root, "endpoint-deleted.json"),
		`${JSON.stringify({ id, deleted: true })}\n`,
	);
	console.info("Webhook endpoint deleted");
} else {
	throw Error("Expected agent, register, or unregister");
}
