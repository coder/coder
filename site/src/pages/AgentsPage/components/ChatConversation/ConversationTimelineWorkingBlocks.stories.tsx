import { MessageScroller } from "@shadcn/react/message-scroller";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { useQueryClient } from "react-query";
import {
	expect,
	fireEvent,
	fn,
	userEvent,
	waitFor,
	within,
} from "storybook/test";
import { preferenceSettingsKey } from "#/api/queries/users";
import type { ChatMessage } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { MockChatMessage } from "#/testHelpers/chatEntities";
import { MockUserPreferenceSettings } from "#/testHelpers/entities";
import { ConversationTimeline } from "./ConversationTimeline";
import {
	getPendingToolCallIDs,
	parseMessagesWithMergedTools,
} from "./messageParsing";
import {
	buildLongConversation,
	buildStreamRenderState,
	buildWorkingConversation,
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
	title: "pages/AgentsPage/ChatConversation/ConversationTimeline/WorkingBlocks",
	component: ConversationTimeline,
	args: {
		organizationId: "organization-id",
		subagentTitles: new Map(),
		parsedMessages: parseMessagesWithMergedTools(MockWorkingMessages),
		now: start + 13000,
	},
	parameters: {
		queries: [
			{
				key: preferenceSettingsKey,
				data: {
					...MockUserPreferenceSettings,
					shell_tool_display_mode: "always_collapsed",
					collapse_assistant_steps: true,
				},
			},
		],
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

export const Completed: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const summary = canvas.getByRole("button", {
			name: "Worked for 12s (2 steps)",
		});
		expect(canvas.getByText("Workspace inspection complete.")).toBeVisible();
		expect(canvas.queryByText(/echo first/)).not.toBeInTheDocument();
		summary.focus();
		await userEvent.keyboard("{Enter}");
		expect(summary).toHaveAttribute("aria-expanded", "true");
		expect(canvas.getByText(/echo first/)).toBeVisible();
		expect(canvas.getByText(/echo second/)).toBeVisible();
		await userEvent.keyboard(" ");
		expect(summary).toHaveAttribute("aria-expanded", "false");
		expect(canvas.queryByText(/echo first/)).not.toBeInTheDocument();
	},
};

export const StreamingToDurable: Story = {
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
		const summary = canvas.getByRole("button", {
			name: "Working for 12s (echo first)",
		});
		await userEvent.click(summary);
		const commandRows = () =>
			canvas.getAllByRole("button", { name: /Expand command/ });
		expect(commandRows()).toHaveLength(1);
		await userEvent.click(
			canvas.getByRole("button", { name: "Advance stream" }),
		);
		expect(summary).toBeInTheDocument();
		expect(summary).toHaveAttribute("aria-expanded", "true");
		expect(summary).toHaveAccessibleName("Working for 12s (echo second)");
		expect(commandRows()).toHaveLength(2);
		await userEvent.click(
			canvas.getByRole("button", { name: "Advance stream" }),
		);
		expect(
			canvas.getByRole("button", { name: "Worked for 12s (2 steps)" }),
		).toHaveAttribute("aria-expanded", "true");
		expect(commandRows()).toHaveLength(2);
		expect(canvas.getByText("Workspace inspection complete.")).toBeVisible();
	},
};

// Between persisted steps the stream is cleared while a tool runs, so the
// timeline only knows the turn is active from the chat status.
export const RunningBetweenSteps: Story = {
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
		const summary = canvas.getByRole("button", {
			name: "Working for 12s (echo second)",
		});
		await userEvent.click(summary);
		expect(summary).toHaveAttribute("aria-expanded", "true");
		expect(
			within(canvas.getByTestId("chat-message-message:4")).getByText(
				/echo second/,
			),
		).toBeVisible();
		await userEvent.click(canvas.getByRole("button", { name: "Finish turn" }));
		expect(
			canvas.getByRole("button", { name: "Worked for 4s (2 steps)" }),
		).toHaveAttribute("aria-expanded", "true");
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
		const summary = canvas.getByRole("button", {
			name: "Working for 12s (echo second)",
		});
		const advance = canvas.getByRole("button", { name: "Advance stream" });
		expect(canvas.queryByTestId("live-activity-slot")).toBeNull();
		await userEvent.click(advance);
		expect(canvas.queryByTestId("live-activity-slot")).toBeNull();
		await userEvent.click(advance);
		expect(canvas.queryByText(/list the files before/i)).toBeNull();
		expect(summary).toHaveAccessibleName(
			"Working for 12s (Planning the inspection)",
		);
		await userEvent.click(summary);
		// The reasoning text streams in through the smoothing buffer.
		expect(await canvas.findByText(/planning the inspection/i)).toBeVisible();
	},
};

