import { createServer } from "node:http";
import { type Event, parseEvent } from "./core.ts";
export const MAX_BODY_BYTES = 65536;
export function webhookServer(options: {
	secret: () => string;
	verify: (
		body: string,
		headers: Record<string, string>,
		secret: string,
	) => Promise<unknown>;
	enqueue: (event: Event) => boolean;
	log: (kind: string, data: Record<string, unknown>) => void;
}) {
	let windowStart = Date.now();
	let requests = 0;
	const server = createServer(async (req, res) => {
		if (Date.now() - windowStart > 60000) {
			windowStart = Date.now();
			requests = 0;
		}
		if (++requests > 120) {
			res.writeHead(429).end();
			return;
		}
		res.setHeader("Cache-Control", "no-store");
		if (req.url !== "/webhooks/openai") {
			res.writeHead(404).end();
			return;
		}
		if (req.method !== "POST") {
			res.writeHead(405).end();
			return;
		}
		const secret = options.secret();
		if (!secret) {
			res.writeHead(503).end();
			return;
		}
		// Reject a declared oversized body without reading it; close the connection so the rest is discarded.
		const declared = Number(req.headers["content-length"]);
		if (declared > MAX_BODY_BYTES) {
			res.writeHead(413, { connection: "close" }).end();
			return;
		}
		try {
			let bytes = 0;
			const chunks: Buffer[] = [];
			for await (const chunk of req) {
				bytes += chunk.length;
				if (bytes > MAX_BODY_BYTES) {
					res.writeHead(413, { connection: "close" }).end(() => req.destroy());
					return;
				}
				chunks.push(chunk);
			}
			const body = Buffer.concat(chunks).toString("utf8");
			const headers: Record<string, string> = {};
			for (const [k, v] of Object.entries(req.headers)) {
				if (typeof v === "string") headers[k] = v;
			}
			try {
				await options.verify(body, headers, secret);
			} catch {
				res.writeHead(401).end();
				return;
			}
			let parsed: unknown;
			try {
				parsed = JSON.parse(body);
			} catch {
				res.writeHead(400).end();
				return;
			}
			const event = parseEvent(parsed);
			if (!event) {
				options.log("rejected_delivery", { reason: "invalid_shape" });
				res.writeHead(400).end();
				return;
			}
			if (
				!["agent.session.action_required", "agent.session.failed"].includes(
					event.type,
				)
			) {
				res.writeHead(204).end();
				return;
			}
			const added = options.enqueue(event);
			options.log("signed_delivery", {
				event: event.id,
				session: event.data.id,
				type: event.type,
				added,
			});
			res.writeHead(202).end();
		} catch {
			if (!res.headersSent) res.writeHead(500).end();
		}
	});
	server.requestTimeout = 10000;
	server.headersTimeout = 10000;
	server.maxConnections = 20;
	return server;
}
