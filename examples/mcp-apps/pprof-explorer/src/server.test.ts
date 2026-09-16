import assert from "node:assert/strict";
import { existsSync } from "node:fs";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { type Server, createServer } from "node:http";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { after, before, test } from "node:test";
import { fileURLToPath } from "node:url";
import { gzipSync } from "node:zlib";
import { Client, InMemoryTransport } from "@modelcontextprotocol/client";
import type { CallToolResult } from "@modelcontextprotocol/server";
import type { Profile } from "pprof-format";
import {
	bigGoroutineProfile,
	bigHeapProfile,
	cpuProfile,
	goroutineProfile,
	heapProfile,
	heapProfileAfter,
} from "./fixtures.js";
import { fetchProfile } from "./fetch.js";
import {
	EXPLORER_URI,
	RESULT_BUDGET_BYTES,
	SCRIPT_PLACEHOLDER,
	allowedHostnames,
	buildServer,
	parseTarget,
} from "./server.js";
import { SnapshotStore } from "./store.js";

const HOST_CAP_BYTES = 64 * 1024;
const srcDir = dirname(fileURLToPath(import.meta.url));

// Fixture target standing in for a Go process with net/http/pprof.
interface Target {
	server: Server;
	url: string;
	cpuInFlight: number;
	cpuMaxInFlight: number;
	heapCaptures: number;
	lastUrl: string;
}

const target: Target = {
	server: createServer(),
	url: "",
	cpuInFlight: 0,
	cpuMaxInFlight: 0,
	heapCaptures: 0,
	lastUrl: "",
};

function encode(profile: Profile): Buffer {
	return Buffer.from(profile.encode());
}

target.server.on("request", (req, res) => {
	const url = new URL(req.url ?? "/", "http://localhost");
	if (url.pathname.startsWith("/debug/pprof/")) {
		target.lastUrl = url.pathname + url.search;
	}
	const send = (body: Buffer, gzip: boolean) => {
		res.writeHead(200, { "content-type": "application/octet-stream" });
		res.end(gzip ? gzipSync(body) : body);
	};
	switch (url.pathname) {
		case "/debug/pprof/heap":
			target.heapCaptures++;
			send(encode(target.heapCaptures === 1 ? heapProfile() : heapProfileAfter()), true);
			return;
		case "/debug/pprof/allocs":
			send(encode(bigHeapProfile()), true);
			return;
		case "/debug/pprof/goroutine":
			send(encode(goroutineProfile()), false);
			return;
		case "/debug/pprof/goroutineleak":
			send(encode(bigGoroutineProfile()), true);
			return;
		case "/debug/pprof/profile": {
			target.cpuInFlight++;
			target.cpuMaxInFlight = Math.max(target.cpuMaxInFlight, target.cpuInFlight);
			setTimeout(() => {
				target.cpuInFlight--;
				send(encode(cpuProfile()), true);
			}, 30);
			return;
		}
		case "/debug/pprof/block":
			res.writeHead(302, { location: `${target.url}/debug/pprof/heap` });
			res.end();
			return;
		case "/debug/pprof/mutex":
			res.writeHead(500, { "content-type": "text/plain" });
			res.end("Could not enable mutex profiling: rate is zero\n");
			return;
		case "/debug/pprof/bomb":
			// 17 MiB of zeros compresses to a few KiB; inflating must be refused.
			send(Buffer.alloc(17 * 1024 * 1024), true);
			return;
		case "/debug/pprof/huge": {
			res.writeHead(200, { "content-type": "application/octet-stream" });
			const chunk = Buffer.alloc(1024 * 1024, 1);
			let sent = 0;
			const push = () => {
				while (sent < 20 && res.write(chunk)) {
					sent++;
				}
				if (sent < 20) {
					res.once("drain", push);
				} else {
					res.end();
				}
			};
			push();
			return;
		}
		case "/api/status":
			res.writeHead(200, { "content-type": "application/json" });
			res.end(
				JSON.stringify({
					goroutines: 512,
					uptime: "3m",
					scenarios: [
						{ name: "goroutine-leak", running: true, state: { count: 500 } },
						{ name: "heap-growth", running: false, state: {} },
					],
				}),
			);
			return;
		default:
			res.writeHead(404);
			res.end("not found");
	}
});

