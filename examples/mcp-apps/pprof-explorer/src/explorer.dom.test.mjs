// DOM tests for the explorer view. The view is mounted into a jsdom iframe
// whose parent window stands in for the MCP Apps host: it records every
// postMessage (with its targetOrigin) and answers JSON-RPC requests.
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import { JSDOM } from "jsdom";
import { mountExplorer } from "./explorer.js";

const ORIGIN = "https://mcp-abc.example.test";
const here = dirname(fileURLToPath(import.meta.url));
// The script is mounted from the imported module, so the placeholder is
// removed instead of inlined.
const HTML = readFileSync(join(here, "explorer.html"), "utf8").replace("<!-- EXPLORER_SCRIPT -->", "");

const tick = (ms = 15) => new Promise((resolve) => setTimeout(resolve, ms));

const p1 = { profile_id: "p1", kind: "heap", captured_at: "2026-09-16T12:01:02Z", totals: { inuse_space: 1000, alloc_space: 5000 } };
const p2 = { profile_id: "p2", kind: "heap", captured_at: "2026-09-16T12:03:11Z", totals: { inuse_space: 2000 } };

const topRows = (suffix) => [
	{ name: "lab.(*heapGrowth).retainBlob" + suffix, file: "lab/heap_growth.go", line: 41, flat: 620, flat_pct: 62, cum: 620, cum_pct: 62 },
	{ name: "main.run" + suffix, file: "main.go", line: 9, flat: 100, flat_pct: 10, cum: 800, cum_pct: 80 },
];

function topResult(profileId, rows) {
	return {
		content: [{ type: "text", text: "top" }],
		structuredContent: { view: "top", profile_id: profileId, sample_type: "inuse_space", unit: "bytes", sort: "flat", total: 1000, truncated: false, rows },
	};
}

function callersResult(profileId, fn) {
	return {
		content: [{ type: "text", text: "callers" }],
		structuredContent: {
			view: "callers", profile_id: profileId, sample_type: "inuse_space", unit: "bytes",
			function: { name: fn, flat: 620, cum: 620 }, callers: [{ name: "caller.of." + fn, weight: 620 }], callees: [], total: 1000,
		},
	};
}

/**
 * Boots the view. `host.onCall(name, args)` produces tools/call results;
 * requests matching `host.hold(msg)` are queued in `host.held` until
 * `host.release()` answers them. Held requests are released when the test
 * ends so no request timeout timer outlives it.
 */
async function boot(t, options = {}) {
	const dom = new JSDOM("<!doctype html><html><body><iframe></iframe></body></html>", { url: ORIGIN + "/", pretendToBeVisual: true });
	const ow = dom.window;
	const frame = ow.document.querySelector("iframe");
	const iw = frame.contentWindow;
	const doc = frame.contentDocument;
	doc.open();
	doc.write(HTML);
	doc.close();

	const host = {
		sent: [],
		held: [],
		profiles: options.profiles || [p1],
		hold: () => false,
		onCall(name, args) {
			if (name === "list_profiles") return { content: [], structuredContent: { view: "list", target: "http://127.0.0.1:6060", profiles: host.profiles } };
			if (name === "top") return topResult(args.profile_id, topRows(""));
			if (name === "callers_callees") return callersResult(args.profile_id, args.function);
			return { content: [{ type: "text", text: "unknown tool " + name }], isError: true };
		},
		deliver(msg, origin = ORIGIN, source = ow) {
			iw.dispatchEvent(new iw.MessageEvent("message", { data: msg, origin, source }));
		},
		reply(id, result) {
			host.deliver({ jsonrpc: "2.0", id, result });
		},
		notify(method, params) {
			host.deliver({ jsonrpc: "2.0", method, params });
		},
		release() {
			const held = host.held.splice(0);
			for (const h of held) h.respond();
		},
		methods() {
			return host.sent.map((s) => s.msg.method).filter(Boolean);
		},
		calls(name) {
			return host.sent.filter((s) => s.msg.method === "tools/call" && s.msg.params.name === name).map((s) => s.msg.params.arguments);
		},
	};

	ow.postMessage = (msg, targetOrigin) => {
		host.sent.push({ msg, targetOrigin });
		if (msg.id === undefined || msg.method === undefined) return;
		let respond;
		if (msg.method === "ui/initialize") {
			respond = () => host.reply(msg.id, { protocolVersion: "2026-01-26", hostInfo: { name: "coder" }, hostCapabilities: {}, hostContext: { theme: "dark" } });
		} else if (msg.method === "tools/call") {
			respond = () => host.reply(msg.id, host.onCall(msg.params.name, msg.params.arguments || {}));
		} else {
			respond = () => host.reply(msg.id, {});
		}
		if (host.hold(msg)) host.held.push({ msg, respond });
		else setTimeout(respond, 0);
	};

	mountExplorer(doc, iw);
	t.after(() => {
		host.release();
		dom.window.close();
	});
	await tick(30);
	const q = (selector) => doc.querySelector(selector);
	const qa = (selector) => Array.from(doc.querySelectorAll(selector));
	const selectedChip = () => (q(".chip.selected .chip-select") || {}).textContent || "";
	const rowNames = () => qa(".row .c-name .name").map((n) => n.textContent);
	return { dom, doc, host, q, qa, selectedChip, rowNames };
}

