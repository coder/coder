import type { Implementation, SessionInfo } from "@agentclientprotocol/sdk";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, userEvent, within } from "storybook/test";
import {
	MockBlockedNativeAgentSession,
	MockBlockedNativeAgentSessionInfo,
	MockFaultedNativeAgentSessionInfo,
	MockNativeAgentInfo,
	MockNativeAgentModes,
	MockNativeAgentNow,
	MockNativeAgentSession,
	MockNativeAgentSessionInfo,
	MockOtherNativeAgentSessionInfo,
	MockRecoveredNativeAgentSessionInfo,
	MockRegisteredNativeAgentSessionInfo,
	MockResumableNativeAgentSessionInfo,
	MockResumePendingNativeAgentSessionInfo,
	MockStaleNativeAgentSessionInfo,
} from "#/testHelpers/nativeAgent";
import { pixelWithPhone } from "#/testHelpers/pixel";
import { NativeAgentPanelView } from "./NativeAgentPanelView";
import type { NativeAgentSessionState } from "./NativeAgentSession";

const mockStreamingSession: NativeAgentSessionState = {
	...MockNativeAgentSession,
	prompting: true,
	entries: [
		...MockNativeAgentSession.entries,
		{
			id: 5,
			kind: "thought",
			text: "The parser expects a **non-empty input**. I should compare `fixtures/input.json` with the test setup before changing the implementation.",
		},
	],
};
const mockNativeDialogSession: NativeAgentSessionState = {
	...mockStreamingSession,
	needsInput: true,
};
const mockHumanSession: NativeAgentSessionState = {
	...MockNativeAgentSession,
	modes: { ...MockNativeAgentModes, currentModeId: "human" },
};
const mockTruncatedSession: NativeAgentSessionState = {
	...MockNativeAgentSession,
	truncated: true,
};
const mockEmptySession: NativeAgentSessionState = {
	...MockNativeAgentSession,
	entries: [],
};
const mockFailedSession: NativeAgentSessionState = {
	...MockNativeAgentSession,
	error: "The retained event stream is incomplete.",
};
const mockLongTokenSession: NativeAgentSessionState = {
	...MockNativeAgentSession,
	entries: [
		{
			id: 1,
			kind: "agent",
			text: `Log token: ${"0123456789abcdef".repeat(100)}`,
		},
	],
};
const mockMarkdownSession: NativeAgentSessionState = {
	...MockNativeAgentSession,
	entries: [
		{
			id: 1,
			kind: "user",
			text: "Check **parser** and `fixtures/input.json`.",
		},
		{
			id: 2,
			kind: "thought",
			text: "Compare the *input* with the expected schema.",
		},
		{
			id: 3,
			kind: "agent",
			text: "### Findings\n\n- **Cause:** empty `input`.\n- *Next:* update the [fixture](https://example.com/fixture).\n\n> Keep the parser unchanged.",
		},
	],
};
const mockMarkdownCodeSession: NativeAgentSessionState = {
	...MockNativeAgentSession,
	entries: [
		{
			id: 1,
			kind: "agent",
			text: [
				"```sh\npnpm test parser.test.ts\n```",
				"```\nPASS parser.test.ts\n  12 tests passed\n```",
				"| Suite | Result |\n| --- | --- |\n| Parser | Passed |",
				"![Test artifact](https://example.com/artifact.png)",
				"[![Test report](https://example.com/report.png)](https://example.com/report)",
			].join("\n\n"),
		},
	],
};
const mockStreamingMarkdownSession: NativeAgentSessionState = {
	...mockStreamingSession,
	entries: [
		{
			id: 1,
			kind: "thought",
			text: "Checking the **long command** before running it.",
		},
		{
			id: 2,
			kind: "agent",
			text: `Work in progress:\n\n\`\`\`sh\nnode scripts/inspect.js --token=${"0123456789abcdef".repeat(10)}\n  --verbose`,
		},
	],
};
const mockModesUnavailableSession: NativeAgentSessionState = {
	...MockNativeAgentSession,
	modes: undefined,
};