// A turn's first reasoning already folds, so thinking never shows and then
// vanishes into the block once its first tool call arrives.
export const ReasoningBeforeFirstToolFolds: Story = {
	args: {
		parsedMessages: parseMessagesWithMergedTools(
			MockWorkingMessages.slice(0, 1),
		),
		chatStatus: "running",
		...MockReasoningStream,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const summary = canvas.getByRole("button", {
			name: "Working for 12s (Planning the inspection)",
		});
		expect(canvas.queryByText(/list the files before/i)).toBeNull();
		expect(canvas.queryByTestId("live-activity-slot")).toBeNull();
		await userEvent.click(summary);
		// The reasoning text streams in through the smoothing buffer.
		expect(await canvas.findByText(/planning the inspection/i)).toBeVisible();
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
	args: {
		chatStatus: "requires_action",
		onSendAskUserQuestionResponse: fn(),
		parsedMessages: parseMessagesWithMergedTools(
			[...MockWorkingMessages.slice(0, 3), MockPendingQuestionMessage],
			{ pendingToolCallIDs: new Set(["question"]) },
		),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("button", { name: "Worked for 3s (1 step)" }),
		).toBeVisible();
		expect(canvas.queryByRole("button", { name: /Working/ })).toBeNull();
		// The question row is its own item, never inside the fold.
		const questionRow = canvas.getByTestId("chat-message-message:4");
		expect(questionRow).toBeVisible();
		expect(
			within(canvas.getByTestId("working-block")).queryByTestId(
				"chat-message-message:4",
			),
		).toBeNull();
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
	args: {
		chatStatus: "requires_action",
		parsedMessages: parseMessagesWithMergedTools(
			[...MockWorkingMessages.slice(0, 3), MockParkedToolMessage],
			{ pendingToolCallIDs: new Set(["editor"]) },
		),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("button", { name: "Worked for 3s (1 step)" }),
		).toBeVisible();
		expect(canvas.queryByRole("button", { name: /Completed/ })).toBeNull();
		const parkedRow = canvas.getByTestId("chat-message-message:4");
		expect(parkedRow).toBeVisible();
		expect(within(parkedRow).getByText("open_editor")).toBeVisible();
		expect(
			within(canvas.getByTestId("working-block")).queryByTestId(
				"chat-message-message:4",
			),
		).toBeNull();
	},
};

// An active turn longer than the loaded page has no prompt row; expansion
// must still survive completion and the later prepend of that prompt.
export const PromptlessLiveBlockKeepsExpansion: Story = {
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
		const live = canvas.getByRole("button", {
			name: "Working for at least 12s (echo second)",
		});
		await userEvent.click(live);
		expect(live).toHaveAttribute("aria-expanded", "true");
		await userEvent.click(canvas.getByRole("button", { name: "Finish turn" }));
		expect(
			canvas.getByRole("button", {
				name: "Worked for at least 12s (2 steps or more)",
			}),
		).toHaveAttribute("aria-expanded", "true");
		await userEvent.click(
			canvas.getByRole("button", { name: "Load older messages" }),
		);
		expect(
			canvas.getByRole("button", { name: "Worked for 12s (2 steps)" }),
		).toHaveAttribute("aria-expanded", "true");
		expect(canvas.getByText(/echo first/)).toBeVisible();
		expect(canvas.getByText("Inspect the workspace")).toBeVisible();
	},
};