test("pins the first parent origin, posts to it afterwards, and drops foreign messages", async (t) => {
	const { host, doc, q } = await boot(t);
	assert.equal(host.sent[0].msg.method, "ui/initialize");
	assert.equal(host.sent[0].targetOrigin, "*");
	for (const s of host.sent.slice(1)) assert.equal(s.targetOrigin, ORIGIN, s.msg.method || "response");
	assert.equal(doc.documentElement.getAttribute("data-theme"), "dark");

	const before = host.sent.length;
	host.deliver({ jsonrpc: "2.0", id: "x1", method: "ui/resource-teardown", params: {} }, "https://evil.example.test");
	host.deliver({ jsonrpc: "2.0", id: "x2", method: "ui/resource-teardown", params: {} }, ORIGIN, doc.defaultView);
	host.deliver({ jsonrpc: "2.0", method: "ui/notifications/host-context-changed", params: { theme: "light" } }, "https://evil.example.test");
	await tick();
	assert.equal(host.sent.length, before, "foreign messages produce no replies");
	assert.equal(doc.documentElement.getAttribute("data-theme"), "dark");

	host.notify("ui/notifications/host-context-changed", { theme: "light" });
	host.deliver({ jsonrpc: "2.0", id: "td1", method: "ui/resource-teardown", params: {} });
	await tick();
	assert.equal(doc.documentElement.getAttribute("data-theme"), "light");
	const reply = host.sent.find((s) => s.msg.id === "td1");
	assert.ok(reply, "teardown answered");
	assert.deepEqual(reply.msg.result, {});
	assert.equal(reply.targetOrigin, ORIGIN);
	assert.ok(q("#target").textContent.includes("127.0.0.1:6060"));
});

test("sends ui/notifications/initialized before any tools/call", async (t) => {
	const { host } = await boot(t);
	const methods = host.methods();
	assert.equal(methods[0], "ui/initialize");
	assert.equal(methods[1], "ui/notifications/initialized");
	assert.ok(methods.indexOf("tools/call") > methods.indexOf("ui/notifications/initialized"));
	assert.deepEqual(host.calls("list_profiles"), [{}]);
});

test("agent capture while a selection exists shows a banner and keeps the selection", async (t) => {
	const { host, q, qa, selectedChip } = await boot(t);
	q(".chip .chip-select").click();
	await tick(30);
	assert.ok(selectedChip().startsWith("p1"));

	host.notify("ui/notifications/tool-input", { arguments: { kind: "heap" } });
	host.notify("ui/notifications/tool-result", {
		content: [],
		structuredContent: { view: "capture", ...p2, sample_types: [{ type: "inuse_space", unit: "bytes" }], default_sample_type: "inuse_space", top: [], target_status: "lab: heap-growth running" },
	});
	await tick();
	assert.equal(qa(".chip").length, 2);
	assert.ok(selectedChip().startsWith("p1"), "selection unchanged");
	assert.equal(q("#banner").hidden, false);
	assert.equal(q("#banner-text").textContent, "Agent captured p2 heap");
	assert.equal(q("#banner-show").hidden, false);
	assert.equal(q("#target-status").textContent, "lab: heap-growth running");

	q("#banner-show").click();
	await tick(30);
	assert.ok(selectedChip().startsWith("p2"), "Show selects the captured snapshot");
	assert.equal(q("#banner").hidden, true);
});

