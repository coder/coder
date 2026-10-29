// PROTOTYPE: the collapsed live working block peek, replayed through the real
// timeline and scroller.
import type { Meta, StoryObj } from "@storybook/react-vite";
import {
	PauseIcon,
	PlayIcon,
	RotateCcwIcon,
	StepForwardIcon,
} from "lucide-react";
import { useEffect, useState } from "react";
import { defaultUrlTransform } from "streamdown";
import { preferenceSettingsKey } from "#/api/queries/users";
import type * as TypesGen from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	MockChatMessage,
	MockMCPServerConfig,
} from "#/testHelpers/chatEntities";
import { MessageScroller } from "#/vendor/message-scroller";
import { ChatMessageScroller } from "../ChatMessageScroller";
import { ConversationTimeline } from "./ConversationTimeline";
import { parseMessagesWithMergedTools } from "./messageParsing";
import {
	buildStreamRenderState,
	FIXTURE_NOW,
	MockCollapsedStepsPreferences,
	pinFixtureClock,
} from "./storyFixtures";

const LinearMCPServer: TypesGen.MCPServerConfig = {
	...MockMCPServerConfig,
	id: "linear-mcp",
	slug: "linear",
	display_name: "Linear",
	icon_url: "/icon/linear.svg",
};

const subagentTitles = new Map([["relay-fix-chat", "Relay backoff fix"]]);

const reasoning = (title: string, body: string): TypesGen.ChatMessagePart => ({
	type: "reasoning",
	text: `**${title}**\n\n${body}`,
});

const call = (
	id: string,
	name: string,
	args: Record<string, string>,
	extra: Partial<TypesGen.ChatToolCallPart> = {},
): TypesGen.ChatMessagePart => ({
	type: "tool-call",
	tool_call_id: id,
	tool_name: name,
	args,
	...extra,
});

/** One server-side change and how long it stays on screen at 1x. */
type SimEvent = { ms: number } & (
	| { type: "start" }
	| { type: "prompt" }
	| { type: "stream"; parts: TypesGen.ChatMessagePart[] }
	| { type: "persist" }
	| { type: "result"; id: string; name: string; result: unknown }
	| { type: "finish" }
);

/** A turn shaped like the reported one: waits, lookups, parallel tools. */
const script: readonly SimEvent[] = [
	{ type: "start", ms: 900 },
	{ type: "prompt", ms: 1200 },
	{
		type: "stream",
		ms: 1100,
		parts: [
			reasoning(
				"Re-reading the ledger",
				"Start from what is already recorded before waiting on the agents.",
			),
		],
	},
	{
		type: "stream",
		ms: 600,
		parts: [call("read", "read_file", { path: "/home/coder/ledger.md" })],
	},
	{ type: "persist", ms: 700 },
	{
		type: "result",
		ms: 500,
		id: "read",
		name: "read_file",
		result: { content: "# Ledger\n\n- relay backoff: filed\n" },
	},
	{
		type: "stream",
		ms: 1000,
		parts: [
			reasoning(
				"Checking on the relay agent",
				"The fix agent was still running at the last check.",
			),
		],
	},
	{
		type: "stream",
		ms: 500,
		parts: [call("wait", "wait_agent", { chat_id: "relay-fix-chat" })],
	},
	{ type: "persist", ms: 3200 },
	{
		type: "result",
		ms: 400,
		id: "wait",
		name: "wait_agent",
		result: { chat_id: "relay-fix-chat", status: "completed" },
	},
	{
		type: "stream",
		ms: 900,
		parts: [
			reasoning(
				"Looking up the backoff issue",
				"Confirm the issue still matches what the agent fixed.",
			),
		],
	},
	{
		type: "stream",
		ms: 400,
		parts: [
			call(
				"issue",
				"linear__get_issue",
				{
					id: "CODAGT-1234",
					model_intent: "checking the filed relay backoff issue",
				},
				{ mcp_server_config_id: LinearMCPServer.id },
			),
		],
	},
	{ type: "persist", ms: 1200 },
	{
		type: "result",
		ms: 400,
		id: "issue",
		name: "linear__get_issue",
		result: { title: "Relay backoff" },
	},
	{
		type: "stream",
		ms: 900,
		parts: [
			reasoning(
				"Updating the ledger and polling",
				"That timing sequence checks out. Record it and keep waiting.",
			),
		],
	},
	{
		type: "stream",
		ms: 500,
		parts: [
			call("write", "write_file", {
				path: "/home/coder/ledger.md",
				content: "# Ledger\n\n- relay backoff: verified\n",
			}),
			call(
				"poll",
				"execute",
				{ command: "cd ~/coder && chatctl wait --any; date; python3 poll.py" },
				{ parsed_commands: [["cd"], ["chatctl"], ["date"], ["python3"]] },
			),
		],
	},
	{ type: "persist", ms: 1400 },
	{
		type: "result",
		ms: 1600,
		id: "write",
		name: "write_file",
		result: { ok: true },
	},
	{
		type: "result",
		ms: 400,
		id: "poll",
		name: "execute",
		result: { output: "no new events", exit_code: 0, wall_duration_ms: 3000 },
	},
	{
		type: "stream",
		ms: 800,
		parts: [
			reasoning(
				"Running the backoff tests",
				"Verify the change before reporting it.",
			),
		],
	},
	{
		type: "stream",
		ms: 400,
		parts: [
			call(
				"test",
				"execute",
				{ command: "go test ./coderd/x/chatd/relay/..." },
				{ parsed_commands: [["go", "test"]] },
			),
		],
	},
	{ type: "persist", ms: 2600 },
	{
		type: "result",
		ms: 400,
		id: "test",
		name: "execute",
		result: { output: "ok", exit_code: 0, wall_duration_ms: 2600 },
	},
	{
		type: "stream",
		ms: 1600,
		parts: [
			reasoning("Summarizing", "Everything checks out."),
			{
				type: "text",
				text: "The relay backoff fix is verified: the tests pass and the ledger is up to date.",
			},
		],
	},
	{ type: "finish", ms: 3500 },
];

