import { MessageScroller } from "@shadcn/react/message-scroller";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { useQueryClient } from "react-query";
import { expect, fn, spyOn, userEvent, waitFor, within } from "storybook/test";
import { API } from "#/api/api";
import { preferenceSettingsKey } from "#/api/queries/users";
import type { ChatMessage } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { MockChatMessage } from "#/testHelpers/chatEntities";
import { MockUserPreferenceSettings } from "#/testHelpers/entities";
import { ConversationTimeline } from "./ConversationTimeline";
import { parseMessagesWithMergedTools } from "./messageParsing";
import { buildStreamTools } from "./streamState";
import type { StreamState } from "./types";

const start = Date.parse("2026-04-01T12:00:00Z");
const time = (seconds: number) =>
	new Date(start + seconds * 1000).toISOString();
const MockWorkingMessages: ChatMessage[] = [
	{
		...MockChatMessage,
		id: 1,
		created_at: time(0),
		content: [{ type: "text", text: "Inspect the workspace" }],
	},
	{
		...MockChatMessage,
		id: 2,
		role: "assistant",
		created_at: time(1),
		content: [
			{
				type: "tool-call",
				tool_call_id: "first",
				tool_name: "execute",
				args: { command: "echo first" },
				created_at: time(1),
			},
		],
	},
	{
		...MockChatMessage,
		id: 3,
		role: "tool",
		created_at: time(4),
		content: [
			{
				type: "tool-result",
				tool_call_id: "first",
				tool_name: "execute",
				result: { output: "First output", exit_code: "0" },
				created_at: time(4),
			},
		],
	},
	{
		...MockChatMessage,
		id: 4,
		role: "assistant",
		created_at: time(5),
		content: [
			{
				type: "tool-call",
				tool_call_id: "second",
				tool_name: "execute",
				args: { command: "echo second" },
				created_at: time(5),
			},
		],
	},
	{
		...MockChatMessage,
		id: 5,
		role: "tool",
		created_at: time(13),
		content: [
			{
				type: "tool-result",
				tool_call_id: "second",
				tool_name: "execute",
				result: { output: "Second output", exit_code: "0" },
				created_at: time(13),
			},
		],
	},
	{
		...MockChatMessage,
		id: 6,
		role: "assistant",
		created_at: time(14),
		content: [{ type: "text", text: "Workspace inspection complete." }],
	},
];

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
		const summary = canvas.getByRole("button", { name: "Working for 12s" });
		await userEvent.click(summary);
		expect(canvas.getByText(/echo first/)).toBeVisible();
		await userEvent.click(
			canvas.getByRole("button", { name: "Advance stream" }),
		);
		expect(summary).toBeInTheDocument();
		expect(summary).toHaveAttribute("aria-expanded", "true");
		expect(canvas.getByText(/echo second/)).toBeVisible();
		await userEvent.click(
			canvas.getByRole("button", { name: "Advance stream" }),
		);
		expect(
			canvas.getByRole("button", { name: "Worked for 12s (2 steps)" }),
		).toHaveAttribute("aria-expanded", "true");
		expect(canvas.getByText(/echo first/)).toBeVisible();
		expect(canvas.getByText("Workspace inspection complete.")).toBeVisible();
	},
};

// Between persisted steps the stream is cleared while a tool runs, so the
// timeline only knows the turn is active from the chat status.
export const RunningBetweenSteps: Story = {
	render: function Render(args) {
		const [status, setStatus] = useState<"running" | "waiting">("running");
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
					parsedMessages={parseMessagesWithMergedTools(
						MockWorkingMessages.slice(0, 4),
						{ pendingToolCallIDs: new Set(["second"]) },
					)}
					chatStatus={status}
					liveStatus={{ phase: "idle", hasAccumulatedOutput: false }}
				/>
			</>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const summary = canvas.getByRole("button", { name: "Working for 12s" });
		await userEvent.click(summary);
		expect(summary).toHaveAttribute("aria-expanded", "true");
		expect(canvas.getByText(/echo second/)).toBeVisible();
		await userEvent.click(canvas.getByRole("button", { name: "Finish turn" }));
		expect(
			canvas.getByRole("button", { name: "Worked for 4s (2 steps)" }),
		).toHaveAttribute("aria-expanded", "true");
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

// On a cold load the saved preference decides the first paint: rows never
// render ungrouped and then fold.
export const ColdLoadUsesSavedPreference: Story = {
	parameters: { queries: [] },
	beforeEach: () => {
		spyOn(API, "getUserPreferenceSettings").mockImplementation(
			() =>
				new Promise((resolve) => {
					setTimeout(
						() =>
							resolve({
								...MockUserPreferenceSettings,
								shell_tool_display_mode: "always_collapsed",
								collapse_assistant_steps: true,
							}),
						300,
					);
				}),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.queryByTestId("chat-message-message:2")).toBeNull();
		expect(canvas.queryByText("Inspect the workspace")).toBeNull();
		await waitFor(
			() => {
				expect(
					canvas.getByRole("button", { name: "Worked for 12s (2 steps)" }),
				).toBeVisible();
			},
			{ timeout: 3000 },
		);
		expect(canvas.queryByTestId("chat-message-message:2")).toBeNull();
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