const mockUnknownStartSessionInfo: SessionInfo = {
	...MockOtherNativeAgentSessionInfo,
	title: "Older session with unknown start",
	_meta: { "claude-acp-bridge": { lastSeen: MockNativeAgentNow } },
};
const mockFaultedSession: NativeAgentSessionState = {
	...MockNativeAgentSession,
	info: MockFaultedNativeAgentSessionInfo,
	modes: undefined,
	entries: [],
	error: "Session integrity fault: retained events could not be loaded.",
};
const mockFaultedSetupSession: NativeAgentSessionState = {
	...MockBlockedNativeAgentSession,
	info: {
		...MockBlockedNativeAgentSessionInfo,
		_meta: { "claude-acp-bridge": { registrationPending: true, fault: true } },
	},
};
const mockStaleSession: NativeAgentSessionState = {
	...MockNativeAgentSession,
	info: MockStaleNativeAgentSessionInfo,
};
const mockRecoveredSession: NativeAgentSessionState = {
	...MockNativeAgentSession,
	info: MockRecoveredNativeAgentSessionInfo,
};
const mockStartingSessionInfo: SessionInfo = {
	...MockBlockedNativeAgentSessionInfo,
	_meta: {
		"claude-acp-bridge": {
			registrationPending: true,
			startedAt: MockNativeAgentNow,
			mode: "human",
		},
	},
};
const mockRegisteredSession: NativeAgentSessionState = {
	...MockBlockedNativeAgentSession,
	info: MockRegisteredNativeAgentSessionInfo,
	modes: { ...MockNativeAgentModes, currentModeId: "human" },
};
const mockEndedSessionInfo: SessionInfo = {
	...MockNativeAgentSessionInfo,
	_meta: { "claude-acp-bridge": { ended: true } },
};
const mockEndedSession: NativeAgentSessionState = {
	...MockNativeAgentSession,
	info: mockEndedSessionInfo,
};
const mockCodexInfo: Implementation = {
	...MockNativeAgentInfo,
	name: "codex",
	title: "Codex",
};
const mockUnknownClaudeInfo: Implementation = {
	...MockNativeAgentInfo,
	name: "claude-unknown-fork",
	title: "Unknown Claude fork",
};
const mockProviderInfo: Implementation = {
	...MockNativeAgentInfo,
	name: "anthropic",
	title: "Unknown agent",
};
const mockUnknownInfo: Implementation = {
	...MockNativeAgentInfo,
	name: "unknown-harness",
	title: undefined,
};