type SimState = {
	messages: TypesGen.ChatMessage[];
	stream: TypesGen.ChatMessagePart[] | null;
	pending: ReadonlySet<string>;
	status: TypesGen.ChatStatus;
};

const historyAt = new Date(FIXTURE_NOW - 3_600_000).toISOString();

/**
 * An earlier exchange, so the prompt arrives after the scroller mounts, as it
 * does when the reader sends it, and anchors the new turn.
 */
const initialState: SimState = {
	messages: [
		{
			...MockChatMessage,
			id: 1,
			created_at: historyAt,
			content: [
				{ type: "text", text: "Spin up an agent to fix the relay backoff." },
			],
		},
		{
			...MockChatMessage,
			id: 2,
			role: "assistant",
			created_at: historyAt,
			content: [
				{
					type: "text",
					text: "I started a sub-agent on the relay backoff fix and logged it in the ledger. It retries with jittered exponential backoff capped at 30 seconds, and I will check its tests once it reports back.",
				},
			],
		},
	],
	stream: null,
	pending: new Set(),
	status: "waiting",
};

const stamp = (
	part: TypesGen.ChatMessagePart,
	at: string,
): TypesGen.ChatMessagePart => {
	switch (part.type) {
		case "reasoning":
		case "tool-call":
		case "tool-result":
			return { ...part, created_at: part.created_at ?? at };
		default:
			return part;
	}
};

const persistStream = (state: SimState, at: string): SimState => {
	if (!state.stream) {
		return state;
	}
	const content = state.stream.map((part) =>
		part.type === "reasoning" ? { ...part, completed_at: at } : part,
	);
	const calls = content.flatMap((part) =>
		part.type === "tool-call" && part.tool_call_id ? [part.tool_call_id] : [],
	);
	return {
		...state,
		stream: null,
		pending: new Set([...state.pending, ...calls]),
		messages: [
			...state.messages,
			{
				...MockChatMessage,
				id: state.messages.length + 1,
				role: "assistant",
				created_at: at,
				content,
			},
		],
	};
};