export const Paginated: Story = {
	render: function Render(args) {
		const [page, setPage] = useState(0);
		return (
			<>
				<Button onClick={() => setPage(page + 1)} disabled={page === 2}>
					Load older messages
				</Button>
				<ConversationTimeline
					{...args}
					hasMoreMessages={page < 2}
					parsedMessages={parseMessagesWithMergedTools(
						MockWorkingMessages.slice(page === 0 ? 3 : page === 1 ? 1 : 0),
					)}
				/>
			</>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Only the newest page is loaded: the block is missing its opening rows,
		// so the label is a lower bound rather than a claim of completeness.
		const partial = canvas.getByRole("button", {
			name: "Worked for at least 8s (1 step or more)",
		});
		expect(canvas.queryByText(/echo second/)).not.toBeInTheDocument();
		await userEvent.click(partial);
		expect(canvas.getByText(/echo second/)).toBeVisible();
		await userEvent.click(
			canvas.getByRole("button", { name: "Load older messages" }),
		);
		// The prepended step joins the same block; identity and expansion hold.
		const grown = canvas.getByRole("button", {
			name: "Worked for at least 12s (2 steps or more)",
		});
		expect(grown).toBe(partial);
		expect(grown).toHaveAttribute("aria-expanded", "true");
		expect(canvas.getByText(/echo first/)).toBeVisible();
		await userEvent.click(
			canvas.getByRole("button", { name: "Load older messages" }),
		);
		const complete = canvas.getByRole("button", {
			name: "Worked for 12s (2 steps)",
		});
		expect(complete).toBe(partial);
		expect(complete).toHaveAttribute("aria-expanded", "true");
		expect(canvas.getAllByText(/echo first/)).toHaveLength(1);
		expect(canvas.getAllByText(/echo second/)).toHaveLength(1);
		expect(canvas.getAllByText("Workspace inspection complete.")).toHaveLength(
			1,
		);
	},
};

const longTurnStep = (index: number): ChatMessage[] => [
	{
		...MockChatMessage,
		id: 100 + index * 2,
		role: "assistant",
		created_at: time(index),
		content: [
			{
				type: "tool-call",
				tool_call_id: `step-${index}`,
				tool_name: "execute",
				args: { command: `echo step-${index}` },
				created_at: time(index),
			},
		],
	},
	{
		...MockChatMessage,
		id: 101 + index * 2,
		role: "tool",
		created_at: time(index),
		content: [
			{
				type: "tool-result",
				tool_call_id: `step-${index}`,
				tool_name: "execute",
				result: { output: `step-${index}`, exit_code: "0" },
				created_at: time(index),
			},
		],
	},
];
const MockLongTurn = Array.from({ length: 60 }, (_, index) =>
	longTurnStep(index),
).flat();
const MockLongTurnPrompt: ChatMessage = {
	...MockChatMessage,
	id: 99,
	created_at: time(-1),
	content: [{ type: "text", text: "Run every step" }],
};
const longTurnPages = [
	MockLongTurn.slice(60),
	MockLongTurn.slice(30),
	[MockLongTurnPrompt, ...MockLongTurn],
];

// Older rows join an expanded partial block inside one scroller item, so the
// scroller cannot anchor them; the block keeps the reading position itself.
// The last page also prepends the prompt row as a new scroller item, which the
// scroller anchors on its own; both corrections have to add up.
export const PrependIntoExpandedBlockKeepsReadingPosition: Story = {
	render: function Render(args) {
		const [page, setPage] = useState(0);
		// The button follows the rows so the scroller sees the previous first
		// item move to a later index, as in the real page, and stays fixed so
		// clicking it never scrolls the viewport away from the top.
		return (
			<>
				<ConversationTimeline
					{...args}
					hasMoreMessages={page < 2}
					parsedMessages={parseMessagesWithMergedTools(longTurnPages[page])}
				/>
				<Button
					className="fixed top-2 right-2 z-20"
					onClick={() => setPage(page + 1)}
					disabled={page === 2}
				>
					Load older messages
				</Button>
			</>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: /Worked for at least/ }),
		);
		const viewport = canvas.getByRole("region", { name: "Messages" });
		// The scroller stops following the bottom only on wheel, touch, or key
		// input, so scroll up the way a reader does.
		await fireEvent.wheel(viewport, { deltaY: -100 });
		viewport.scrollTop = 0;
		await waitFor(() => expect(viewport.scrollTop).toBe(0));
		const anchor = canvas.getByText(/echo step-30/);
		const anchorTop = anchor.getBoundingClientRect().top;
		await userEvent.click(
			canvas.getByRole("button", { name: "Load older messages" }),
		);
		expect(canvas.getByText(/echo step-15$/)).toBeInTheDocument();
		expect(anchor.getBoundingClientRect().top).toBeCloseTo(anchorTop, 0);
		expect(viewport.scrollTop).toBeGreaterThan(0);
		// History only pages while the reader sits at the very top, where the
		// browser suspends its own scroll anchoring.
		await fireEvent.wheel(viewport, { deltaY: -100 });
		viewport.scrollTop = 0;
		await waitFor(() => expect(viewport.scrollTop).toBe(0));
		const topAnchor = canvas.getByText(/echo step-15$/);
		const topAnchorTop = topAnchor.getBoundingClientRect().top;
		await userEvent.click(
			canvas.getByRole("button", { name: "Load older messages" }),
		);
		expect(canvas.getByText("Run every step")).toBeInTheDocument();
		expect(canvas.getByText(/echo step-0$/)).toBeInTheDocument();
		expect(canvas.getByText(/echo step-15$/)).toBe(topAnchor);
		expect(topAnchor.getBoundingClientRect().top).toBeCloseTo(topAnchorTop, 0);
		expect(viewport.scrollTop).toBeGreaterThan(0);
	},
};

