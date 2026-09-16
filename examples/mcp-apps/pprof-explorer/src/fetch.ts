import { gunzipSync } from "node:zlib";
import { Profile } from "pprof-format";
import { normalizeString } from "./sanitize.js";
import type { ProfileKind } from "./store.js";

export const MAX_PROFILE_BYTES = 16 * 1024 * 1024;
export const MAX_STATUS_BYTES = 8 * 1024;
const MAX_ERROR_BODY_BYTES = 8 * 1024;
const MAX_ERROR_TEXT_LENGTH = 1024;
const MAX_STATUS_SUMMARY_LENGTH = 400;
const STATUS_TIMEOUT_MS = 3_000;
const FETCH_GRACE_MS = 10_000;

export interface FetchProfileOptions {
	/** CPU profiling duration; only sent for kind "profile". */
	seconds?: number;
	/** Whether heap and allocs captures ask the runtime to GC first. */
	gc?: boolean;
}

export class BodyTooLargeError extends Error {
	constructor(limit: number) {
		super(`response body exceeded ${limit} bytes`);
		this.name = "BodyTooLargeError";
	}
}

/**
 * Fetches /debug/pprof/<kind> from the target, gunzips the body when it
 * carries the gzip magic bytes, and decodes it as a pprof Profile. Redirects
 * are refused, the request is aborted after seconds + 10s, and the body is
 * abandoned past MAX_PROFILE_BYTES. A non-2xx response throws an Error whose
 * message ends with the target's own response body.
 */
export async function fetchProfile(
	target: string,
	kind: ProfileKind,
	options: FetchProfileOptions = {},
): Promise<Profile> {
	const url = new URL(`${stripTrailingSlash(target)}/debug/pprof/${kind}`);
	let seconds = 0;
	if (kind === "profile") {
		seconds = options.seconds ?? 5;
		url.searchParams.set("seconds", String(seconds));
	}
	if ((kind === "heap" || kind === "allocs") && options.gc !== false) {
		url.searchParams.set("gc", "1");
	}
	const timeoutMs = seconds * 1000 + FETCH_GRACE_MS;
	const body = await fetchBody(url, timeoutMs, MAX_PROFILE_BYTES);
	const raw = isGzip(body) ? gunzipCapped(body) : body;
	try {
		return Profile.decode(raw);
	} catch (err) {
		const reason = err instanceof Error ? err.message : String(err);
		throw new Error(`could not decode pprof profile: ${reason}`);
	}
}

// Inflates a gzip body, refusing output larger than MAX_PROFILE_BYTES so a
// small compressed body cannot expand without bound.
function gunzipCapped(body: Uint8Array): Uint8Array {
	try {
		return new Uint8Array(
			gunzipSync(body, { maxOutputLength: MAX_PROFILE_BYTES }),
		);
	} catch (err) {
		const code = (err as { code?: unknown }).code;
		if (code === "ERR_BUFFER_TOO_LARGE" || err instanceof RangeError) {
			throw new BodyTooLargeError(MAX_PROFILE_BYTES);
		}
		const reason = err instanceof Error ? err.message : String(err);
		throw new Error(`could not gunzip pprof profile: ${reason}`);
	}
}

/**
 * Fetches GET /api/status from the target and returns a one-line sanitized
 * summary of the JSON body, at most MAX_STATUS_SUMMARY_LENGTH characters.
 * Any failure (network, non-2xx, oversize body, non-JSON) yields undefined;
 * this endpoint is optional on the target.
 */
export async function fetchTargetStatus(
	target: string,
): Promise<string | undefined> {
	try {
		const url = new URL(`${stripTrailingSlash(target)}/api/status`);
		const body = await fetchBody(url, STATUS_TIMEOUT_MS, MAX_STATUS_BYTES);
		const parsed: unknown = JSON.parse(Buffer.from(body).toString("utf8"));
		const summary = summarizeStatus(parsed);
		return summary.length > 0 ? summary : undefined;
	} catch {
		return undefined;
	}
}

