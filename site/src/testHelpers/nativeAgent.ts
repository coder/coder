import type {
	Implementation,
	SessionInfo,
	SessionModeState,
} from "@agentclientprotocol/sdk";
import type { NativeAgentSessionState } from "#/modules/resources/NativeAgentSession";

/** An existing workspace-native session returned by ACP session/list. */
export const MockNativeAgentSessionInfo: SessionInfo = {
	sessionId: "00c083d0-b72a-4a12-853b-270bf30de904",
	cwd: "/home/coder/project",
	title: "Investigate the failing tests",
	_meta: {
		"claude-acp-bridge": {
			startedAt: 1_792_756_800_000,
			ended: false,
			fault: false,
			stale: false,
		},
	},
};

/** A real native session waiting for onboarding before runtime registration. */
export const MockBlockedNativeAgentSessionInfo: SessionInfo = {
	...MockNativeAgentSessionInfo,
	title: "Set up the project agent",
	_meta: {
		"claude-acp-bridge": {
			startedAt: 1_792_756_800_000,
			registrationPending: true,
			blockedReason: "Approve project MCP servers in the native terminal.",
			mode: "human",
			ended: false,
			fault: false,
			stale: false,
		},
	},
};

/** The same UUID after native registration, still under human control. */
export const MockRegisteredNativeAgentSessionInfo: SessionInfo = {
	...MockBlockedNativeAgentSessionInfo,
	_meta: {
		"claude-acp-bridge": {
			startedAt: 1_792_756_800_000,
			registrationPending: false,
			mode: "human",
			ended: false,
			fault: false,
			stale: false,
		},
	},
};

/** Saved native history after its runtime stopped, resumable only on explicit Join. */
export const MockResumableNativeAgentSessionInfo: SessionInfo = {
	...MockNativeAgentSessionInfo,
	_meta: {
		"claude-acp-bridge": {
			startedAt: 1_792_756_800_000,
			archived: true,
			resumable: true,
			resumePending: false,
			registrationPending: false,
			ended: true,
			fault: false,
			stale: false,
		},
	},
};

/** Join prepared the saved UUID; the native terminal has not registered yet. */
export const MockResumePendingNativeAgentSessionInfo: SessionInfo = {
	...MockNativeAgentSessionInfo,
	_meta: {
		"claude-acp-bridge": {
			startedAt: 1_792_756_800_000,
			archived: false,
			resumable: false,
			resumePending: true,
			registrationPending: true,
			mode: "human",
			ended: false,
			fault: false,
			stale: false,
		},
	},
};

/** Fixed reference time for relative session timestamps in visual stories. */
export const MockNativeAgentNow = 1_792_757_400_000;

/** A stopped session whose integrity fault remains inspectable. */
export const MockFaultedNativeAgentSessionInfo: SessionInfo = {
	...MockNativeAgentSessionInfo,
	sessionId: "2bd856e4-d93a-46b6-967d-d13a21bd204b",
	title: "Inspect an interrupted session",
	_meta: {
		"claude-acp-bridge": {
			startedAt: 1_792_752_000_000,
			ended: true,
			fault: true,
			stale: false,
			lastError: {
				message:
					"The retained event stream is incomplete. Session integrity could not be verified.",
				at: 1_792_757_100_000,
			},
		},
	},
};

/** A session with an old heartbeat, not a confirmed fault. */
export const MockStaleNativeAgentSessionInfo: SessionInfo = {
	...MockNativeAgentSessionInfo,
	sessionId: "f824e06a-094d-4dcd-bf67-556f047fd82a",
	title: "Check runtime availability",
	_meta: {
		"claude-acp-bridge": {
			startedAt: 1_792_756_800_000,
			ended: false,
			fault: false,
			stale: true,
		},
	},
};

/** A running session with a retained recoverable tool failure. */
export const MockRecoveredNativeAgentSessionInfo: SessionInfo = {
	...MockNativeAgentSessionInfo,
	sessionId: "a10c6abd-a7b0-4212-bf23-9d796fb14c6c",
	title: "Continue after a tool error",
	_meta: {
		"claude-acp-bridge": {
			startedAt: 1_792_756_800_000,
			ended: false,
			fault: false,
			stale: false,
			lastError: {
				message: "Shell command exited with code 1: missing input fixture.",
				at: 1_792_757_100_000,
			},
		},
	},
};

/** Identity supplied by the Claude ACP adapter during initialization. */
export const MockNativeAgentInfo: Implementation = {
	name: "claude-acp-bridge",
	title: "Claude Code",
	version: "0.1.0",
};

/** Another running session in the same workspace. */
export const MockOtherNativeAgentSessionInfo: SessionInfo = {
	...MockNativeAgentSessionInfo,
	sessionId: "4a7ebd77-9156-489c-ae77-a82080a15a00",
	title: "Inspect the next failure",
};

/** Native terminal and ACP control modes with automation selected. */
export const MockNativeAgentModes: SessionModeState = {
	currentModeId: "automation",
	availableModes: [
		{ id: "human", name: "Human", description: "Use the native terminal." },
		{
			id: "automation",
			name: "Automation",
			description: "Submit ACP prompts.",
		},
	],
};

/** Only native terminal control is available before registration. */
export const MockPendingNativeAgentModes: SessionModeState = {
	...MockNativeAgentModes,
	currentModeId: "human",
	availableModes: MockNativeAgentModes.availableModes.filter(
		(mode) => mode.id === "human",
	),
};

/** A loaded native session with retained user, agent, and tool events. */
export const MockNativeAgentSession: NativeAgentSessionState = {
	info: MockNativeAgentSessionInfo,
	modes: MockNativeAgentModes,
	entries: [
		{ id: 1, kind: "user", text: "Find out why the unit tests are failing." },
		{
			id: 2,
			kind: "agent",
			text: "I will run the targeted suite and inspect the first failure.",
		},
		{
			id: 3,
			kind: "tool",
			toolCallID: "test-suite",
			title: "Run unit tests",
			status: "completed",
		},
		{
			id: 4,
			kind: "agent",
			text: "The first failure is in the parser. I am checking the input fixture.",
		},
	],
	truncated: false,
	needsInput: false,
	prompting: false,
};

/** Pending sessions load successfully before they have a runtime or transcript. */
export const MockBlockedNativeAgentSession: NativeAgentSessionState = {
	...MockNativeAgentSession,
	info: MockBlockedNativeAgentSessionInfo,
	modes: MockPendingNativeAgentModes,
	entries: [],
};
