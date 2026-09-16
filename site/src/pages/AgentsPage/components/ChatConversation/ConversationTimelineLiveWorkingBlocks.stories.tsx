import { MessageScroller } from "@shadcn/react/message-scroller";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { fn, userEvent, within } from "storybook/test";
import { preferenceSettingsKey } from "#/api/queries/users";
import type { ChatMessage } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { MockChatMessage } from "#/testHelpers/chatEntities";
import { ConversationTimeline } from "./ConversationTimeline";
import {
	getPendingToolCallIDs,
	parseMessagesWithMergedTools,
} from "./messageParsing";
import {
	buildStreamRenderState,
	buildWorkingConversation,
	MockCollapsedStepsPreferences,
	type StoryStreamRenderState,
	WORKING_FIXTURE_START,
	workingFixtureTime,
} from "./storyFixtures";
import { buildStreamTools, createEmptyStreamState } from "./streamState";
import type { StreamState } from "./types";

const start = WORKING_FIXTURE_START;
const time = workingFixtureTime;
const MockWorkingMessages = buildWorkingConversation();

const meta: Meta<typeof ConversationTimeline> = {
	title:
		"pages/AgentsPage/ChatConversation/ConversationTimeline/LiveWorkingBlocks",
	component: ConversationTimeline,
	args: {
		organizationId: "organization-id",
		subagentTitles: new Map(),
		parsedMessages: parseMessagesWithMergedTools(MockWorkingMessages),
		now: start + 13000,
	},
	decorators: [
		(Story) => (
			<MessageScroller.Provider autoScroll defaultScrollPosition="end">
				<MessageScroller.Root>
					<MessageScroller.Viewport className="max-h-[600px] overflow-auto">
						<MessageScroller.Content>
							<Story />
						</MessageScroller.Content>
					</MessageScroller.Viewport>
				</MessageScroller.Root>
			</MessageScroller.Provider>
		),
	],
};
export default meta;
type Story = StoryObj<typeof ConversationTimeline>;

export const StreamingToDurable: Story = {
	parameters: {
		queries: [
			{ key: preferenceSettingsKey, data: MockCollapsedStepsPreferences },
		],
	},
	render: function Render(args) {
		const [stage, setStage] = useState(0);
		const stream: StreamState = {
			startedAt: stage === 0 ? time(1) : time(5),
			blocks: [{ type: "tool", id: stage === 0 ? "first" : "second" }],
			toolCalls:
				stage === 0
					? {
							first: {
								id: "first",
								name: "execute",
								args: { command: "echo first" },
								createdAt: time(1),
							},
						}
					: {
							second: {
								id: "second",
								name: "execute",
								args: { command: "echo second" },
								createdAt: time(5),
							},
						},
			toolResults: {},
			sources: [],
		};
		return (
			<>
				<Button onClick={() => setStage(stage + 1)} disabled={stage === 2}>
					Advance stream
				</Button>
				<ConversationTimeline
					{...args}
					parsedMessages={parseMessagesWithMergedTools(
						stage === 0
							? MockWorkingMessages.slice(0, 1)
							: stage === 1
								? MockWorkingMessages.slice(0, 3)
								: MockWorkingMessages,
					)}
					streamState={stage < 2 ? stream : null}
					streamTools={
						stage < 2
							? buildStreamTools(stream.toolCalls, stream.toolResults)
							: []
					}
					chatStatus={stage < 2 ? "running" : "waiting"}
					liveStatus={{
						phase: stage < 2 ? "streaming" : "idle",
						hasAccumulatedOutput: stage < 2,
					}}
				/>
			</>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Working for 12s" }),
		);
		const advance = canvas.getByRole("button", { name: "Advance stream" });
		await userEvent.click(advance);
		await userEvent.click(advance);
	},
};

// Between persisted steps the stream is cleared while a tool runs, so the
// timeline only knows the turn is active from the chat status.
export const RunningBetweenSteps: Story = {
	parameters: {
		queries: [
			{ key: preferenceSettingsKey, data: MockCollapsedStepsPreferences },
		],
	},
	render: function Render(args) {
		const [status, setStatus] = useState<"running" | "waiting">("running");
		const messages = MockWorkingMessages.slice(0, 4);
		return (
			<>
				<Button
					onClick={() => setStatus("waiting")}
					disabled={status === "waiting"}
				>
					Finish turn
				</Button>
				<ConversationTimeline
					{...args}
					parsedMessages={parseMessagesWithMergedTools(messages, {
						pendingToolCallIDs: getPendingToolCallIDs(messages, status),
					})}
					chatStatus={status}
					liveStatus={{ phase: "idle", hasAccumulatedOutput: false }}
				/>
			</>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Working for 12s" }),
		);
		await userEvent.click(canvas.getByRole("button", { name: "Finish turn" }));
	},
};

const MockReasoningStream = buildStreamRenderState([
	{
		type: "reasoning",
		text: "Planning the inspection\n\nList the files before reading any.",
		created_at: time(1),
	},
]);