// The scroller follows the bottom while the viewport is pinned there and
// re-pins on every resize. Expanding a block from that position must keep
// its header where it is instead of scrolling the opened rows past.
export const ExpandAtBottomKeepsHeaderInPlace: Story = {
	args: {
		parsedMessages: parseMessagesWithMergedTools([
			...buildLongConversation(MockChatMessage.chat_id, 20),
			...MockLongTurn,
		]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const viewport = canvas.getByRole("region", { name: "Messages" });
		const summary = canvas.getByRole("button", { name: /^Worked for/ });
		await waitFor(() =>
			expect(
				viewport.scrollHeight - viewport.scrollTop - viewport.clientHeight,
			).toBeLessThanOrEqual(1),
		);
		const summaryTop = summary.getBoundingClientRect().top;
		const scrollTop = viewport.scrollTop;
		await userEvent.click(summary);
		expect(canvas.getByText(/echo step-0$/)).toBeInTheDocument();
		await new Promise((resolve) => requestAnimationFrame(resolve));
		await new Promise((resolve) => requestAnimationFrame(resolve));
		expect(summary.getBoundingClientRect().top).toBeCloseTo(summaryTop, 0);
		expect(viewport.scrollTop).toBe(scrollTop);
	},
};

export const PreferenceChanges: Story = {
	render: function Render(args) {
		const client = useQueryClient();
		return (
			<>
				<Button
					onClick={() =>
						client.setQueryData(preferenceSettingsKey, {
							...MockUserPreferenceSettings,
							shell_tool_display_mode: "always_collapsed",
							collapse_assistant_steps: false,
						})
					}
				>
					Show individual steps
				</Button>
				<Button
					onClick={() =>
						client.setQueryData(preferenceSettingsKey, {
							...MockUserPreferenceSettings,
							shell_tool_display_mode: "always_collapsed",
							collapse_assistant_steps: true,
						})
					}
				>
					Restore grouping
				</Button>
				<ConversationTimeline {...args} />
			</>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Worked for 12s (2 steps)" }),
		);
		await userEvent.click(
			canvas.getByRole("button", { name: "Show individual steps" }),
		);
		expect(
			canvas.queryByRole("button", { name: /Worked/ }),
		).not.toBeInTheDocument();
		expect(canvas.getByText(/echo first/)).toBeVisible();
		await userEvent.click(
			canvas.getByRole("button", { name: "Restore grouping" }),
		);
		expect(
			canvas.getByRole("button", { name: "Worked for 12s (2 steps)" }),
		).toHaveAttribute("aria-expanded", "true");
	},
};

export const FailedStepCounted: Story = {
	args: {
		parsedMessages: parseMessagesWithMergedTools(
			MockWorkingMessages.map((message) =>
				message.id === 5
					? {
							...message,
							content: [
								{
									type: "tool-result",
									tool_call_id: "second",
									tool_name: "execute",
									is_error: true,
									result: { output: "Command failed", exit_code: "1" },
									created_at: time(13),
								},
							],
						}
					: message,
			),
		),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const summary = canvas.getByRole("button", {
			name: "Worked for 12s (2 steps) 1 failed step",
		});
		expect(summary).toHaveAttribute("aria-expanded", "false");
		expect(canvas.getByText("Workspace inspection complete.")).toBeVisible();
		await userEvent.click(summary);
		const failedStep = canvas.getByTestId("chat-message-message:4");
		await userEvent.click(
			within(failedStep).getByRole("button", { name: /Expand command/ }),
		);
		expect(within(failedStep).getByText("Command failed")).toBeVisible();
	},
};

export const QuestionStaysVisible: Story = {
	args: {
		isChatCompleted: true,
		onSendAskUserQuestionResponse: fn(),
		parsedMessages: parseMessagesWithMergedTools([
			...MockWorkingMessages.slice(0, 3),
			{
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
					},
				],
			},
			{
				...MockChatMessage,
				id: 5,
				role: "tool",
				created_at: time(5),
				content: [
					{
						type: "tool-result",
						tool_call_id: "question",
						tool_name: "ask_user_question",
						result: {
							output: JSON.stringify({
								questions: [
									{
										header: "Deploy",
										question: "Deploy the workspace?",
										options: [
											{ label: "Yes", description: "Deploy now" },
											{ label: "No", description: "Keep changes local" },
										],
									},
								],
							}),
						},
					},
				],
			},
		]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("button", { name: "Worked for 3s (1 step)" }),
		).toHaveAttribute("aria-expanded", "false");
		expect(canvas.getByText("Deploy the workspace?")).toBeVisible();
		await userEvent.click(canvas.getByRole("radio", { name: /Yes/ }));
		expect(canvas.getByText("Deploy the workspace?")).toBeVisible();
	},
};