test("agent callers result while the user is active offers Show; applying it discards the stale in-flight drill response", async (t) => {
	const { host, q, qa, selectedChip } = await boot(t);
	q(".chip .chip-select").click();
	await tick(30);
	assert.equal(qa(".row").length, 2);

	// The user's own drill-down request stays unanswered.
	host.hold = (msg) => msg.method === "tools/call" && msg.params.name === "callers_callees";
	qa(".row")[0].click();
	await tick();
	assert.equal(host.held.length, 1);
	assert.equal(host.held[0].msg.params.arguments.function, "lab.(*heapGrowth).retainBlob");

	host.notify("ui/notifications/tool-input", { arguments: { profile_id: "p1", function: "main.run" } });
	host.notify("ui/notifications/tool-result", callersResult("p1", "main.run"));
	await tick();
	assert.equal(q("#banner").hidden, false);
	assert.equal(q("#banner-text").textContent, "Agent viewed callers of main.run in p1");
	assert.equal(q("#banner-show").hidden, false, "Show offered because the user was active");
	assert.ok(q("#drill .drill-head .name").textContent !== "main.run", "not auto-applied");

	q("#banner-show").click();
	await tick();
	assert.equal(q("#drill .drill-head .name").textContent, "main.run");
	assert.equal(selectedChip().slice(0, 2), "p1");

	host.release();
	await tick();
	assert.equal(q("#drill .drill-head .name").textContent, "main.run", "stale user drill response ignored");
	assert.equal(host.held.length, 0);
});

test("applying an agent top result discards the stale in-flight report response", async (t) => {
	const { host, q, qa, rowNames } = await boot(t, { profiles: [p1, p2] });
	q(".chip .chip-select").click();
	await tick(30);
	assert.deepEqual(rowNames(), topRows("").map((r) => r.name));

	// The user switches to p2; that top request stays unanswered.
	host.hold = (msg) => msg.method === "tools/call" && msg.params.name === "top";
	qa(".chip .chip-select")[1].click();
	await tick();
	assert.equal(host.held.length, 1);
	assert.equal(host.held[0].msg.params.arguments.profile_id, "p2");

	host.notify("ui/notifications/tool-input", { arguments: { profile_id: "p2", sort: "flat" } });
	host.notify("ui/notifications/tool-result", topResult("p2", topRows(".agent")));
	await tick();
	assert.equal(q("#banner-show").hidden, false);
	q("#banner-show").click();
	await tick();
	assert.deepEqual(rowNames(), topRows(".agent").map((r) => r.name));

	host.release();
	await tick();
	assert.deepEqual(rowNames(), topRows(".agent").map((r) => r.name), "stale user top response ignored");
});

test("agent isError result renders a notice and keeps the selection", async (t) => {
	const { host, q, selectedChip } = await boot(t);
	q(".chip .chip-select").click();
	await tick(30);
	host.notify("ui/notifications/tool-input", { arguments: { profile_id: "p9", sort: "flat" } });
	host.notify("ui/notifications/tool-result", { content: [{ type: "text", text: "profile p9 not found" }], isError: true });
	await tick();
	assert.equal(q("#notice").hidden, false);
	assert.ok(q("#notice").textContent.includes("profile p9 not found"));
	assert.ok(q("#notice").classList.contains("error"));
	assert.ok(selectedChip().startsWith("p1"));
});

