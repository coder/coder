import { MessageScroller } from "@shadcn/react/message-scroller";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { fireEvent, fn, userEvent, waitFor, within } from "storybook/test";
import { preferenceSettingsKey } from "#/api/queries/users";
import { Button } from "#/components/Button/Button";
import { MockChatMessage } from "#/testHelpers/chatEntities";
import { ConversationTimeline } from "./ConversationTimeline";
import { parseMessagesWithMergedTools } from "./messageParsing";
import {
	MockCollapsedStepsPreferences,
	MockLongTurnPageLoads,
	MockWorkingMessages,
	pinFixtureClock,
	workingFixtureTime,
} from "./storyFixtures";

const meta: Meta<typeof ConversationTimeline> = {
	title: "pages/AgentsPage/ChatConversation/ConversationTimeline/WorkingBlocks",
	component: ConversationTimeline,
	beforeEach: pinFixtureClock,
	args: {
		organizationId: "organization-id",
		subagentTitles: new Map(),
		chatStatus: null,
		parsedMessages: parseMessagesWithMergedTools(MockWorkingMessages),
	},
	parameters: {
		queries: [
			{ key: preferenceSettingsKey, data: MockCollapsedStepsPreferences },
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

export const Completed: Story = {};

export const Expanded: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", {
				name: "Worked for 12s (2 steps)",
			}),
		);
	},
};

export const Paginated: Story = {
	args: {
		hasMoreMessages: true,
		parsedMessages: parseMessagesWithMergedTools(MockWorkingMessages.slice(3)),
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", {
				name: "Worked for at least 8s (1 step or more)",
			}),
		);
	},
};

// Older rows land inside the expanded block's own scroller item, so the block
// rather than the scroller has to keep the reading position.
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
					parsedMessages={parseMessagesWithMergedTools(
						MockLongTurnPageLoads[page],
					)}
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
									created_at: workingFixtureTime(13),
								},
							],
						}
					: message,
			),
		),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Testing Library pads the badge with spaces; browsers read the name
		// as "Worked for 12s (2 steps), 1 failed step".
		const summary = canvas.getByRole("button", {
			name: /^Worked for 12s \(2 steps\)\s?,\s?1 failed step$/,
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
	args: {
		isChatCompleted: true,
		onSendAskUserQuestionResponse: fn(),
		parsedMessages: parseMessagesWithMergedTools([
			...MockWorkingMessages.slice(0, 3),
			{
				...MockChatMessage,
				id: 4,
				role: "assistant",
				created_at: workingFixtureTime(5),
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
				created_at: workingFixtureTime(5),
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
	globals: { viewport: { value: "mobile1", isRotated: false } },
};

// Editing makes the block inert, so it has to expand before the edit starts;
// the capture shows the outer item dimming the nested rows once.
export const EditingPrecedingMessage: Story = {
	render: function Render(args) {
		const [editing, setEditing] = useState(false);
		return (
			<>
				<Button onClick={() => setEditing(true)} disabled={editing}>
					Edit prompt
				</Button>
				<ConversationTimeline {...args} editingMessageId={editing ? 1 : null} />
			</>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Worked for 12s (2 steps)" }),
		);
		await userEvent.click(canvas.getByRole("button", { name: "Edit prompt" }));
	},
};
