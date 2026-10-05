import assert from "node:assert/strict";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import {
	type Dependencies,
	isSession,
	MAX_ATTEMPTS,
	parseEvent,
	processEvent,
	Store,
} from "./core.ts";

const event = {
	id: "evt_one",
	type: "agent.session.action_required",
	data: { id: "sess_one", required_action: { type: "environment_connection" } },
};
const session = {
	id: "sess_one",
	status: "requires_action",
	agent: { id: "agent_demo" },
	environment: {
		type: "self_hosted",
		id: "env_one",
		remote_url: "https://api.openai.com/v1/agents/api/connect/test",
		workspace_directory: "/home/coder/demo",
	},
	required_actions: [
		{ type: "environment_connection", environment_id: "env_one" },
	],
};
function fixture(mode: "audit" | "allocate" = "allocate") {
	const store = new Store(":memory:");
	let calls = 0;
	const deps: Dependencies = {
		store,
		mode,
		agentId: "agent_demo",
		workspace: "david-fraley/codex-stage1",
		retrieve: async () => session,
		connect: async () => {
			calls++;
		},
		stop: async () => {},
		log: () => {},
	};
	return { store, deps, count: () => calls };
}
test("duplicate delivery is durably deduplicated", () => {
	const { store } = fixture();
	assert.equal(store.enqueue(event), true);
	assert.equal(store.enqueue(event), false);
	assert.equal(store.next()?.id, event.id);
	store.close();
});
test("audit gate reads current action without allocating", async () => {
	const f = fixture("audit");
	await processEvent(event, f.deps);
	assert.equal(f.count(), 0);
	assert.equal(f.store.allocation("sess_one"), undefined);
	f.store.close();
});
test("real connection action allocates exactly once when its delivery repeats", async () => {
	const f = fixture();
	await processEvent(event, f.deps);
	await processEvent(event, f.deps);
	assert.equal(f.count(), 1);
	f.store.close();
});
test("wrong agent and resolved action cannot allocate", async () => {
	for (const s of [
		{ ...session, agent: { id: "other" } },
		{ ...session, required_actions: [] },
	]) {
		const f = fixture();
		f.deps.retrieve = async () => s;
		await processEvent(event, f.deps);
		assert.equal(f.count(), 0);
		f.store.close();
	}
});
test("function call is not a connection request", async () => {
	const f = fixture();
	await processEvent(
		{
			...event,
			data: { ...event.data, required_action: { type: "function_call" } },
		},
		f.deps,
	);
	assert.equal(f.count(), 0);
	f.store.close();
});
test("second session cannot claim the leased workspace", async () => {
	const f = fixture();
	await processEvent(event, f.deps);
	f.deps.retrieve = async () => ({
		...session,
		id: "sess_two",
		environment: { ...session.environment, id: "env_two" },
		required_actions: [
			{ type: "environment_connection", environment_id: "env_two" },
		],
	});
	await assert.rejects(
		processEvent(
			{ ...event, id: "evt_two", data: { ...event.data, id: "sess_two" } },
			f.deps,
		),
		/capacity/,
	);
	assert.equal(f.count(), 1);
	f.store.close();
});
test("ended lease cannot be resurrected by delayed events", async () => {
	const f = fixture();
	await processEvent(event, f.deps);
	f.store.end("sess_one");
	await processEvent({ ...event, id: "evt_late" }, f.deps);
	assert.equal(f.count(), 1);
	f.store.close();
});
test("allocation failure retains reservation and retries safely", async () => {
	const f = fixture();
	let fail = true;
	f.deps.connect = async () => {
		if (fail) throw Error("temporary");
	};
	await assert.rejects(processEvent(event, f.deps));
	assert.equal(f.store.allocation("sess_one")?.state, "connecting");
	fail = false;
	await processEvent(event, f.deps);
	assert.equal(f.store.allocation("sess_one")?.state, "connected");
	f.store.close();
});

function tracked(mode: "audit" | "allocate" = "allocate") {
	const f = fixture(mode);
	const stops: string[][] = [];
	const logs: string[] = [];
	f.deps.stop = async (w: string, s: string) => {
		stops.push([w, s]);
	};
	f.deps.log = (k: string) => {
		logs.push(k);
	};
	return { ...f, stops, logs };
}
const missing = async () => {
	throw Object.assign(Error("not found"), { status: 404 });
};
const failedEvent = {
	id: "evt_failed",
	type: "agent.session.failed",
	data: { id: "sess_one" },
};