const meta = {
	title: "modules/resources/NativeAgentPanelView",
	component: NativeAgentPanelView,
	args: {
		agentStatus: "connected",
		agentReady: true,
		snapshot: {
			status: "connected",
			agentInfo: MockNativeAgentInfo,
			session: MockNativeAgentSession,
		},
		sessions: [MockNativeAgentSessionInfo],
		sessionExpanded: true,
		loadingSessions: false,
		refreshingSessions: false,
		busy: false,
		hasMoreSessions: false,
		cwd: MockNativeAgentSessionInfo.cwd,
		prompt: "",
		now: MockNativeAgentNow,
		onCwdChange: fn(),
		onPromptChange: fn(),
		onCreateSubmit: fn(),
		onPromptSubmit: fn(),
		onAction: fn(),
		onRefresh: fn(),
		onMoreSessions: fn(),
	},
	parameters: { layout: "padded" },
} satisfies Meta<typeof NativeAgentPanelView>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Connected: Story = {};
export const CollapsedSession: Story = {
	args: { sessionExpanded: false, prompt: "Keep my unsent draft." },
	parameters: { pixel: { matrix: pixelWithPhone } },
};
export const Discovered: Story = {
	args: {
		snapshot: { status: "connected", agentInfo: MockNativeAgentInfo },
		sessions: [MockNativeAgentSessionInfo, MockOtherNativeAgentSessionInfo],
	},
	parameters: { pixel: { matrix: pixelWithPhone } },
};
export const SessionStatuses: Story = {
	args: {
		snapshot: { status: "connected", agentInfo: MockNativeAgentInfo },
		sessions: [
			MockNativeAgentSessionInfo,
			MockFaultedNativeAgentSessionInfo,
			MockStaleNativeAgentSessionInfo,
			MockRecoveredNativeAgentSessionInfo,
			mockUnknownStartSessionInfo,
		],
	},
	parameters: { pixel: { matrix: pixelWithPhone } },
};
export const BlockedSetupDiscovered: Story = {
	args: {
		snapshot: { status: "connected", agentInfo: MockNativeAgentInfo },
		sessions: [
			MockBlockedNativeAgentSessionInfo,
			MockOtherNativeAgentSessionInfo,
		],
	},
	parameters: { pixel: { matrix: pixelWithPhone } },
};
export const BlockedSetupSelected: Story = {
	args: {
		snapshot: {
			status: "connected",
			agentInfo: MockNativeAgentInfo,
			session: MockBlockedNativeAgentSession,
		},
		sessions: [MockBlockedNativeAgentSessionInfo],
		prompt: "Keep my unsent draft until setup is complete.",
	},
	parameters: { pixel: { matrix: pixelWithPhone } },
};
export const BlockedSetupCollapsed: Story = {
	args: { ...BlockedSetupSelected.args, sessionExpanded: false },
	parameters: { pixel: { matrix: pixelWithPhone } },
};
export const BlockedSetupJoining: Story = {
	args: { ...BlockedSetupSelected.args, busy: true },
	parameters: { pixel: { matrix: pixelWithPhone } },
};
export const BlockedSetupJoinFailed: Story = {
	args: {
		...BlockedSetupSelected.args,
		error: "Human mode was not acknowledged.",
	},
};
export const BlockedSetupFaulted: Story = {
	args: {
		snapshot: {
			status: "connected",
			agentInfo: MockNativeAgentInfo,
			session: mockFaultedSetupSession,
		},
		sessions: [mockFaultedSetupSession.info],
		prompt: "Keep this draft while inspecting the fault.",
	},
	parameters: { pixel: { matrix: pixelWithPhone } },
};
export const NativeSessionStarting: Story = {
	args: {
		snapshot: { status: "connected", agentInfo: MockNativeAgentInfo },
		sessions: [mockStartingSessionInfo],
	},
	parameters: { pixel: { matrix: pixelWithPhone } },
};
export const NativeSessionRegistered: Story = {
	args: {
		snapshot: {
			status: "connected",
			agentInfo: MockNativeAgentInfo,
			session: mockRegisteredSession,
		},
		sessions: [MockRegisteredNativeAgentSessionInfo],
		prompt: "Keep my unsent draft until setup is complete.",
	},
	parameters: { pixel: { matrix: pixelWithPhone } },
};
export const BlockedSetupJoinKeyboardFocus: Story = {
	args: BlockedSetupDiscovered.args,
	play: async ({ canvasElement }) => {
		within(canvasElement)
			.getByRole("button", { name: "Join Set up the project agent" })
			.focus();
	},
};
export const FaultedSessionLoadFailed: Story = {
	args: {
		sessions: [MockNativeAgentSessionInfo, MockFaultedNativeAgentSessionInfo],
		snapshot: {
			status: "connected",
			agentInfo: MockNativeAgentInfo,
			session: mockFaultedSession,
		},
	},
	parameters: { pixel: { matrix: pixelWithPhone } },
};
export const StaleSession: Story = {
	args: {
		sessions: [MockStaleNativeAgentSessionInfo],
		snapshot: {
			status: "connected",
			agentInfo: MockNativeAgentInfo,
			session: mockStaleSession,
		},
	},
	parameters: { pixel: { matrix: pixelWithPhone } },
};
export const RecoverableToolError: Story = {
	args: {
		sessions: [MockRecoveredNativeAgentSessionInfo],
		snapshot: {
			status: "connected",
			agentInfo: MockNativeAgentInfo,
			session: mockRecoveredSession,
		},
		prompt: "Continue after fixing the input fixture.",
	},
	parameters: { pixel: { matrix: pixelWithPhone } },
};
export const EndedSession: Story = {
	args: {
		sessions: [mockEndedSessionInfo],
		snapshot: {
			status: "connected",
			agentInfo: MockNativeAgentInfo,
			session: mockEndedSession,
		},
	},
};
export const StoppedSavedSession: Story = {
	args: {
		sessions: [MockResumableNativeAgentSessionInfo],
		snapshot: {
			status: "connected",
			agentInfo: MockNativeAgentInfo,
			session: {
				...mockHumanSession,
				info: MockResumableNativeAgentSessionInfo,
			},
		},
		prompt: "Keep my draft while resuming the saved conversation.",
	},
	parameters: { pixel: { matrix: pixelWithPhone } },
};
export const StoppedSavedSessionCollapsed: Story = {
	args: { ...StoppedSavedSession.args, sessionExpanded: false },
	parameters: { pixel: { matrix: pixelWithPhone } },
};
export const SavedSessionResumePending: Story = {
	args: {
		sessions: [MockResumePendingNativeAgentSessionInfo],
		snapshot: {
			status: "connected",
			agentInfo: MockNativeAgentInfo,
			session: {
				...mockHumanSession,
				info: MockResumePendingNativeAgentSessionInfo,
			},
		},
		prompt: "Keep my draft until registration and Observe.",
	},
	parameters: { pixel: { matrix: pixelWithPhone } },
};
export const SavedSessionResumePendingCollapsed: Story = {
	args: { ...SavedSessionResumePending.args, sessionExpanded: false },
	parameters: { pixel: { matrix: pixelWithPhone } },
};
export const SavedSessionJoinFailed: Story = {
	args: {
		...StoppedSavedSession.args,
		error: "The saved native conversation could not be resumed.",
	},
};
export const WaitingForStartup: Story = {
	args: {
		agentReady: false,
		snapshot: { status: "disconnected" },
		sessions: [],
	},
};
export const Codex: Story = {
	args: {
		snapshot: {
			status: "connected",
			agentInfo: mockCodexInfo,
		},
	},
};
export const UnknownClaudeFork: Story = {
	args: {
		snapshot: {
			status: "connected",
			agentInfo: mockUnknownClaudeInfo,
		},
	},
};
export const ProviderNameIsNotAgentIdentity: Story = {
	args: {
		snapshot: {
			status: "connected",
			agentInfo: mockProviderInfo,
		},
	},
};
export const UnknownAgent: Story = {
	args: {
		snapshot: {
			status: "connected",
			agentInfo: mockUnknownInfo,
		},
	},
};
export const ConnectedBusy: Story = { args: { busy: true } };
export const Setup: Story = {
	args: { snapshot: { status: "disconnected" }, sessions: [] },
};
export const Connecting: Story = {
	args: { snapshot: { status: "connecting" }, busy: true, sessions: [] },
};
export const LoadingSessions: Story = {
	args: {
		snapshot: { status: "connected" },
		sessions: [],
		loadingSessions: true,
		refreshingSessions: true,
	},
};
export const Empty: Story = {
	args: { snapshot: { status: "connected" }, sessions: [] },
};
export const EmptySelectedSession: Story = {
	args: { snapshot: { status: "connected", session: mockEmptySession } },
};
export const Refreshing: Story = {
	args: {
		refreshingSessions: true,
		prompt: "Keep this draft while refreshing sessions.",
	},
};
export const Pagination: Story = {
	args: {
		sessions: [MockNativeAgentSessionInfo, MockOtherNativeAgentSessionInfo],
		hasMoreSessions: true,
	},
};
export const MobilePagination: Story = {
	args: {
		sessions: [MockNativeAgentSessionInfo, MockOtherNativeAgentSessionInfo],
		hasMoreSessions: true,
	},
	parameters: { pixel: { matrix: pixelWithPhone } },
	globals: { viewport: { value: "iphone12", isRotated: false } },
};
export const SessionListError: Story = {
	args: { sessionsError: "The adapter could not list sessions." },
};
export const SessionError: Story = {
	args: { snapshot: { status: "connected", session: mockFailedSession } },
};
export const AdapterMissing: Story = {
	args: {
		snapshot: {
			status: "disconnected",
			error:
				"Workspace ACP adapter exited. Check coder-acp setup in the workspace.",
		},
		sessions: [],
	},
};
export const Streaming: Story = {
	args: {
		snapshot: {
			status: "connected",
			agentInfo: MockNativeAgentInfo,
			session: mockStreamingSession,
		},
	},
};
export const Markdown: Story = {
	args: { snapshot: { status: "connected", session: mockMarkdownSession } },
	parameters: { pixel: { matrix: pixelWithPhone } },
};
export const MarkdownCodeAndTable: Story = {
	args: { snapshot: { status: "connected", session: mockMarkdownCodeSession } },
	parameters: { pixel: { matrix: pixelWithPhone } },
};
export const StreamingMarkdown: Story = {
	args: {
		snapshot: { status: "connected", session: mockStreamingMarkdownSession },
	},
	parameters: { pixel: { matrix: pixelWithPhone } },
};
export const ModesUnavailable: Story = {
	args: {
		snapshot: { status: "connected", session: mockModesUnavailableSession },
	},
};
export const NativeDialog: Story = {
	args: { snapshot: { status: "connected", session: mockNativeDialogSession } },
};
export const HumanControl: Story = {
	args: { snapshot: { status: "connected", session: mockHumanSession } },
};
export const Reconnect: Story = {
	args: {
		snapshot: {
			status: "disconnected",
			session: MockNativeAgentSession,
			error:
				"ACP disconnected. The native session may still be running. Reconnect to reload retained history; prompts are never resent.",
		},
		prompt: "Preserve my unsent draft.",
	},
};
export const WorkspaceAgentOffline: Story = {
	args: {
		agentStatus: "disconnected",
		agentReady: false,
		snapshot: { status: "disconnected", session: MockNativeAgentSession },
		cwd: "/home/coder/another-project",
		prompt: "Preserve this unsent draft until the workspace agent reconnects.",
	},
};
export const Truncated: Story = {
	args: { snapshot: { status: "connected", session: mockTruncatedSession } },
};
export const LongToken: Story = {
	args: { snapshot: { status: "connected", session: mockLongTokenSession } },
};
export const Mobile: Story = {
	parameters: { pixel: { matrix: pixelWithPhone } },
	globals: { viewport: { value: "iphone12", isRotated: false } },
};
export const CreateSession: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Add session" }),
		);
	},
};
export const SessionKeyboardFocus: Story = {
	play: async ({ canvasElement }) => {
		within(canvasElement)
			.getByRole("button", { name: /Investigate the failing tests/ })
			.focus();
	},
};
export const KeyboardFocus: Story = {
	play: async ({ canvasElement }) => {
		within(canvasElement).getByRole("textbox", { name: "Prompt" }).focus();
	},
};
export const TranscriptKeyboardFocus: Story = {
	play: async ({ canvasElement }) => {
		within(canvasElement).getByRole("textbox", { name: "Prompt" }).focus();
		await userEvent.tab({ shift: true });
	},
};