test("agent result without structuredContent re-issues the tool from the tool-input arguments", async (t) => {
	const { host } = await boot(t);
	assert.equal(host.calls("top").length, 0);
	host.notify("ui/notifications/tool-input", { arguments: { profile_id: "p1", sort: "cum", focus: "main" } });
	host.notify("ui/notifications/tool-result", { content: [{ type: "text", text: "large text only" }] });
	await tick(30);
	assert.deepEqual(host.calls("top"), [{ profile_id: "p1", sort: "cum", focus: "main" }]);

	// A profile unknown to the view triggers list_profiles before inferring.
	const lists = host.calls("list_profiles").length;
	host.profiles = [p1, { profile_id: "p3", kind: "goroutine", captured_at: "2026-09-16T12:05:00Z", totals: { goroutine: 12 } }];
	host.notify("ui/notifications/tool-input", { arguments: { profile_id: "p3", limit: 25 } });
	host.notify("ui/notifications/tool-result", { content: [{ type: "text", text: "large text only" }] });
	await tick(30);
	assert.equal(host.calls("list_profiles").length, lists + 1);
	assert.deepEqual(host.calls("goroutine_groups"), [{ profile_id: "p3", limit: 25 }]);
});

test("Explain disables every Explain button while the ui/message consent is pending", async (t) => {
	const { host, q, qa } = await boot(t);
	q(".chip .chip-select").click();
	await tick(30);
	host.hold = (msg) => msg.method === "ui/message";
	qa(".row button.explain")[0].click();
	await tick();
	assert.equal(host.held.length, 1);
	const message = host.held[0].msg.params;
	assert.equal(message.role, "user");
	assert.ok(message.content[0].text.startsWith('Explain why the function "lab.(*heapGrowth).retainBlob"'));
	assert.ok(qa("button.explain").length > 1);
	assert.ok(qa("button.explain").every((b) => b.disabled), "all Explain buttons disabled");
	assert.equal(q("#explain-status").textContent, "Waiting for confirmation...");

	host.release();
	await tick();
	assert.ok(qa("button.explain").every((b) => !b.disabled), "all Explain buttons enabled again");
	assert.equal(q("#explain-status").textContent, "Sent to the agent");
	const contexts = host.sent.filter((s) => s.msg.method === "ui/update-model-context");
	assert.ok(contexts.length > 0);
	assert.ok(contexts[contexts.length - 1].msg.params.content[0].text.startsWith("Viewing p1 (heap, inuse_space, sort flat)"));
});

test("dropping a snapshot needs a second click on the armed trash button", async (t) => {
	const { host, q, qa } = await boot(t);
	assert.equal(host.calls("drop_profile").length, 0);
	q(".chip .chip-drop").click();
	await tick();
	assert.equal(host.calls("drop_profile").length, 0, "first click only arms");
	assert.equal(q(".chip .chip-drop").textContent, "drop?");
	host.onCall = (name, args) => (name === "drop_profile" ? { content: [], structuredContent: { view: "dropped", profile_id: args.profile_id } } : { content: [], isError: true });
	q(".chip .chip-drop").click();
	await tick();
	assert.deepEqual(host.calls("drop_profile"), [{ profile_id: "p1" }]);
	assert.equal(qa(".chip").length, 0);
});

test("rows and drill edges are keyboard operable", async (t) => {
	const { doc, host, q, qa } = await boot(t);
	q(".chip .chip-select").click();
	await tick(30);
	const row = qa(".row")[1];
	assert.equal(row.getAttribute("tabindex"), "0");
	assert.equal(row.getAttribute("role"), "row");
	assert.equal(row.getAttribute("aria-selected"), "false");
	row.dispatchEvent(new doc.defaultView.KeyboardEvent("keydown", { key: "Enter", bubbles: true }));
	await tick();
	assert.equal(host.calls("callers_callees").length, 1);
	assert.equal(qa(".row")[1].getAttribute("aria-selected"), "true");
	const edge = q("#drill .edge .edge-name");
	assert.equal(edge.getAttribute("role"), "button");
	assert.equal(edge.getAttribute("tabindex"), "0");
	edge.dispatchEvent(new doc.defaultView.KeyboardEvent("keydown", { key: " ", bubbles: true }));
	await tick();
	assert.equal(host.calls("callers_callees").length, 2);
});
