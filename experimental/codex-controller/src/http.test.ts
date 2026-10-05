import assert from "node:assert/strict";
import { createHmac } from "node:crypto";
import { once } from "node:events";
import { request } from "node:http";
import { test } from "node:test";
import OpenAI from "openai";
import type { Event } from "./core.ts";
import { MAX_BODY_BYTES, webhookServer } from "./http.ts";

test("only correctly signed fresh payloads can reach queue; no admin routes", async () => {
	const key = Buffer.from("unit-test-secret-not-a-production-key");
	const secret = `whsec_${key.toString("base64")}`;
	const client = new OpenAI({ apiKey: "test" });
	let queued = 0;
	const server = webhookServer({
		secret: () => secret,
		verify: (b, h, s) => client.webhooks.verifySignature(b, h, s),
		enqueue: () => {
			queued++;
			return true;
		},
		log: () => {},
	});
	server.listen(0, "127.0.0.1");
	await once(server, "listening");
	const port = (server.address() as { port: number }).port;
	const url = `http://127.0.0.1:${port}/webhooks/openai`;
	const body = JSON.stringify({
		id: "evt_signed",
		type: "agent.session.action_required",
		data: {
			id: "sess_demo",
			required_action: { type: "environment_connection" },
		},
	});
	const sign = (timestamp: number) => ({
		"webhook-id": "wh_test",
		"webhook-timestamp": String(timestamp),
		"webhook-signature":
			"v1," +
			createHmac("sha256", key)
				.update(`wh_test.${timestamp}.${body}`)
				.digest("base64"),
		"content-type": "application/json",
	});
	try {
		assert.equal((await fetch(url, { method: "POST", body })).status, 401);
		assert.equal(
			(
				await fetch(url, {
					method: "POST",
					body,
					headers: sign(Math.floor(Date.now() / 1000) - 1000),
				})
			).status,
			401,
		);
		assert.equal(
			(
				await fetch(url, {
					method: "POST",
					body,
					headers: sign(Math.floor(Date.now() / 1000)),
				})
			).status,
			202,
		);
		assert.equal(queued, 1);
		assert.equal(
			(await fetch(url.replace("/webhooks/openai", "/admin"))).status,
			404,
		);
	} finally {
		server.close();
		await once(server, "close");
	}
});

async function harness(
	verify: (b: string) => Promise<unknown> = async () => {},
) {
	const queued: Event[] = [];
	const logs: string[] = [];
	const server = webhookServer({
		secret: () => "whsec_test",
		verify: (b) => verify(b),
		enqueue: (e) => {
			queued.push(e);
			return true;
		},
		log: (k) => {
			logs.push(k);
		},
	});
	server.listen(0, "127.0.0.1");
	await once(server, "listening");
	const url = `http://127.0.0.1:${(server.address() as { port: number }).port}/webhooks/openai`;
	return {
		server,
		url,
		queued,
		logs,
		post: (body: string) =>
			fetch(url, {
				method: "POST",
				body,
				headers: { "content-type": "application/json" },
			}),
		async close() {
			server.close();
			server.closeAllConnections();
			await once(server, "close");
		},
	};
}

test("verified but malformed payloads are rejected before enqueue", async () => {
	const h = await harness();
	try {
		for (const body of [
			"",
			"not json",
			'{"id":',
			"null",
			"[]",
			'"evt"',
			"{}",
			'{"id":"e","type":"agent.session.failed"}',
			'{"id":"e","type":"agent.session.failed","data":null}',
			'{"id":1,"type":"agent.session.failed","data":{"id":"s"}}',
			'{"id":"e","type":"agent.session.failed","data":{"id":""}}',
			JSON.stringify({
				id: "e".repeat(300),
				type: "agent.session.failed",
				data: { id: "s" },
			}),
			'{"id":"e","type":"agent.session.action_required","data":{"id":"s","required_action":"environment_connection"}}',
		])
			assert.equal((await h.post(body)).status, 400, body);
		assert.equal(h.queued.length, 0);
		assert.equal(
			(await h.post('{"id":"e","type":"response.completed","data":{"id":"r"}}'))
				.status,
			204,
		);
		assert.equal(h.queued.length, 0);
		assert.equal(
			(
				await h.post(
					JSON.stringify({
						id: "e",
						type: "agent.session.failed",
						object: "event",
						data: { id: "s", secret: "x" },
					}),
				)
			).status,
			202,
		);
		assert.deepEqual(
			h.queued,
			[{ id: "e", type: "agent.session.failed", data: { id: "s" } }],
			"only validated fields are enqueued",
		);
	} finally {
		await h.close();
	}
});

test("signature is checked before JSON parsing", async () => {
	const h = await harness(async () => {
		throw Error("bad signature");
	});
	try {
		assert.equal((await h.post("not json")).status, 401);
		assert.equal(h.queued.length, 0);
	} finally {
		await h.close();
	}
});

test("oversized bodies are rejected without verification or enqueue", async () => {
	let verified = 0;
	const h = await harness(async () => {
		verified++;
	});
	try {
		const big = JSON.stringify({
			id: "e",
			type: "agent.session.failed",
			data: { id: "s", pad: "x".repeat(MAX_BODY_BYTES) },
		});
		assert.equal((await h.post(big)).status, 413, "declared length");
		// Chunked upload without Content-Length is cut off while streaming.
		const status = await new Promise<number | string>((resolve) => {
			const req = request(
				h.url,
				{ method: "POST", headers: { "transfer-encoding": "chunked" } },
				(res) => {
					res.resume();
					resolve(res.statusCode!);
				},
			);
			req.on("error", (e) =>
				resolve((e as NodeJS.ErrnoException).code ?? "error"),
			);
			const chunk = "x".repeat(16384);
			let sent = 0;
			const pump = () => {
				while (sent < MAX_BODY_BYTES * 2) {
					sent += chunk.length;
					if (!req.write(chunk)) {
						req.once("drain", pump);
						return;
					}
				}
				req.end();
			};
			pump();
		});
		assert.ok(
			status === 413 || status === "ECONNRESET" || status === "EPIPE",
			String(status),
		);
		assert.equal(verified, 0);
		assert.equal(h.queued.length, 0);
		assert.equal(
			(
				await h.post(
					JSON.stringify({
						id: "e",
						type: "agent.session.failed",
						data: { id: "s" },
					}),
				)
			).status,
			202,
			"server still serves after rejecting",
		);
	} finally {
		await h.close();
	}
});

test("enqueue failure returns 500 so the sender retries", async () => {
	const server = webhookServer({
		secret: () => "whsec_test",
		verify: async () => {},
		enqueue: () => {
			throw Error("db locked");
		},
		log: () => {},
	});
	server.listen(0, "127.0.0.1");
	await once(server, "listening");
	try {
		assert.equal(
			(
				await fetch(
					`http://127.0.0.1:${(server.address() as { port: number }).port}/webhooks/openai`,
					{
						method: "POST",
						body: JSON.stringify({
							id: "e",
							type: "agent.session.failed",
							data: { id: "s" },
						}),
					},
				)
			).status,
			500,
		);
	} finally {
		server.close();
		server.closeAllConnections();
		await once(server, "close");
	}
});