// Narration that precedes a tool call folds with it; inside the expanded
// block it is bracketed off the rule, while the final answer outside is not.
export const NarrationBracketedInsideBlock: Story = {
	args: {
		parsedMessages: parseMessagesWithMergedTools(
			MockWorkingMessages.map((message) =>
				message.id === 4
					? {
							...message,
							content: [
								{ type: "text", text: "Let me try the second command." },
								...(message.content ?? []),
							],
						}
					: message,
			),
		),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Worked for 12s (2 steps)" }),
		);
		const bracket = canvas.getByTestId("step-narration-bracket");
		expect(
			within(bracket).getByText("Let me try the second command."),
		).toBeVisible();
		expect(canvas.getByText("Workspace inspection complete.")).toBeVisible();
		expect(canvas.getAllByTestId("step-narration-bracket")).toHaveLength(1);
	},
};

// The narration that opens a block's first step is the turn's lead-in, so it
// stays above the fold; the tool call it introduces still folds.
export const OpeningNarrationStaysAboveBlock: Story = {
	args: {
		parsedMessages: parseMessagesWithMergedTools(
			MockWorkingMessages.map((message) =>
				message.id === 2
					? {
							...message,
							content: [
								{ type: "text", text: "I'll inspect the workspace first." },
								...(message.content ?? []),
							],
						}
					: message,
			),
		),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const summary = canvas.getByRole("button", {
			name: "Worked for 12s (2 steps)",
		});
		const opening = canvas.getByText("I'll inspect the workspace first.");
		expect(opening).toBeVisible();
		expect(
			within(canvas.getByTestId("working-block")).queryByText(
				"I'll inspect the workspace first.",
			),
		).toBeNull();
		expect(
			opening.compareDocumentPosition(summary) &
				Node.DOCUMENT_POSITION_FOLLOWING,
		).toBeTruthy();
		await userEvent.click(summary);
		expect(
			canvas.getAllByText("I'll inspect the workspace first."),
		).toHaveLength(1);
		expect(
			canvas.getAllByRole("button", { name: /Expand command/ }),
		).toHaveLength(2);
		expect(canvas.queryByTestId("step-narration-bracket")).toBeNull();
	},
};

