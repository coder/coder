import { DatabaseSync } from "node:sqlite";
export interface Event {
	id: string;
	type: string;
	data: { id: string; required_action?: { type: string } };
}
export interface Session {
	id: string;
	status: string;
	agent: { id: string };
	environment: {
		type: string;
		id?: string;
		remote_url?: string;
		workspace_directory?: string;
	};
	required_actions: { type: string; environment_id?: string }[];
}
type Allocation = {
	session: string;
	environment: string;
	workspace: string;
	state: string;
	event: string;
};
export const MAX_ATTEMPTS = 3;
const MAX_ID_LENGTH = 256;
const isObject = (v: unknown): v is Record<string, unknown> =>
	typeof v === "object" && v !== null && !Array.isArray(v);
const isId = (v: unknown): v is string =>
	typeof v === "string" && v.length > 0 && v.length <= MAX_ID_LENGTH;
const optionalString = (v: unknown) =>
	v === undefined || v === null || typeof v === "string";
/** Validates an untrusted webhook payload and returns only the fields the worker uses. */
export function parseEvent(value: unknown): Event | undefined {
	if (
		!isObject(value) ||
		!isId(value.id) ||
		!isId(value.type) ||
		!isObject(value.data) ||
		!isId(value.data.id)
	)
		return;
	const action = value.data.required_action;
	if (
		action !== undefined &&
		action !== null &&
		!(isObject(action) && isId(action.type))
	)
		return;
	const event: Event = {
		id: value.id,
		type: value.type,
		data: { id: value.data.id },
	};
	if (isObject(action))
		event.data.required_action = { type: action.type as string };
	return event;
}
/** Checks that an API response has the shape processEvent relies on before any field is trusted. */
export function isSession(value: unknown): value is Session {
	if (
		!isObject(value) ||
		!isId(value.id) ||
		typeof value.status !== "string" ||
		!isObject(value.agent) ||
		!isId(value.agent.id)
	)
		return false;
	const env = value.environment;
	if (
		!isObject(env) ||
		typeof env.type !== "string" ||
		!optionalString(env.id) ||
		!optionalString(env.remote_url) ||
		!optionalString(env.workspace_directory)
	)
		return false;
	return (
		Array.isArray(value.required_actions) &&
		value.required_actions.every(
			(a) =>
				isObject(a) &&
				typeof a.type === "string" &&
				optionalString(a.environment_id),
		)
	);
}
export class Store {
	db: DatabaseSync;
	constructor(path: string) {
		this.db = new DatabaseSync(path);
		this.db.exec(
			`PRAGMA journal_mode=WAL; CREATE TABLE IF NOT EXISTS events(id TEXT PRIMARY KEY,payload TEXT NOT NULL,state TEXT NOT NULL DEFAULT 'pending',attempts INTEGER NOT NULL DEFAULT 0,error TEXT); CREATE TABLE IF NOT EXISTS allocations(session TEXT PRIMARY KEY,environment TEXT UNIQUE NOT NULL,workspace TEXT UNIQUE NOT NULL,state TEXT NOT NULL,event TEXT NOT NULL);`,
		);
	}
	enqueue(event: Event) {
		const e = parseEvent(event);
		if (!e) throw Error("invalid event");
		return (
			this.db
				.prepare("INSERT OR IGNORE INTO events(id,payload) VALUES(?,?)")
				.run(e.id, JSON.stringify(e)).changes === 1
		);
	}
	next(): Event | undefined {
		const row = this.db
			.prepare(
				"SELECT payload FROM events WHERE state='pending' AND attempts<? ORDER BY rowid LIMIT 1",
			)
			.get(MAX_ATTEMPTS) as { payload: string } | undefined;
		return row ? JSON.parse(row.payload) : undefined;
	}
	done(id: string) {
		this.db.prepare("UPDATE events SET state='done' WHERE id=?").run(id);
	}
	// Exhausted events move to 'failed' so they stay visible for manual inspection instead of looking pending.
	fail(id: string, error: string) {
		this.db
			.prepare(
				"UPDATE events SET attempts=attempts+1,error=?,state=CASE WHEN attempts+1>=? THEN 'failed' ELSE state END WHERE id=? AND state='pending'",
			)
			.run(error, MAX_ATTEMPTS, id);
	}
	event(id: string) {
		return this.db
			.prepare("SELECT id,state,attempts,error FROM events WHERE id=?")
			.get(id) as
			| { id: string; state: string; attempts: number; error: string | null }
			| undefined;
	}
	allocation(id: string) {
		return this.db
			.prepare("SELECT * FROM allocations WHERE session=?")
			.get(id) as Allocation | undefined;
	}
	reserve(
		session: string,
		environment: string,
		workspace: string,
		event: string,
	) {
		this.db.exec("BEGIN IMMEDIATE");
		try {
			const old = this.allocation(session);
			if (old) {
				if (old.state === "ended") throw Error("allocation already ended");
				if (old.environment !== environment || old.workspace !== workspace)
					throw Error("allocation identity mismatch");
				this.db.exec("COMMIT");
				return old;
			}
			if (
				this.db
					.prepare(
						"SELECT 1 FROM allocations WHERE workspace=? OR environment=?",
					)
					.get(workspace, environment)
			)
				throw Error(
					"capacity: workspace already reserved; manual retirement required",
				);
			this.db
				.prepare("INSERT INTO allocations VALUES(?,?,?,'connecting',?)")
				.run(session, environment, workspace, event);
			this.db.exec("COMMIT");
			return this.allocation(session)!;
		} catch (e) {
			this.db.exec("ROLLBACK");
			throw e;
		}
	}
	connected(id: string, event: string) {
		this.db
			.prepare(
				"UPDATE allocations SET state='connected',event=? WHERE session=? AND state!='ended'",
			)
			.run(event, id);
	}
	end(id: string) {
		this.db
			.prepare("UPDATE allocations SET state='ended' WHERE session=?")
			.run(id);
	}
	close() {
		this.db.close();
	}
}
export interface Dependencies {
	store: Store;
	mode: "audit" | "allocate";
	agentId: string;
	workspace: string;
	retrieve: (id: string) => Promise<Session>;
	connect: (input: {
		workspace: string;
		environmentId: string;
		remoteUrl: string;
		sessionId: string;
	}) => Promise<void>;
	stop: (workspace: string, sessionId: string) => Promise<void>;
	log: (kind: string, data: Record<string, unknown>) => void;
}
// Stops and ends a live lease held by this session. Only local allocation records are used, so a
// session can only release the workspace it reserved. A stop failure throws and leaves the lease live for retry.
async function release(
	sessionId: string,
	event: Event,
	reason: string,
	d: Dependencies,
) {
	if (d.mode !== "allocate") return;
	const a = d.store.allocation(sessionId);
	if (!a || a.state === "ended") return;
	await d.stop(a.workspace, sessionId);
	d.store.end(sessionId);
	d.log("released_executor", {
		event: event.id,
		session: sessionId,
		workspace: a.workspace,
		reason,
	});
}
export async function processEvent(event: Event, d: Dependencies) {
	if (
		event.type !== "agent.session.failed" &&
		!(
			event.type === "agent.session.action_required" &&
			event.data.required_action?.type === "environment_connection"
		)
	)
		return;
	let s: unknown;
	try {
		s = await d.retrieve(event.data.id);
	} catch (e) {
		if ((e as { status?: number }).status === 404) {
			d.log("session_missing", { event: event.id, session: event.data.id });
			await release(event.data.id, event, "session_missing", d);
			return;
		}
		throw e;
	}
	if (!isSession(s)) {
		d.log("invalid_session", { event: event.id, session: event.data.id });
		return;
	}
	if (
		s.id !== event.data.id ||
		s.agent.id !== d.agentId ||
		s.environment.type !== "self_hosted"
	) {
		d.log("ignored_session", { event: event.id, session: event.data.id });
		return;
	}
	// The current session state decides; a failed session never keeps or gains a lease, whichever event arrives.
	if (s.status === "failed") {
		await release(s.id, event, "session_failed", d);
		if (event.type !== "agent.session.failed")
			d.log("terminal_session", { event: event.id, session: s.id });
		return;
	}
	if (event.type === "agent.session.failed") {
		d.log("failure_not_confirmed", {
			event: event.id,
			session: s.id,
			status: s.status,
		});
		return;
	}
	const env = s.environment;
	if (
		!env.id ||
		!env.remote_url ||
		!s.required_actions.some(
			(a) => a.type === "environment_connection" && a.environment_id === env.id,
		)
	) {
		d.log("resolved_action", { event: event.id, session: s.id });
		return;
	}
	if (env.workspace_directory !== "/home/coder/demo")
		throw Error("workspace directory is not allowlisted");
	const url = new URL(env.remote_url);
	if (
		url.origin !== "https://api.openai.com" ||
		url.username ||
		url.password ||
		!url.pathname.startsWith("/v1/agents/api/connect/") ||
		url.search ||
		url.hash
	)
		throw Error("untrusted executor URL");
	d.log("verified_connection_request", {
		event: event.id,
		session: s.id,
		environment: env.id,
		mode: d.mode,
	});
	if (d.mode === "audit") return;
	const old = d.store.allocation(s.id);
	if (old?.state === "ended") {
		d.log("ended_lease", { event: event.id, session: s.id });
		return;
	}
	const allocation = d.store.reserve(s.id, env.id, d.workspace, event.id);
	// An exact repeated delivery cannot trigger another launch. A new current action can reconnect.
	if (allocation.state === "connected" && allocation.event === event.id) return;
	await d.connect({
		workspace: allocation.workspace,
		environmentId: env.id,
		remoteUrl: env.remote_url,
		sessionId: s.id,
	});
	d.store.connected(s.id, event.id);
	d.log("assigned_executor", {
		event: event.id,
		session: s.id,
		workspace: allocation.workspace,
	});
}