async function fetchBody(
	url: URL,
	timeoutMs: number,
	limit: number,
): Promise<Uint8Array> {
	const controller = new AbortController();
	const signal = AbortSignal.any([
		controller.signal,
		AbortSignal.timeout(timeoutMs),
	]);
	let response: Response;
	try {
		response = await fetch(url, { redirect: "error", signal });
	} catch (err) {
		throw new Error(
			`request to ${url.pathname} failed: ${describeFetchError(err)}`,
		);
	}
	if (!response.ok) {
		const errBody = await readCapped(
			response,
			controller,
			MAX_ERROR_BODY_BYTES,
		).catch(() => new Uint8Array());
		const text = normalizeString(
			Buffer.from(errBody).toString("utf8").trim(),
			MAX_ERROR_TEXT_LENGTH,
		);
		throw new Error(
			`target returned HTTP ${response.status} for ${url.pathname}: ${text}`,
		);
	}
	return readCapped(response, controller, limit);
}

// Reads the response body chunk by chunk and aborts the request as soon as
// the running total passes the limit.
async function readCapped(
	response: Response,
	controller: AbortController,
	limit: number,
): Promise<Uint8Array> {
	if (response.body === null) {
		return new Uint8Array();
	}
	const reader = response.body.getReader();
	const chunks: Uint8Array[] = [];
	let total = 0;
	for (;;) {
		const { done, value } = await reader.read();
		if (done) {
			break;
		}
		total += value.byteLength;
		if (total > limit) {
			controller.abort();
			await reader.cancel().catch(() => undefined);
			throw new BodyTooLargeError(limit);
		}
		chunks.push(value);
	}
	return Buffer.concat(chunks);
}

function describeFetchError(err: unknown): string {
	if (err instanceof Error) {
		if (err.name === "TimeoutError") {
			return "timed out";
		}
		const cause = (err as { cause?: unknown }).cause;
		if (cause instanceof Error && cause.message) {
			return `${err.message} (${cause.message})`;
		}
		return err.message;
	}
	return String(err);
}

function isGzip(body: Uint8Array): boolean {
	return body.length >= 2 && body[0] === 0x1f && body[1] === 0x8b;
}

function stripTrailingSlash(target: string): string {
	return target.replace(/\/+$/, "");
}

// Produces "scenario parts; key=value; key=value": entries of a "scenarios"
// field (array or object keyed by name) come first, running ones as
// "name running (k=v, ...)" ahead of "name stopped", followed by the
// document's top-level scalars.
function summarizeStatus(status: unknown): string {
	if (!isRecord(status)) {
		return "";
	}
	const scenarios: string[] = [];
	const scalars: string[] = [];
	for (const [key, value] of Object.entries(status)) {
		if (key === "scenarios") {
			scenarios.push(...summarizeScenarios(value));
			continue;
		}
		if (isScalar(value)) {
			scalars.push(`${normalizeString(key)}=${scalarString(value)}`);
		}
	}
	return normalizeString(
		[...scenarios, ...scalars].join("; "),
		MAX_STATUS_SUMMARY_LENGTH,
	);
}

function summarizeScenarios(value: unknown): string[] {
	const entries: Array<[string, unknown]> = Array.isArray(value)
		? value.map((v, i) => [
				isRecord(v) && typeof v.name === "string" ? v.name : String(i),
				v,
			])
		: isRecord(value)
			? Object.entries(value)
			: [];
	const running: string[] = [];
	const stopped: string[] = [];
	for (const [name, entry] of entries) {
		if (!isRecord(entry)) {
			continue;
		}
		if (entry.running !== true) {
			stopped.push(`${normalizeString(name)} stopped`);
			continue;
		}
		const state = isRecord(entry.state) ? entry.state : {};
		const knobs: string[] = [];
		for (const [k, v] of Object.entries(state)) {
			if (isScalar(v)) {
				knobs.push(`${normalizeString(k)}=${scalarString(v)}`);
			}
		}
		const suffix = knobs.length > 0 ? ` (${knobs.join(", ")})` : "";
		running.push(`${normalizeString(name)} running${suffix}`);
	}
	return [...running, ...stopped];
}

function isRecord(value: unknown): value is Record<string, unknown> {
	return typeof value === "object" && value !== null && !Array.isArray(value);
}

type Scalar = string | number | boolean;

function isScalar(value: unknown): value is Scalar {
	return (
		typeof value === "string" ||
		typeof value === "number" ||
		typeof value === "boolean"
	);
}

function scalarString(value: Scalar): string {
	return typeof value === "string" ? normalizeString(value) : String(value);
}