let viewDir: string;

before(async () => {
	await new Promise<void>((resolve) =>
		target.server.listen(0, "127.0.0.1", resolve),
	);
	const address = target.server.address();
	assert.ok(address && typeof address === "object");
	target.url = `http://127.0.0.1:${address.port}`;

	// Stand-in view so the resource test does not depend on the real files.
	viewDir = await mkdtemp(join(tmpdir(), "pprof-explorer-view-"));
	await writeFile(
		join(viewDir, "explorer.html"),
		`<!doctype html><html><body><div id="app"></div>\n${SCRIPT_PLACEHOLDER}\n</body></html>\n`,
	);
	await writeFile(
		join(viewDir, "explorer.js"),
		`const marker = "</script>";\nexport const ready = true; // $& stays literal\n`,
	);
});

after(async () => {
	await new Promise<void>((resolve) => target.server.close(() => resolve()));
	await rm(viewDir, { recursive: true, force: true });
});

async function connect(store = new SnapshotStore()): Promise<Client> {
	const [clientTransport, serverTransport] = InMemoryTransport.createLinkedPair();
	const server = buildServer({ target: target.url, store, viewDir });
	await server.connect(serverTransport);
	const client = new Client({ name: "test", version: "0.0.0" });
	await client.connect(clientTransport);
	return client;
}

async function call(
	client: Client,
	name: string,
	args: Record<string, unknown> = {},
): Promise<CallToolResult> {
	return client.callTool({ name, arguments: args });
}

function text(result: CallToolResult): string {
	const block = result.content[0];
	assert.ok(block && block.type === "text");
	return block.text;
}

function structured(result: CallToolResult): Record<string, unknown> {
	assert.ok(result.structuredContent, "structuredContent missing");
	return result.structuredContent as Record<string, unknown>;
}

function bytes(result: CallToolResult): number {
	return Buffer.byteLength(JSON.stringify(result), "utf8");
}

test("parseTarget accepts http(s) with a host and rejects the rest", () => {
	assert.equal(parseTarget("http://127.0.0.1:6060"), "http://127.0.0.1:6060");
	assert.equal(parseTarget("https://lab.internal/"), "https://lab.internal");
	assert.equal(parseTarget("http://lab:6060/prefix/"), "http://lab:6060/prefix");
	assert.throws(() => parseTarget("ftp://x"), /must use http or https/);
	assert.throws(() => parseTarget("not a url"), /not a valid URL/);
	assert.throws(() => parseTarget("http://x/?a=b"), /query or fragment/);
	assert.throws(() => parseTarget("file:///etc/passwd"), /must use http or https/);
});

test("allowedHostnames covers loopback, the bound host, and extras", () => {
	assert.deepEqual(allowedHostnames("127.0.0.1"), ["localhost", "127.0.0.1", "[::1]"]);
	assert.deepEqual(allowedHostnames("0.0.0.0"), ["localhost", "127.0.0.1", "[::1]"]);
	assert.deepEqual(allowedHostnames("10.0.0.5"), ["localhost", "127.0.0.1", "[::1]", "10.0.0.5"]);
	assert.deepEqual(allowedHostnames("::", " 172.17.0.3, lab.internal ,fe80::1"), [
		"localhost",
		"127.0.0.1",
		"[::1]",
		"172.17.0.3",
		"lab.internal",
		"[fe80::1]",
	]);
});

