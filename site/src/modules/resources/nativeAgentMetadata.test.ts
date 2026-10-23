import {
	MockBlockedNativeAgentSessionInfo,
	MockNativeAgentSessionInfo,
} from "#/testHelpers/nativeAgent";
import { nativeAgentMetadata } from "./nativeAgentMetadata";

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

it.each([undefined, null, "true", 1, {}])(
	"ignores malformed setup metadata %s",
	(value) => {
		expect(
			nativeAgentMetadata({
				...MockNativeAgentSessionInfo,
				_meta: {
					"claude-acp-bridge": {
						registrationPending: value,
						blockedReason: value,
						mode: value,
					},
				},
			}),
		).toMatchObject({
			registrationPending: undefined,
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
		startedAt: 1_792_756_800_000,
		lastError: { message: "Tool failed", at: 1_792_757_100_000 },
		fault: false,
		ended: false,
		stale: true,
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
		startedAt: 0,
		lastError: { message: "A retained failure", at: undefined },
		fault: false,
		ended: false,
		stale: false,
		registrationPending: undefined,
		blockedReason: undefined,
		unavailable: false,
	});
});
