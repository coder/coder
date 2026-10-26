import { PROTOCOL_VERSION } from "@agentclientprotocol/sdk";
import {
	MockArchivedOpenCodeNativeAgentSessionInfo,
	MockBlockedNativeAgentSessionInfo,
	MockCodexNativeAgentSessionInfo,
	MockNativeAgentHarnesses,
	MockNativeAgentInfo,
	MockNativeAgentMultiplexerInfo,
	MockNativeAgentSessionInfo,
	MockResumableNativeAgentSessionInfo,
	MockResumePendingNativeAgentSessionInfo,
} from "#/testHelpers/nativeAgent";
import {
	nativeAgentHarnesses,
	nativeAgentIdentity,
	nativeAgentMetadata,
} from "./nativeAgentMetadata";

it("prefers shared diagnostics without mixing in legacy fields", () => {
	expect(
		nativeAgentMetadata({
			...MockCodexNativeAgentSessionInfo,
			_meta: {
				...MockCodexNativeAgentSessionInfo._meta,
				"claude-acp-bridge": {
					harness: "claude",
					fault: true,
					startedAt: 0,
					lastError: { message: "Old failure" },
				},
			},
		}),
	).toMatchObject({
		harness: "codex",
		fault: false,
		startedAt: 1_792_756_800_000,
		lastError: undefined,
	});
});

it.each([
	{ harness: "claude", name: "claude-acp-bridge", title: "Claude Code" },
	{ harness: "codex", name: "codex", title: "Codex" },
	{ harness: "opencode", name: "opencode", title: "OpenCode" },
	{
		harness: "unknown-harness",
		name: "unknown-harness",
		title: "unknown-harness",
	},
])(
	"uses the per-session harness $harness instead of the adapter identity",
	({ harness, name, title }) => {
		const mockSessionInfo = {
			...MockNativeAgentSessionInfo,
			_meta: { "coder-acp": { harness } },
		};
		expect(
			nativeAgentIdentity(mockSessionInfo, MockNativeAgentMultiplexerInfo),
		).toEqual({ name, title });
	},
);

it.each([
	undefined,
	[],
	MockNativeAgentHarnesses.filter((item) => item.id !== "opencode"),
	MockNativeAgentHarnesses,
])(
	"keeps archived OpenCode identity independent of advertised harnesses: %s",
	(harnesses) => {
		expect(
			nativeAgentIdentity(
				MockArchivedOpenCodeNativeAgentSessionInfo,
				MockNativeAgentInfo,
				harnesses,
			),
		).toMatchObject({ name: "opencode", title: "OpenCode" });
		expect(
			nativeAgentMetadata(MockArchivedOpenCodeNativeAgentSessionInfo),
		).toMatchObject({
			harness: "opencode",
			archived: true,
			resumable: true,
			unavailable: true,
		});
	},
);

it("keeps old single-harness identities and legacy Claude metadata compatible", () => {
	expect(nativeAgentIdentity(undefined, MockNativeAgentInfo)).toEqual({
		name: "claude-acp-bridge",
		title: "Claude Code",
	});
	expect(
		nativeAgentIdentity(
			MockNativeAgentSessionInfo,
			MockNativeAgentMultiplexerInfo,
		),
	).toEqual({ name: "claude-acp-bridge", title: "Claude Code" });
});

it("validates advertised harnesses and ignores duplicates and malformed entries", () => {
	expect(
		nativeAgentHarnesses({
			protocolVersion: PROTOCOL_VERSION,
			_meta: {
				"coder-acp": {
					harnesses: [
						...MockNativeAgentHarnesses,
						...MockNativeAgentHarnesses,
						null,
						"codex",
						{ id: "other" },
						{ id: "", name: "bad", title: "Bad" },
					],
				},
			},
		}),
	).toEqual(MockNativeAgentHarnesses);
	expect(nativeAgentHarnesses({ protocolVersion: PROTOCOL_VERSION })).toEqual(
		[],
	);
});

it.each([
	undefined,
	null,
	"2026-10-23",
	-1,
	Number.NaN,
	Number.POSITIVE_INFINITY,
	9e15,
])(
	"does not invent a start from a heartbeat or invalid timestamp %s",
	(startedAt) => {
		expect(
			nativeAgentMetadata({
				...MockNativeAgentSessionInfo,
				_meta: {
					"claude-acp-bridge": { startedAt, lastSeen: 1_792_757_400_000 },
				},
			}).startedAt,
		).toBeUndefined();
	},
);

it("keeps pending native setup joinable rather than inventing a fault or stale heartbeat", () => {
	expect(nativeAgentMetadata(MockBlockedNativeAgentSessionInfo)).toMatchObject({
		registrationPending: true,
		blockedReason: "Approve project MCP servers in the native terminal.",
		fault: false,
		ended: false,
		stale: false,
		unavailable: false,
	});
});

it.each([
	{
		info: MockResumableNativeAgentSessionInfo,
		expected: {
			archived: true,
			resumable: true,
			resumePending: false,
			unavailable: true,
		},
	},
	{
		info: MockResumePendingNativeAgentSessionInfo,
		expected: {
			archived: false,
			resumable: false,
			resumePending: true,
			unavailable: false,
		},
	},
])("parses saved native resume metadata $expected", ({ info, expected }) => {
	expect(nativeAgentMetadata(info)).toMatchObject(expected);
});

it.each([undefined, null, "true", 1, {}])(
	"ignores malformed setup metadata %s",
	(value) => {
		expect(
			nativeAgentMetadata({
				...MockNativeAgentSessionInfo,
				_meta: {
					"claude-acp-bridge": {
						registrationPending: value,
						archived: value,
						resumable: value,
						resumePending: value,
						blockedReason: value,
						mode: value,
					},
				},
			}),
		).toMatchObject({
			registrationPending: undefined,
			archived: false,
			resumable: false,
			resumePending: false,
			blockedReason: typeof value === "string" ? value : undefined,
		});
	},
);

it("keeps a recoverable error separate from fault and heartbeat availability", () => {
	expect(
		nativeAgentMetadata({
			...MockNativeAgentSessionInfo,
			_meta: {
				"claude-acp-bridge": {
					startedAt: 1_792_756_800_000,
					lastError: { message: "Tool failed", at: 1_792_757_100_000 },
					fault: false,
					stale: true,
				},
			},
		}),
	).toEqual({
		harness: "claude",
		startedAt: 1_792_756_800_000,
		lastError: { message: "Tool failed", at: 1_792_757_100_000 },
		fault: false,
		ended: false,
		stale: true,
		archived: false,
		resumable: false,
		resumePending: false,
		registrationPending: undefined,
		blockedReason: undefined,
		unavailable: true,
	});
});

it("ignores unknown metadata and rejects malformed extension fields independently", () => {
	expect(
		nativeAgentMetadata({
			...MockNativeAgentSessionInfo,
			_meta: {
				"another-adapter": { startedAt: 123, fault: true },
				"claude-acp-bridge": {
					startedAt: 0,
					fault: "true",
					stale: 1,
					lastError: { message: "A retained failure", at: "yesterday" },
				},
			},
		}),
	).toEqual({
		harness: "claude",
		startedAt: 0,
		lastError: { message: "A retained failure", at: undefined },
		fault: false,
		ended: false,
		stale: false,
		archived: false,
		resumable: false,
		resumePending: false,
		registrationPending: undefined,
		blockedReason: undefined,
		unavailable: false,
	});
});
