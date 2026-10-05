import { appendFileSync, mkdirSync, readFileSync } from "node:fs";
import { homedir } from "node:os";
import { join } from "node:path";
import OpenAI from "openai";
import { processEvent, type Session, Store } from "./core.ts";
import {
	connectDockerWorkspace as connectWorkspace,
	stopDockerWorkspaceExecutor as stopWorkspaceExecutor,
} from "./docker.ts";
import { webhookServer } from "./http.ts";
import {
	ensureWorkspaceRunning,
	prepareOnDemandWorkspace,
} from "./provision.ts";

process.umask(0o077);
const root = process.env.STATE_DIR ?? join(import.meta.dirname, "../state");
mkdirSync(root, { recursive: true, mode: 0o700 });
const credentialsPath = join(homedir(), ".config/codex-spike/credentials.json");
const credentials = () =>
	JSON.parse(readFileSync(credentialsPath, "utf8")) as Record<string, string>;
const key = credentials().OPENAI_API_KEY;
if (!key) throw Error("Application key missing");
const client = new OpenAI({
	apiKey: key,
	baseURL: "https://api.openai.com/v1",
	maxRetries: 0,
	timeout: 20000,
});
const config = JSON.parse(readFileSync(join(root, "config.json"), "utf8")) as {
	mode: "audit" | "allocate";
	agentId: string;
	workspace: string;
	image?: string;
	provision?: "create";
};
if (!["audit", "allocate"].includes(config.mode) || !config.agentId)
	throw Error("Invalid configuration");
const store = new Store(join(root, "controller.sqlite"));
const redact = (text: string) => {
	for (const value of Object.values(credentials()))
		if (value) text = text.replaceAll(value, "[REDACTED]");
	return text.replace(/sk-[\w-]+/g, "[REDACTED]");
};
const log = (kind: string, data: Record<string, unknown>) => {
	const line = redact(
		JSON.stringify({ time: new Date().toISOString(), kind, ...data }),
	);
	appendFileSync(join(root, "trace.jsonl"), `${line}\n`);
	console.info(line);
};
const server = webhookServer({
	secret: () => credentials().OPENAI_WEBHOOK_SECRET ?? "",
	verify: (b, h, s) => client.webhooks.verifySignature(b, h, s),
	enqueue: (e) => store.enqueue(e),
	log,
});
let busy = false;
let shuttingDown = false;
const tick = async () => {
	if (busy || shuttingDown) return;
	const e = store.next();
	if (!e) return;
	busy = true;
	try {
		await processEvent(e, {
			store,
			...config,
			retrieve: async (id) =>
				(await client.beta.agents.sessions.retrieve(id)) as unknown as Session,
			connect: async (input) => {
				let image = config.image;
				if (config.provision === "create")
					image = await prepareOnDemandWorkspace(
						{ workspace: input.workspace, sessionId: input.sessionId },
						log,
					);
				else await ensureWorkspaceRunning(input.workspace);
				await connectWorkspace(
					{ ...input, executorKey: credentials().OPENAI_EXECUTOR_API_KEY },
					log,
					{ image },
				);
			},
			stop: stopWorkspaceExecutor,
			log,
		});
		store.done(e.id);
	} catch (error) {
		const message = redact(error instanceof Error ? error.message : "unknown");
		store.fail(e.id, message);
		log("worker_error", { event: e.id, error: message });
	} finally {
		busy = false;
	}
};
let timer: ReturnType<typeof setInterval> | undefined;
const port = Number(process.env.PORT ?? 8791);
// Listening exclusively through Coder's app proxy. The only route is signature-gated.
server.listen(port, "127.0.0.1", () => {
	log("receiver_ready", { port, mode: config.mode, agent: config.agentId });
	timer = setInterval(() => void tick(), 2000);
});
async function shutdown() {
	shuttingDown = true;
	clearInterval(timer);
	server.close();
	while (busy) await new Promise((r) => setTimeout(r, 100));
	store.close();
	process.exit(0);
}
process.on("SIGTERM", () => void shutdown());
process.on("SIGINT", () => void shutdown());
