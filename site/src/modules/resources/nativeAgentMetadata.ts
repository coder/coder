import type { InitializeResponse, SessionInfo } from "@agentclientprotocol/sdk";

/** A native harness advertised by the workspace ACP adapter. */
export type NativeAgentHarness = { id: string; name: string; title: string };

const isRecord = (value: unknown): value is Record<string, unknown> =>
	value !== null && typeof value === "object" && !Array.isArray(value);

const timestamp = (value: unknown): number | undefined =>
	typeof value === "number" &&
	Number.isFinite(value) &&
	value >= 0 &&
	value <= 8.64e15
		? value
		: undefined;

/** Reads advertised harnesses without requiring the extension from older adapters. */
export function nativeAgentHarnesses(
	initialized: InitializeResponse,
): NativeAgentHarness[] {
	const meta = initialized._meta?.["coder-acp"];
	if (!isRecord(meta) || !Array.isArray(meta.harnesses)) return [];
	const harnesses: NativeAgentHarness[] = [];
	for (const item of meta.harnesses) {
		if (
			isRecord(item) &&
			typeof item.id === "string" &&
			item.id.trim() &&
			typeof item.name === "string" &&
			item.name.trim() &&
			typeof item.title === "string" &&
			item.title.trim() &&
			!harnesses.some((harness) => harness.id === item.id)
		)
			harnesses.push({ id: item.id, name: item.name, title: item.title });
	}
	return harnesses;
}

/** Resolves session identity before falling back to a single-harness adapter. */
export function nativeAgentIdentity(
	info: SessionInfo | undefined,
	agentInfo: InitializeResponse["agentInfo"],
	harnesses: readonly NativeAgentHarness[] = [],
) {
	const { harness } = nativeAgentMetadata(info);
	if (harness) {
		const declared = harnesses.find((item) => item.id === harness);
		if (declared) return declared;
		if (harness === "claude")
			return { name: "claude-acp-bridge", title: "Claude Code" };
		if (harness === "codex") return { name: "codex", title: "Codex" };
		if (harness === "opencode") return { name: "opencode", title: "OpenCode" };
		return { name: harness, title: harness };
	}
	const name = agentInfo?.name.trim() ?? "";
	return { name, title: agentInfo?.title?.trim() || name || "Native agent" };
}

/** The shared extension takes precedence over the older Claude adapter extension. */
export function nativeAgentMetadata(info?: SessionInfo) {
	const shared = info?._meta?.["coder-acp"];
	const value =
		shared !== undefined ? shared : info?._meta?.["claude-acp-bridge"];
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
		harness:
			typeof meta.harness === "string" && meta.harness.trim()
				? meta.harness.trim()
				: shared === undefined && isRecord(value)
					? "claude"
					: undefined,
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