// After a tool result the stream reopens empty, then reasons, before the next
// call streams. Each of those moments is the same block still working, so
// nothing appears under its summary.
const NextStepStages: readonly StoryStreamRenderState[] = [
	{
		streamState: null,
		streamTools: [],
		liveStatus: { phase: "starting", hasAccumulatedOutput: false },
	},
	{
		streamState: createEmptyStreamState(),
		streamTools: [],
		liveStatus: { phase: "streaming", hasAccumulatedOutput: false },
	},
	MockReasoningStream,
];

export const NextStepStartsInsideBlock: Story = {
	parameters: {
		queries: [
			{ key: preferenceSettingsKey, data: MockCollapsedStepsPreferences },
		],
	},
	render: function Render(args) {
		const [stage, setStage] = useState(0);
		const messages = MockWorkingMessages.slice(0, 5);
		return (
			<>
				<Button
					onClick={() => setStage(stage + 1)}
					disabled={stage === NextStepStages.length - 1}
				>
					Advance stream
				</Button>
				<ConversationTimeline
					{...args}
					parsedMessages={parseMessagesWithMergedTools(messages)}
					chatStatus="running"
					{...NextStepStages[stage]}
				/>
			</>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const advance = canvas.getByRole("button", { name: "Advance stream" });
		await userEvent.click(advance);
		await userEvent.click(advance);
		await userEvent.click(
			canvas.getByRole("button", { name: "Working for 12s" }),
		);
		// The reasoning text streams in through the smoothing buffer.
		await canvas.findByText(/planning the inspection/i);
	},
};

// A turn's first reasoning already folds, so thinking never shows and then
// vanishes into the block once its first tool call arrives.
export const ReasoningBeforeFirstToolFolds: Story = {
	parameters: {
		queries: [
			{ key: preferenceSettingsKey, data: MockCollapsedStepsPreferences },
		],
	},
	args: {
		parsedMessages: parseMessagesWithMergedTools(
			MockWorkingMessages.slice(0, 1),
		),
		chatStatus: "running",
		...MockReasoningStream,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Working for 12s" }),
		);
		// The reasoning text streams in through the smoothing buffer.
		await canvas.findByText(/planning the inspection/i);
	},
};

const MockPendingQuestionMessage: ChatMessage = {
	...MockChatMessage,
	id: 4,
	role: "assistant",
	created_at: time(5),
	content: [
		{
			type: "tool-call",
			tool_call_id: "question",
			tool_name: "ask_user_question",
			args: {},
			created_at: time(5),
		},
	],
};

// A pending ask_user_question parks the chat in requires_action: the agent is
// waiting on the user, so the preceding steps read as finished work.
export const RequiresActionCompletesBlock: Story = {
	parameters: {
		queries: [
			{ key: preferenceSettingsKey, data: MockCollapsedStepsPreferences },
		],
	},
	args: {
		chatStatus: "requires_action",
		onSendAskUserQuestionResponse: fn(),
		parsedMessages: parseMessagesWithMergedTools(
			[...MockWorkingMessages.slice(0, 3), MockPendingQuestionMessage],
			{ pendingToolCallIDs: new Set(["question"]) },
		),
	},
};

const MockParkedToolMessage: ChatMessage = {
	...MockChatMessage,
	id: 4,
	role: "assistant",
	created_at: time(5),
	content: [
		{
			type: "tool-call",
			tool_call_id: "editor",
			tool_name: "open_editor",
			args: { path: "README.md" },
			created_at: time(5),
		},
	],
};

// A client-executed tool parks the chat in requires_action with its call
// still running. It waits on that client, so it stays outside the fold.
export const RequiresActionKeepsPendingToolVisible: Story = {
	parameters: {
		queries: [
			{ key: preferenceSettingsKey, data: MockCollapsedStepsPreferences },
		],
	},
	args: {
		chatStatus: "requires_action",
		parsedMessages: parseMessagesWithMergedTools(
			[...MockWorkingMessages.slice(0, 3), MockParkedToolMessage],
			{ pendingToolCallIDs: new Set(["editor"]) },
		),
	},
};

// An active turn longer than the loaded page has no prompt row; expansion
// must still survive completion and the later prepend of that prompt.
export const PromptlessLiveBlockKeepsExpansion: Story = {
	parameters: {
		queries: [
			{ key: preferenceSettingsKey, data: MockCollapsedStepsPreferences },
		],
	},
	render: function Render(args) {
		const [stage, setStage] = useState(0);
		const messages =
			stage === 0
				? MockWorkingMessages.slice(1, 4)
				: stage === 1
					? MockWorkingMessages.slice(1)
					: MockWorkingMessages;
		return (
			<>
				<Button onClick={() => setStage(stage + 1)} disabled={stage === 2}>
					{stage === 0 ? "Finish turn" : "Load older messages"}
				</Button>
				<ConversationTimeline
					{...args}
					hasMoreMessages={stage < 2}
					chatStatus={stage === 0 ? "running" : "waiting"}
					parsedMessages={parseMessagesWithMergedTools(
						messages,
						stage === 0 ? { pendingToolCallIDs: new Set(["second"]) } : {},
					)}
					liveStatus={{ phase: "idle", hasAccumulatedOutput: false }}
				/>
			</>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Working for at least 12s" }),
		);
		await userEvent.click(canvas.getByRole("button", { name: "Finish turn" }));
		await userEvent.click(
			canvas.getByRole("button", { name: "Load older messages" }),
		);
	},
};