test("every tool advertises the explorer resource; drop_profile is app-only", async () => {
	const client = await connect();
	const { tools } = await client.listTools();
	const names = tools.map((t) => t.name).sort();
	assert.deepEqual(names, [
		"callers_callees",
		"capture_profile",
		"diff_profiles",
		"drop_profile",
		"goroutine_groups",
		"list_profiles",
		"top",
	]);
	for (const tool of tools) {
		const ui = (tool._meta as { ui?: { resourceUri?: string; visibility?: string[] } })?.ui;
		assert.ok(ui, `${tool.name} has no _meta.ui`);
		assert.equal(ui.resourceUri, EXPLORER_URI, tool.name);
		if (tool.name === "drop_profile") {
			assert.deepEqual(ui.visibility, ["app"]);
		} else {
			assert.deepEqual(ui.visibility, ["model", "app"], tool.name);
		}
	}
	const capture = tools.find((t) => t.name === "capture_profile");
	assert.ok(capture);
	assert.match(capture.description ?? "", /capture, then inspect/i);
	assert.match(capture.description ?? "", /cumulative since process start/);
	const diffTool = tools.find((t) => t.name === "diff_profiles");
	assert.match(diffTool?.description ?? "", /leaks/);
	const cc = tools.find((t) => t.name === "callers_callees");
	assert.match(cc?.description ?? "", /who allocates/);
});

