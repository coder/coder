import { MessageScroller } from "@shadcn/react/message-scroller";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { useQueryClient } from "react-query";
import { fireEvent, fn, userEvent, waitFor, within } from "storybook/test";
import { preferenceSettingsKey } from "#/api/queries/users";
import type { ChatMessage } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { MockChatMessage } from "#/testHelpers/chatEntities";
import { ConversationTimeline } from "./ConversationTimeline";
import { parseMessagesWithMergedTools } from "./messageParsing";
import {
	buildWorkingConversation,
	MockCollapsedStepsPreferences,
	WORKING_FIXTURE_START,
	workingFixtureTime,
} from "./storyFixtures";

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
	parameters: {
		queries: [
			{ key: preferenceSettingsKey, data: MockCollapsedStepsPreferences },
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const summary = canvas.getByRole("button", {
			name: "Worked for 12s (2 steps)",
		});
		summary.focus();
		await userEvent.keyboard("{Enter}");
		await userEvent.keyboard(" ");
	},
};

export const Paginated: Story = {
	parameters: {
		queries: [
			{ key: preferenceSettingsKey, data: MockCollapsedStepsPreferences },
		],
	},
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
		await userEvent.click(partial);
		await userEvent.click(
			canvas.getByRole("button", { name: "Load older messages" }),
		);
		await canvas.findByRole("button", {
			name: "Worked for at least 12s (2 steps or more)",
		});
		await userEvent.click(
			canvas.getByRole("button", { name: "Load older messages" }),
		);
		await canvas.findByRole("button", {
			name: "Worked for 12s (2 steps)",
		});
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
	parameters: {
		queries: [
			{ key: preferenceSettingsKey, data: MockCollapsedStepsPreferences },
		],
	},
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
		await waitFor(() => {
			if (viewport.scrollTop !== 0) {
				throw new Error("Waiting for the viewport to reach the top");
			}
		});
		await userEvent.click(
			canvas.getByRole("button", { name: "Load older messages" }),
		);
		await canvas.findByText(/echo step-15$/);
		// History only pages while the reader sits at the very top, where the
		// browser suspends its own scroll anchoring.
		await fireEvent.wheel(viewport, { deltaY: -100 });
		viewport.scrollTop = 0;
		await waitFor(() => {
			if (viewport.scrollTop !== 0) {
				throw new Error("Waiting for the viewport to reach the top");
			}
		});
		await userEvent.click(
			canvas.getByRole("button", { name: "Load older messages" }),
		);
		await canvas.findByText("Run every step");
	},
};

export const PreferenceChanges: Story = {
	parameters: {
		queries: [
			{ key: preferenceSettingsKey, data: MockCollapsedStepsPreferences },
		],
	},
	render: function Render(args) {
		const client = useQueryClient();
		return (
			<>
				<Button
					onClick={() =>
						client.setQueryData(preferenceSettingsKey, {
							...MockCollapsedStepsPreferences,
							collapse_assistant_steps: false,
						})
					}
				>
					Show individual steps
				</Button>
				<Button
					onClick={() =>
						client.setQueryData(
							preferenceSettingsKey,
							MockCollapsedStepsPreferences,
						)
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
		await userEvent.click(
			canvas.getByRole("button", { name: "Restore grouping" }),
		);
		await canvas.findByRole("button", { name: "Worked for 12s (2 steps)" });
	},
};

export const FailedStepCounted: Story = {
	parameters: {
		queries: [
			{ key: preferenceSettingsKey, data: MockCollapsedStepsPreferences },
		],
	},
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
		await userEvent.click(summary);
		const failedStep = canvas.getByTestId("chat-message-message:4");
		await userEvent.click(
			within(failedStep).getByRole("button", { name: /Expand command/ }),
		);
		await within(failedStep).findByText("Command failed");
	},
};

export const QuestionStaysVisible: Story = {
	parameters: {
		queries: [
			{ key: preferenceSettingsKey, data: MockCollapsedStepsPreferences },
		],
	},
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
		await userEvent.click(canvas.getByRole("radio", { name: /Yes/ }));
	},
};

export const Mobile: Story = {
	...Completed,
	globals: { viewport: { value: "mobile1", isRotated: false } },
};

export const EditingPrecedingMessage: Story = {
	parameters: {
		queries: [
			{ key: preferenceSettingsKey, data: MockCollapsedStepsPreferences },
		],
	},
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
		await userEvent.click(canvas.getByRole("button", { name: "Edit prompt" }));
	},
};