const applyEvent = (state: SimState, event: SimEvent, at: string): SimState => {
	switch (event.type) {
		case "start":
			return state;
		case "prompt":
			return {
				...state,
				status: "running",
				messages: [
					...state.messages,
					{
						...MockChatMessage,
						id: state.messages.length + 1,
						created_at: at,
						content: [
							{
								type: "text",
								text: "Please continue tracking the work so you can react and keep it going.",
							},
						],
					},
				],
			};
		case "stream":
			return {
				...state,
				stream: [
					...(state.stream ?? []),
					...event.parts.map((part) => stamp(part, at)),
				],
			};
		case "persist":
			return persistStream(state, at);
		case "result": {
			const pending = new Set(state.pending);
			pending.delete(event.id);
			return {
				...state,
				pending,
				messages: [
					...state.messages,
					{
						...MockChatMessage,
						id: state.messages.length + 1,
						role: "tool",
						created_at: at,
						content: [
							{
								type: "tool-result",
								tool_call_id: event.id,
								tool_name: event.name,
								result: event.result,
								created_at: at,
							},
						],
					},
				],
			};
		}
		case "finish":
			return { ...persistStream(state, at), status: "waiting" };
	}
};

/** The state after `times.length` events, each applied at its time. */
const simulate = (times: readonly number[]): SimState =>
	times.reduce(
		(state, time, index) =>
			applyEvent(state, script[index], new Date(time).toISOString()),
		initialState,
	);

type TimelinePaneProps = {
	state: SimState;
	peek: boolean;
	title?: string;
	height: number;
};

const TimelinePane: React.FC<TimelinePaneProps> = ({
	state,
	peek,
	title,
	height,
}) => {
	const parsedMessages = parseMessagesWithMergedTools(state.messages, {
		pendingToolCallIDs: new Set(state.pending),
	});
	const live = state.stream
		? buildStreamRenderState(state.stream)
		: {
				streamState: null,
				streamTools: [],
				liveStatus: {
					phase: state.status === "running" ? "starting" : "idle",
					hasAccumulatedOutput: false,
				} as const,
			};
	return (
		<div className="flex min-w-0 flex-1 flex-col gap-2">
			{title && (
				<h3 className="m-0 text-xs font-medium uppercase tracking-wide text-content-secondary">
					{title}
				</h3>
			)}
			<div
				className="flex flex-col overflow-hidden rounded-lg border border-solid border-border bg-surface-primary"
				style={{ height }}
			>
				<MessageScroller.Provider autoScroll defaultScrollPosition="end">
					<ChatMessageScroller
						hasMoreMessages={false}
						isFetchingMoreMessages={false}
						isHydratingMessages={false}
						hasFetchMoreError={false}
						hasTranscriptRows
						onFetchMoreMessages={async () => {}}
					>
						<ConversationTimeline
							hasMoreMessages={false}
							chatStatus={state.status}
							organizationId="organization-id"
							parsedMessages={parsedMessages}
							streamState={live.streamState}
							streamTools={live.streamTools}
							liveStatus={live.liveStatus}
							subagentStatusOverrides={new Map()}
							subagentTitles={subagentTitles}
							subagentVariants={new Map()}
							isChatCompleted={state.status !== "running"}
							urlTransform={defaultUrlTransform}
							mcpServers={[LinearMCPServer]}
							showDesktopPreviews={false}
							hasActiveStream={state.stream !== null}
							isAwaitingFirstStreamChunk={false}
							workingBlockPeek={peek}
						/>
					</ChatMessageScroller>
				</MessageScroller.Provider>
			</div>
		</div>
	);
};

const speeds = [0.5, 1, 2] as const;

type DemoArgs = {
	compare: boolean;
	/** Off renders today's collapsed block, for side-by-side measurements. */
	peek: boolean;
	/** A short pane makes the turn outgrow it, so the view follows the end. */
	paneHeight: number;
};