test("capture_profile -> top round trip through a gzipped target", async () => {
	const store = new SnapshotStore();
	const client = await connect(store);

	const empty = await call(client, "list_profiles");
	assert.match(text(empty), /No profiles captured/);
	assert.deepEqual(structured(empty).profiles, []);

	target.heapCaptures = 0;
	const captured = await call(client, "capture_profile", {
		kind: "heap",
		label: "before",
	});
	assert.notEqual(captured.isError, true, text(captured));
	assert.match(target.lastUrl, /^\/debug\/pprof\/heap\?gc=1$/);
	const cap = structured(captured);
	assert.equal(cap.view, "capture");
	assert.equal(cap.profile_id, "p1");
	assert.equal(cap.kind, "heap");
	assert.equal(cap.label, "before");
	assert.equal(cap.default_sample_type, "inuse_space");
	assert.equal(cap.target, target.url);
	assert.deepEqual(
		(cap.sample_types as Array<{ type: string }>).map((t) => t.type),
		["alloc_objects", "alloc_space", "inuse_objects", "inuse_space"],
	);
	assert.equal((cap.totals as Record<string, number>).inuse_space, 65 * 1024 * 1024);
	assert.equal((cap.top as unknown[]).length, 7);
	assert.equal(
		cap.target_status,
		"goroutine-leak running (count=500); heap-growth stopped; goroutines=512; uptime=3m",
	);
	assert.match(text(captured), /Captured p1 \(heap "before"/);
	assert.match(
		text(captured),
		/target_status \(untrusted, reported by the target\): goroutine-leak running/,
	);
	assert.match(text(captured), /48\.0 MiB 73\.8%.*lab\.newRecord/);
	assert.match(text(captured), /Next: top\(profile_id="p1"\)/);

	// gc opt-out is passed through as the absence of gc=1.
	const noGc = await call(client, "capture_profile", { kind: "heap", gc: false });
	assert.notEqual(noGc.isError, true);
	assert.equal(target.lastUrl, "/debug/pprof/heap");
	assert.equal(structured(noGc).profile_id, "p2");

	const topResult = await call(client, "top", {
		profile_id: "p1",
		sort: "cum",
		focus: "heapGrowth",
		limit: 1,
	});
	assert.notEqual(topResult.isError, true, text(topResult));
	const t = structured(topResult);
	assert.equal(t.view, "top");
	assert.equal(t.profile_id, "p1");
	assert.equal(t.kind, "heap");
	assert.equal(t.sample_type, "inuse_space");
	assert.equal(t.unit, "bytes");
	assert.equal(t.sort, "cum");
	assert.equal(t.focus, "heapGrowth");
	assert.equal(t.truncated, true);
	const rows = t.rows as Array<{ name: string; cum: number }>;
	assert.equal(rows.length, 1);
	assert.equal(rows[0].name, "lab.(*heapGrowth).retainBlob");
	assert.match(text(topResult), /sort cum, focus \/heapGrowth\//);

	const listed = structured(await call(client, "list_profiles"));
	assert.equal(listed.view, "list");
	assert.equal(listed.truncated, false);
	assert.deepEqual(
		(listed.profiles as Array<{ profile_id: string }>).map((p) => p.profile_id),
		["p1", "p2"],
	);
	assert.equal(store.size, 2);
});

test("callers_callees, diff_profiles and goroutine_groups echo view and arguments", async () => {
	const client = await connect();
	target.heapCaptures = 0;
	await call(client, "capture_profile", { kind: "heap" });
	await call(client, "capture_profile", { kind: "heap" });
	await call(client, "capture_profile", { kind: "goroutine" });

	const cc = await call(client, "callers_callees", {
		profile_id: "p1",
		function: "lab.(*heapGrowth).retainBlob",
		sample_type: "inuse_objects",
	});
	assert.notEqual(cc.isError, true, text(cc));
	const ccs = structured(cc);
	assert.equal(ccs.view, "callers");
	assert.equal(ccs.sample_type, "inuse_objects");
	assert.deepEqual(ccs.callers, [{ name: "lab.(*heapGrowth).run", weight: 1200 }]);
	assert.deepEqual(ccs.callees, [{ name: "lab.newRecord", weight: 1000 }]);
	assert.match(text(cc), /Callers \(1\):\n\s+1200 99\.1%\s+lab\.\(\*heapGrowth\)\.run/);
	assert.match(text(cc), /Callees \(1\):/);

	const d = await call(client, "diff_profiles", { base_id: "p1", profile_id: "p2" });
	assert.notEqual(d.isError, true, text(d));
	const ds = structured(d);
	assert.equal(ds.view, "diff");
	assert.equal(ds.base_id, "p1");
	assert.equal(ds.profile_id, "p2");
	assert.equal(ds.sample_type, "inuse_space");
	const drows = ds.rows as Array<{ name: string; delta_flat: number }>;
	assert.equal(drows[0].name, "lab.(*heapGrowth).retainBlob");
	assert.equal(drows[0].delta_flat, 12 * 1024 * 1024);
	assert.match(text(d), /Diff p1 -> p2 \(heap, inuse_space\): total 65\.0 MiB -> 78\.0 MiB \(\+13\.0 MiB\)/);

	const g = await call(client, "goroutine_groups", { profile_id: "p3", limit: 2 });
	assert.notEqual(g.isError, true, text(g));
	const gs = structured(g);
	assert.equal(gs.view, "groups");
	assert.equal(gs.total, 504);
	assert.equal(gs.group_count, 4);
	assert.equal(gs.truncated, true);
	const groups = gs.groups as Array<{ count: number; labels: Record<string, string> }>;
	assert.equal(groups.length, 2);
	assert.equal(groups[0].count, 300);
	assert.equal(groups[0].labels.scenario, "goroutine-leak");
	assert.equal(groups[1].count, 200);
	assert.match(text(g), /300\s+runtime\.gopark <- runtime\.chanrecv <- lab\.\(\*goroutineLeak\)\.parkedWorker\s+\[scenario=goroutine-leak\]/);
	assert.match(text(g), /200\s+runtime\.gopark.*\[batch=second scenario=goroutine-leak\]/);
});

test("errors return isError results with clear text", async () => {
	const client = await connect();
	target.heapCaptures = 0;
	await call(client, "capture_profile", { kind: "heap" });

	const unknown = await call(client, "top", { profile_id: "p99" });
	assert.equal(unknown.isError, true);
	assert.match(text(unknown), /No profile with id p99\. Known ids: p1\./);
	assert.equal(structured(unknown).view, "error");

	const mismatch = await call(client, "goroutine_groups", { profile_id: "p1" });
	assert.equal(mismatch.isError, true);
	assert.match(text(mismatch), /p1 is a heap profile; goroutine_groups needs/);

	const badRegex = await call(client, "top", { profile_id: "p1", focus: "(a+)+" });
	assert.equal(badRegex.isError, true);
	assert.match(text(badRegex), /quantifiers applied to a group.*plain substring/);

	const badType = await call(client, "top", { profile_id: "p1", sample_type: "nope" });
	assert.equal(badType.isError, true);
	assert.match(text(badType), /unknown sample_type "nope"/);

	const missingFn = await call(client, "callers_callees", {
		profile_id: "p1",
		function: "nope",
	});
	assert.equal(missingFn.isError, true);
	assert.match(text(missingFn), /not found in profile/);

	await call(client, "capture_profile", { kind: "goroutine" });
	const kinds = await call(client, "diff_profiles", { base_id: "p1", profile_id: "p2" });
	assert.equal(kinds.isError, true);
	assert.match(text(kinds), /kinds must match/);

	// The target's own error body is surfaced.
	const failed = await call(client, "capture_profile", { kind: "mutex" });
	assert.equal(failed.isError, true);
	assert.match(text(failed), /Capture of mutex from .* failed: .*HTTP 500.*Could not enable mutex profiling: rate is zero/);

	const dropped = await call(client, "drop_profile", { profile_id: "p1" });
	assert.notEqual(dropped.isError, true);
	assert.equal(structured(dropped).view, "dropped");
	assert.equal(structured(dropped).profile_id, "p1");
	assert.equal(structured(dropped).truncated, false);
	assert.deepEqual(
		(structured(dropped).profiles as Array<{ profile_id: string }>).map((p) => p.profile_id),
		["p2"],
	);
	const again = await call(client, "drop_profile", { profile_id: "p1" });
	assert.equal(again.isError, true);
});

test("results stay under the host cap at worst-case limits", async () => {
	const client = await connect();
	const allocs = await call(client, "capture_profile", { kind: "allocs" });
	assert.notEqual(allocs.isError, true, text(allocs));
	assert.ok(bytes(allocs) <= RESULT_BUDGET_BYTES, `capture ${bytes(allocs)}`);
	const leak = await call(client, "capture_profile", { kind: "goroutineleak" });
	assert.notEqual(leak.isError, true, text(leak));
	assert.ok(bytes(leak) <= RESULT_BUDGET_BYTES, `capture ${bytes(leak)}`);
	const allocs2 = await call(client, "capture_profile", { kind: "allocs" });
	assert.notEqual(allocs2.isError, true);

	const checks: Array<[string, Record<string, unknown>]> = [
		["list_profiles", {}],
		["top", { profile_id: "p1", limit: 200 }],
		["top", { profile_id: "p1", limit: 200, sort: "cum" }],
		["callers_callees", { profile_id: "p1", function: "hub.Dispatch" }],
		["diff_profiles", { base_id: "p1", profile_id: "p3", limit: 200 }],
		["goroutine_groups", { profile_id: "p2", limit: 200 }],
	];
	for (const [name, args] of checks) {
		const result = await call(client, name, args);
		assert.notEqual(result.isError, true, `${name}: ${text(result)}`);
		const size = bytes(result);
		assert.ok(size <= RESULT_BUDGET_BYTES, `${name} is ${size} bytes`);
		assert.ok(size < HOST_CAP_BYTES);
	}

	// The worst cases are actually cut, so the budget is exercised.
	const topResult = structured(await call(client, "top", { profile_id: "p1", limit: 200 }));
	assert.equal(topResult.truncated, true);
	assert.ok((topResult.rows as unknown[]).length < 200);
	assert.equal(topResult.function_count, 552);
	const groups = structured(await call(client, "goroutine_groups", { profile_id: "p2", limit: 200 }));
	assert.equal(groups.truncated, true);
	assert.ok((groups.groups as unknown[]).length < 200);
	const cc = structured(
		await call(client, "callers_callees", { profile_id: "p1", function: "hub.Dispatch" }),
	);
	assert.equal(cc.truncated, true);
	assert.ok((cc.callers as unknown[]).length + (cc.callees as unknown[]).length < 250);
	const d = structured(
		await call(client, "diff_profiles", { base_id: "p1", profile_id: "p3", limit: 200 }),
	);
	assert.equal(d.truncated, true);
});

test("CPU captures are serialized and the fetch carries seconds", async () => {
	const client = await connect();
	target.cpuMaxInFlight = 0;
	const results = await Promise.all([
		call(client, "capture_profile", { kind: "profile", seconds: 1 }),
		call(client, "capture_profile", { kind: "profile", seconds: 2 }),
		call(client, "capture_profile", { kind: "profile" }),
	]);
	for (const r of results) {
		assert.notEqual(r.isError, true, text(r));
		assert.equal(structured(r).default_sample_type, "cpu");
	}
	assert.equal(target.cpuMaxInFlight, 1);
	assert.match(target.lastUrl, /^\/debug\/pprof\/profile\?seconds=[125]$/);
	assert.match(text(results[0]), /4\.80s 96\.0%.*hotLoop/);
});

test("fetchProfile refuses redirects, oversize bodies, and oversize gzip output", async () => {
	await assert.rejects(
		fetchProfile(target.url, "block"),
		/request to \/debug\/pprof\/block failed/,
	);
	await assert.rejects(
		fetchProfile(target.url, "huge" as "heap"),
		/exceeded 16777216 bytes/,
	);
	await assert.rejects(
		fetchProfile(target.url, "bomb" as "heap"),
		/exceeded 16777216 bytes/,
	);
	const client = await connect();
	const redirected = await call(client, "capture_profile", { kind: "block" });
	assert.equal(redirected.isError, true);
	assert.match(text(redirected), /Capture of block from .* failed/);
});

test("explorer resource inlines the module script", async () => {
	const client = await connect();
	const { resources } = await client.listResources();
	const explorer = resources.find((r) => r.uri === EXPLORER_URI);
	assert.ok(explorer);
	assert.equal(explorer.mimeType, "text/html;profile=mcp-app");
	assert.deepEqual((explorer._meta as { ui?: unknown })?.ui, { prefersBorder: true });

	const { contents } = await client.readResource({ uri: EXPLORER_URI });
	assert.equal(contents.length, 1);
	const [content] = contents;
	assert.equal(content.mimeType, "text/html;profile=mcp-app");
	assert.ok("text" in content && typeof content.text === "string");
	assert.ok(!content.text.includes(SCRIPT_PLACEHOLDER));
	assert.match(
		content.text,
		/<script type="module">\nconst marker = "<\\\/script>";\nexport const ready = true; \/\/ \$& stays literal\n\n<\/script>/,
	);
});

test("the real explorer.html carries the script placeholder", { skip: !existsSync(join(srcDir, "explorer.html")) }, async () => {
	const html = await readFile(join(srcDir, "explorer.html"), "utf8");
	assert.ok(html.includes(SCRIPT_PLACEHOLDER));
	assert.ok(existsSync(join(srcDir, "explorer.js")));
	const [clientTransport, serverTransport] = InMemoryTransport.createLinkedPair();
	const server = buildServer({ target: target.url, viewDir: srcDir });
	await server.connect(serverTransport);
	const client = new Client({ name: "test", version: "0.0.0" });
	await client.connect(clientTransport);
	const { contents } = await client.readResource({ uri: EXPLORER_URI });
	const [content] = contents;
	assert.ok("text" in content && typeof content.text === "string");
	assert.match(content.text, /<script type="module">/);
	assert.ok(!content.text.includes(SCRIPT_PLACEHOLDER));
});