test("queue, dedupe, and leases survive a controller restart", async () => {
	const dir = mkdtempSync(join(tmpdir(), "core-test-"));
	const path = join(dir, "controller.sqlite");
	try {
		let store = new Store(path);
		let calls = 0;
		const deps = (s: Store): Dependencies => ({
			store: s,
			mode: "allocate",
			agentId: "agent_demo",
			workspace: "owner/ws",
			retrieve: async () => session,
			connect: async () => {
				calls++;
			},
			stop: async () => {},
			log: () => {},
		});
		assert.equal(store.enqueue(event), true);
		const e = store.next()!;
		await processEvent(e, deps(store));
		store.done(e.id);
		store.close();
		store = new Store(path);
		assert.equal(
			store.enqueue(event),
			false,
			"redelivery after restart is deduplicated",
		);
		assert.equal(store.next(), undefined);
		assert.equal(store.allocation("sess_one")?.state, "connected");
		await processEvent(event, deps(store));
		assert.equal(calls, 1, "repeated delivery after restart does not relaunch");
		store.end("sess_one");
		store.close();
		store = new Store(path);
		await processEvent({ ...event, id: "evt_after_restart" }, deps(store));
		assert.equal(calls, 1, "ended lease stays ended across restart");
		assert.equal(store.allocation("sess_one")?.state, "ended");
		store.close();
	} finally {
		rmSync(dir, { recursive: true, force: true });
	}
});
test("exhausted events are parked as failed and stop being retried", () => {
	const { store } = fixture();
	store.enqueue(event);
	for (let i = 0; i < MAX_ATTEMPTS; i++) {
		assert.equal(store.next()?.id, event.id);
		store.fail(event.id, "boom");
	}
	assert.equal(store.next(), undefined);
	assert.deepEqual(
		{ ...store.event(event.id) },
		{ id: event.id, state: "failed", attempts: MAX_ATTEMPTS, error: "boom" },
	);
	store.fail(event.id, "late");
	assert.equal(store.event(event.id)?.attempts, MAX_ATTEMPTS);
	store.close();
});
test("store persists only validated event fields and rejects malformed events", () => {
	const { store } = fixture();
	assert.throws(
		() => store.enqueue({ id: "", type: "x", data: { id: "s" } }),
		/invalid event/,
	);
	store.enqueue({ ...event, extra: "x".repeat(1000) } as typeof event);
	assert.deepEqual(store.next(), event);
	store.close();
});
test("parseEvent rejects untrusted shapes and strips unknown fields", () => {
	for (const bad of [
		null,
		[],
		"x",
		1,
		{},
		{ id: 1, type: "t", data: { id: "s" } },
		{ id: "e", type: "t" },
		{ id: "e", type: "t", data: null },
		{ id: "e", type: "t", data: [] },
		{ id: "e", type: "t", data: { id: "" } },
		{ id: "e".repeat(300), type: "t", data: { id: "s" } },
		{
			id: "e",
			type: "t",
			data: { id: "s", required_action: "environment_connection" },
		},
		{ id: "e", type: "t", data: { id: "s", required_action: { type: 7 } } },
	])
		assert.equal(parseEvent(bad), undefined, JSON.stringify(bad));
	assert.deepEqual(
		parseEvent({
			id: "e",
			type: "t",
			object: "event",
			data: {
				id: "s",
				extra: 1,
				required_action: { type: "environment_connection", other: 2 },
			},
		}),
		{
			id: "e",
			type: "t",
			data: { id: "s", required_action: { type: "environment_connection" } },
		},
	);
	assert.deepEqual(
		parseEvent({
			id: "e",
			type: "t",
			data: { id: "s", required_action: null },
		}),
		{ id: "e", type: "t", data: { id: "s" } },
	);
});
test("missing session with a live lease releases it exactly once", async () => {
	for (const e of [failedEvent, { ...event, id: "evt_again" }]) {
		const f = tracked();
		await processEvent(event, f.deps);
		f.deps.retrieve = missing;
		await processEvent(e, f.deps);
		await processEvent(e, f.deps);
		assert.deepEqual(f.stops, [["david-fraley/codex-stage1", "sess_one"]]);
		assert.equal(f.store.allocation("sess_one")?.state, "ended");
		assert.ok(
			f.logs.includes("session_missing") &&
				f.logs.includes("released_executor"),
		);
		f.store.close();
	}
});
test("missing session without a lease, or in audit mode, stops nothing", async () => {
	for (const mode of ["allocate", "audit"] as const) {
		const f = tracked(mode);
		f.deps.retrieve = missing;
		await processEvent(failedEvent, f.deps);
		assert.deepEqual(f.stops, []);
		f.store.close();
	}
});
test("non-404 retrieval errors propagate and keep the lease", async () => {
	const f = tracked();
	await processEvent(event, f.deps);
	f.deps.retrieve = async () => {
		throw Object.assign(Error("server"), { status: 500 });
	};
	await assert.rejects(processEvent(failedEvent, f.deps), /server/);
	assert.deepEqual(f.stops, []);
	assert.equal(f.store.allocation("sess_one")?.state, "connected");
	f.store.close();
});
test("failed session releases lease; stop failure keeps lease live for retry", async () => {
	const f = tracked();
	await processEvent(event, f.deps);
	f.deps.retrieve = async () => ({ ...session, status: "failed" });
	let fail = true;
	f.deps.stop = async (w: string, s: string) => {
		if (fail) throw Error("stop unavailable");
		f.stops.push([w, s]);
	};
	await assert.rejects(processEvent(failedEvent, f.deps), /stop unavailable/);
	assert.equal(f.store.allocation("sess_one")?.state, "connected");
	fail = false;
	await processEvent(failedEvent, f.deps);
	assert.equal(f.store.allocation("sess_one")?.state, "ended");
	await processEvent(failedEvent, f.deps);
	assert.equal(f.stops.length, 1, "ended lease is not stopped twice");
	f.store.close();
});
test("failure event is only acted on when the current session is failed and trusted", async () => {
	for (const s of [
		{ ...session, status: "requires_action" },
		{ ...session, status: "failed", agent: { id: "other" } },
		{ ...session, status: "failed", id: "sess_other" },
		{
			...session,
			status: "failed",
			environment: { ...session.environment, type: "cloud" },
		},
	]) {
		const f = tracked();
		await processEvent(event, f.deps);
		f.deps.retrieve = async () => s;
		await processEvent(failedEvent, f.deps);
		assert.deepEqual(f.stops, []);
		assert.equal(f.store.allocation("sess_one")?.state, "connected");
		f.store.close();
	}
});
test("connection request for a failed session never allocates and releases a stale lease", async () => {
	const f = tracked();
	f.deps.retrieve = async () => ({ ...session, status: "failed" });
	await processEvent(event, f.deps);
	assert.equal(f.count(), 0);
	assert.equal(f.store.allocation("sess_one"), undefined);
	f.deps.retrieve = async () => session;
	await processEvent(event, f.deps);
	assert.equal(f.count(), 1);
	f.deps.retrieve = async () => ({ ...session, status: "failed" });
	await processEvent({ ...event, id: "evt_late" }, f.deps);
	assert.equal(f.count(), 1);
	assert.deepEqual(f.stops, [["david-fraley/codex-stage1", "sess_one"]]);
	assert.equal(f.store.allocation("sess_one")?.state, "ended");
	f.store.close();
});
test("invalid session shapes are logged and never allocate or stop", async () => {
	const bad: unknown[] = [
		null,
		{},
		{ ...session, agent: undefined },
		{ ...session, agent: { id: 5 } },
		{ ...session, required_actions: "environment_connection" },
		{ ...session, required_actions: [null] },
		{ ...session, environment: null },
		{ ...session, environment: { ...session.environment, id: 7 } },
		{ ...session, status: undefined },
		{ ...session, id: "" },
	];
	for (const s of bad) {
		assert.equal(isSession(s), false, JSON.stringify(s));
		const f = tracked();
		await processEvent(event, f.deps);
		f.deps.retrieve = async () => s as typeof session;
		await processEvent({ ...event, id: "evt_bad" }, f.deps);
		await processEvent(failedEvent, f.deps);
		assert.equal(f.count(), 1);
		assert.deepEqual(f.stops, []);
		assert.ok(f.logs.includes("invalid_session"));
		f.store.close();
	}
	assert.equal(isSession(session), true);
});
test("untrusted executor URL or directory is rejected before reserving", async () => {
	for (const [s, msg] of [
		[
			{
				...session,
				environment: {
					...session.environment,
					remote_url: "https://evil.example/v1/agents/api/connect/x",
				},
			},
			/untrusted/,
		],
		[
			{
				...session,
				environment: {
					...session.environment,
					remote_url: "https://api.openai.com/v1/agents/api/connect/x?y=1",
				},
			},
			/untrusted/,
		],
		[
			{
				...session,
				environment: { ...session.environment, workspace_directory: "/" },
			},
			/allowlisted/,
		],
	] as const) {
		const f = tracked();
		f.deps.retrieve = async () => s;
		await assert.rejects(processEvent(event, f.deps), msg);
		assert.equal(f.store.allocation("sess_one"), undefined);
		assert.equal(f.count(), 0);
		f.store.close();
	}
});
test("stale events after a lease ends neither reconnect nor restop", async () => {
	const f = tracked();
	await processEvent(event, f.deps);
	f.store.end("sess_one");
	await processEvent({ ...event, id: "evt_stale" }, f.deps);
	await processEvent(event, f.deps);
	f.deps.retrieve = async () => ({ ...session, status: "failed" });
	await processEvent(failedEvent, f.deps);
	assert.equal(f.count(), 1);
	assert.deepEqual(f.stops, []);
	assert.ok(f.logs.includes("ended_lease"));
	assert.throws(
		() =>
			f.store.reserve(
				"sess_one",
				"env_one",
				"david-fraley/codex-stage1",
				"evt_x",
			),
		/ended/,
	);
	f.store.close();
});