// The lead-in must not fold away when its first tool call streams in, nor
// move when the step persists.
export const OpeningNarrationHoldsThroughStreaming: Story = {
	render: function Render(args) {
		const [stage, setStage] = useState(0);
		const leadIn = "I'll inspect the workspace first.";
		const stream: StreamState = {
			startedAt: time(1),
			blocks:
				stage === 0
					? [{ type: "response", text: leadIn }]
					: [
							{ type: "response", text: leadIn },
							{ type: "tool", id: "first" },
						],
			toolCalls:
				stage === 0
					? {}
					: {
							first: {
								id: "first",
								name: "execute",
								args: { command: "echo first" },
								createdAt: time(1),
							},
						},
			toolResults: {},
			sources: [],
		};
		const persisted = MockWorkingMessages.slice(0, 3).map((message) =>
			message.id === 2
				? {
						...message,
						content: [
							{ type: "text" as const, text: leadIn },
							...(message.content ?? []),
						],
					}
				: message,
		);
		return (
			<>
				<Button onClick={() => setStage(stage + 1)} disabled={stage === 2}>
					Advance stream
				</Button>
				<ConversationTimeline
					{...args}
					parsedMessages={parseMessagesWithMergedTools(
						stage < 2 ? MockWorkingMessages.slice(0, 1) : persisted,
					)}
					streamState={stage < 2 ? stream : null}
					streamTools={
						stage < 2
							? buildStreamTools(stream.toolCalls, stream.toolResults)
							: []
					}
					chatStatus="running"
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
		const leadIn = await canvas.findByText("I'll inspect the workspace first.");
		expect(canvas.queryByRole("button", { name: /^Working/ })).toBeNull();
		const leadInTop = leadIn.getBoundingClientRect().top;
		await userEvent.click(
			canvas.getByRole("button", { name: "Advance stream" }),
		);
		const working = canvas.getByRole("button", {
			name: "Working for 12s (echo first)",
		});
		expect(canvas.getByText("I'll inspect the workspace first.")).toBeVisible();
		expect(
			canvas
				.getByText("I'll inspect the workspace first.")
				.getBoundingClientRect().top,
		).toBeCloseTo(leadInTop, 0);
		const workingTop = working.getBoundingClientRect().top;
		expect(workingTop).toBeGreaterThan(leadInTop);
		await userEvent.click(
			canvas.getByRole("button", { name: "Advance stream" }),
		);
		expect(canvas.getByText("I'll inspect the workspace first.")).toBeVisible();
		// Streamdown lays out streaming and static markdown a few pixels apart.
		expect(
			Math.abs(
				canvas.getByRole("button", { name: /^Working/ }).getBoundingClientRect()
					.top - workingTop,
			),
		).toBeLessThan(4);
	},
};

export const Mobile: Story = {
	...Completed,
	globals: { viewport: { value: "mobile1", isRotated: false } },
};

export const EditingPrecedingMessage: Story = {
	render: function Render(args) {
		const [editing, setEditing] = useState(false);
		return (
			<>
				<Button onClick={() => setEditing(!editing)}>
					{editing ? "Finish editing" : "Edit prompt"}
				</Button>
				<ConversationTimeline {...args} editingMessageId={editing ? 1 : null} />
			</>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const summary = canvas.getByRole("button", {
			name: "Worked for 12s (2 steps)",
		});
		await userEvent.click(canvas.getByRole("button", { name: "Edit prompt" }));
		summary.focus();
		expect(summary).not.toHaveFocus();
		expect(summary).toHaveAttribute("aria-expanded", "false");
		await userEvent.click(
			canvas.getByRole("button", { name: "Finish editing" }),
		);
		summary.focus();
		expect(summary).toHaveFocus();
		await userEvent.keyboard("{Enter}");
		expect(canvas.getByText(/echo first/)).toBeVisible();
	},
};