/** Replays the script in real time, looping, with transport controls. */
const LiveDemo: React.FC<DemoArgs> = ({ compare, peek, paneHeight }) => {
	const [run, setRun] = useState(0);
	const [times, setTimes] = useState<number[]>(() => [Date.now()]);
	const [playing, setPlaying] = useState(true);
	const [speed, setSpeed] = useState<(typeof speeds)[number]>(1);
	const index = times.length - 1;
	const done = index === script.length - 1;

	// The demo's clock is an external system: each tick applies one event.
	useEffect(() => {
		if (!playing) {
			return;
		}
		const timer = window.setTimeout(() => {
			if (done) {
				setRun((value) => value + 1);
				setTimes([Date.now()]);
			} else {
				setTimes((value) => [...value, Date.now()]);
			}
		}, script[index].ms / speed);
		return () => window.clearTimeout(timer);
	}, [playing, index, speed, done]);

	const state = simulate(times);
	const event = script[index];

	return (
		<div className="flex max-w-5xl flex-col gap-3 p-4">
			<div className="flex flex-wrap items-center gap-2">
				<Button
					size="sm"
					variant="outline"
					onClick={() => setPlaying((value) => !value)}
				>
					{playing ? <PauseIcon /> : <PlayIcon />}
					{playing ? "Pause" : "Play"}
				</Button>
				<Button
					size="sm"
					variant="outline"
					disabled={done}
					onClick={() => setTimes((value) => [...value, Date.now()])}
				>
					<StepForwardIcon />
					Step
				</Button>
				<Button
					size="sm"
					variant="outline"
					onClick={() => {
						setRun((value) => value + 1);
						setTimes([Date.now()]);
					}}
				>
					<RotateCcwIcon />
					Restart
				</Button>
				{speeds.map((value) => (
					<Button
						key={value}
						size="xs"
						variant={value === speed ? "default" : "subtle"}
						onClick={() => setSpeed(value)}
					>
						{value}x
					</Button>
				))}
				<span className="ml-auto text-xs tabular-nums text-content-secondary">
					{`Event ${index + 1} of ${script.length}: ${event.type}`}
				</span>
			</div>
			<div className="flex gap-4">
				{compare && (
					<TimelinePane
						key={`today-${run}`}
						state={state}
						peek={false}
						title="Today"
						height={paneHeight}
					/>
				)}
				<TimelinePane
					key={`peek-${run}`}
					state={state}
					peek={peek}
					title={compare ? "With peek" : undefined}
					height={paneHeight}
				/>
			</div>
		</div>
	);
};

/** Script offsets in ms, so a static frame can sit at a pinned clock. */
const offsets = [0];
for (let index = 1; index < script.length; index++) {
	offsets.push(offsets[index - 1] + script[index - 1].ms);
}

/** The state just after event `index`, frozen 1s later on the pinned clock. */
const frameAt = (index: number) => {
	const base = FIXTURE_NOW - offsets[index] - 1000;
	return simulate(offsets.slice(0, index + 1).map((offset) => base + offset));
};

const eventIndex = (predicate: (event: SimEvent, index: number) => boolean) =>
	script.findIndex(predicate);

/** The event that persists the step that called `id`. */
const persistOf = (id: string) =>
	eventIndex(
		(event) =>
			event.type === "stream" &&
			event.parts.some(
				(part) => part.type === "tool-call" && part.tool_call_id === id,
			),
	) + 1;

const meta: Meta<DemoArgs> = {
	title:
		"pages/AgentsPage/ChatConversation/ConversationTimeline/WorkingBlockPeek",
	parameters: {
		layout: "fullscreen",
		queries: [
			{ key: preferenceSettingsKey, data: MockCollapsedStepsPreferences },
		],
	},
	args: { compare: false, peek: true, paneHeight: 560 },
	render: (args) => <LiveDemo {...args} />,
};
export default meta;
type Story = StoryObj<DemoArgs>;

export const Live: Story = {};

export const LiveCompare: Story = { args: { compare: true } };

/** The turn outgrows the pane, so the scroller follows its end. */
export const LiveFollowingEnd: Story = { args: { paneHeight: 240 } };

const staticFrame = (index: number): Story => ({
	beforeEach: pinFixtureClock,
	render: () => (
		<div className="max-w-3xl p-4">
			<TimelinePane state={frameAt(index)} peek height={420} />
		</div>
	),
});

/** The first thought, before any tool. */
export const FirstStep = staticFrame(
	eventIndex((event) => event.type === "stream"),
);

/** A long sub-agent wait, with one earlier step folded away. */
export const WaitingOnSubagent = staticFrame(persistOf("wait"));

/** Two tools run at once, the case from the report. */
export const ParallelTools = staticFrame(persistOf("poll"));

/** The answer streams after the block while the peek still shows its steps. */
export const AnswerStreaming = staticFrame(script.length - 2);

/** The turn ended, so the block folds to its summary. */
export const Completed = staticFrame(script.length - 1);
