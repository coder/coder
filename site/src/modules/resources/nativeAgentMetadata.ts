import type { SessionInfo } from "@agentclientprotocol/sdk";

const isRecord = (value: unknown): value is Record<string, unknown> =>
	value !== null && typeof value === "object" && !Array.isArray(value);

const timestamp = (value: unknown): number | undefined =>
	typeof value === "number" &&
	Number.isFinite(value) &&
	value >= 0 &&
	value <= 8.64e15
		? value
		: undefined;

/** Reads only the known adapter extension; older adapters need not supply it. */
export function nativeAgentMetadata(info?: SessionInfo) {
	const value = info?._meta?.["claude-acp-bridge"];
	const meta = isRecord(value) ? value : {};
	const failure = meta.lastError;
	const lastError =
		isRecord(failure) &&
		typeof failure.message === "string" &&
		failure.message.trim()
			? { message: failure.message.slice(0, 8_000), at: timestamp(failure.at) }
			: undefined;
	const fault = meta.fault === true;
	const ended = meta.ended === true;
	const stale = meta.stale === true;
	const archived = meta.archived === true;
	return {
		startedAt: timestamp(meta.startedAt),
		lastError,
		fault,
		ended,
		stale,
		archived,
		resumable: meta.resumable === true,
		resumePending: meta.resumePending === true,
		registrationPending:
			typeof meta.registrationPending === "boolean"
				? meta.registrationPending
				: undefined,
		blockedReason:
			typeof meta.blockedReason === "string" && meta.blockedReason.trim()
				? meta.blockedReason.trim().slice(0, 1_000)
				: undefined,
		unavailable: fault || ended || stale || archived,
	};
}
