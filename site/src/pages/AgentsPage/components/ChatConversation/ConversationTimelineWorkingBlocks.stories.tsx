import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { fireEvent, fn, userEvent, waitFor, within } from "storybook/test";
import { defaultUrlTransform } from "streamdown";
import { preferenceSettingsKey } from "#/api/queries/users";
import { Button } from "#/components/Button/Button";
import { MockChatMessage } from "#/testHelpers/chatEntities";
import { MessageScroller } from "#/vendor/message-scroller";
import { ConversationTimeline } from "./ConversationTimeline";
import { parseMessagesWithMergedTools } from "./messageParsing";
import {
	buildLiveStatus,
	MockCollapsedStepsPreferences,
	MockLongTurnPageLoads,
	MockQuestionCallMessage,
	MockWebSearchAnswerMessages,
	MockWorkingMessages,
	pinFixtureClock,
	workingFixtureTime,
} from "./storyFixtures";

const meta: Meta<typeof ConversationTimeline> = {
	title: "pages/AgentsPage/ChatConversation/ConversationTimeline/WorkingBlocks",
	component: ConversationTimeline,
	beforeEach: pinFixtureClock,
	parameters: {
		queries: [
			{ key: preferenceSettingsKey, data: MockCollapsedStepsPreferences },
		],
	},
	args: {
		organizationId: "organization-id",
		urlTransform: defaultUrlTransform,
		mcpServers: [],
		streamTools: [],
		liveStatus: buildLiveStatus(),
		subagentStatusOverrides: new Map(),
		subagentTitles: new Map(),
		subagentVariants: new Map(),
		isChatCompleted: false,
		showDesktopPreviews: false,
		hasActiveStream: false,
		isAwaitingFirstStreamChunk: false,
		automationNames: { names: new Map(), status: "settled" },
		chatStatus: null,
		hasMoreMessages: false,
		parsedMessages: parseMessagesWithMergedTools(MockWorkingMessages),
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

// The answer's reasoning and web search fold into the block it ends, so only
// its text shows after the summary.
export const AnswerWorkFolds: Story = {
	args: {
		parsedMessages: parseMessagesWithMergedTools(MockWebSearchAnswerMessages),
	},
};

export const AnswerWorkFoldsExpanded: Story = {
	args: {
		parsedMessages: parseMessagesWithMergedTools(MockWebSearchAnswerMessages),
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", {
				name: "Worked for 15s (3 steps)",
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

export const PrependIntoExpandedBlockKeepsReadingPosition: Story = {
	render: function Render(args) {
		const [page, setPage] = useState(0);

		// Placed after the rows, as on the real page, and fixed so clicking it
		// never scrolls the viewport.
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
		// History pages at the very top, and the scroller only stops following
		// the bottom on user input, so wheel up first.
		const loadOlderFromTop = async () => {
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
		};

		await loadOlderFromTop();
		await canvas.findByText(/echo step-15$/);

		await loadOlderFromTop();
		await canvas.findByText("Run every step");
	},
};

export const QuestionStaysVisible: Story = {
	args: {
		isChatCompleted: true,
		onSendAskUserQuestionResponse: fn(),
		parsedMessages: parseMessagesWithMergedTools([
			...MockWorkingMessages.slice(0, 3),
			MockQuestionCallMessage,
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
