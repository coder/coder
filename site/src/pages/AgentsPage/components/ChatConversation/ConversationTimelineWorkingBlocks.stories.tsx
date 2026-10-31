import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import {
	fireEvent,
	fn,
	screen,
	userEvent,
	waitFor,
	within,
} from "storybook/test";
import { defaultUrlTransform } from "streamdown";
import { chatFileTextKey } from "#/api/queries/chats";
import { preferenceSettingsKey } from "#/api/queries/users";
import { Button } from "#/components/Button/Button";
import {
	MockChatFileMetadata,
	MockChatMessage,
} from "#/testHelpers/chatEntities";
import { MockUserPreferenceSettings } from "#/testHelpers/entities";
import { MessageScroller } from "#/vendor/message-scroller";
import { ConversationTimeline } from "./ConversationTimeline";
import { parseMessagesWithMergedTools } from "./messageParsing";
import {
	buildLiveStatus,
	buildQuestionTimelineArgs,
	buildStreamRenderState,
	MockAttachmentWorkingMessages,
	MockBrokenImageAttachmentMessages,
	MockClearedAttachmentMessages,
	MockCollapsedStepsPreferences,
	MockEmptyQuestionState,
	MockLeadInSearchMessages,
	MockLongTurnPageLoads,
	MockMixedAttachmentMessages,
	MockPageFromAttachedFileMessages,
	MockPendingToolMessage,
	MockPlanAttachmentMessages,
	MockQuestionCallMessage,
	MockSearchAnswerAttachmentMessages,
	MockTextBeforeSecondStepMessage,
	MockUnansweredAttachmentMessages,
	MockUncitedSearchAnswerMessages,
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
		chatStatus: null,
		hasMoreMessages: false,
		parsedMessages: parseMessagesWithMergedTools(MockWorkingMessages),
		askUserQuestionState: MockEmptyQuestionState,
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

// The answer's lead-in text and search fold into the steps' block, and only its
// final text shows after the summary.
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

export const SearchOnlyAnswerFolds: Story = {
	args: {
		parsedMessages: parseMessagesWithMergedTools([
			MockWebSearchAnswerMessages[0],
			...MockWebSearchAnswerMessages.slice(5),
		]),
	},
};

export const UncitedSearchAnswerFoldsExpanded: Story = {
	args: {
		parsedMessages: parseMessagesWithMergedTools(
			MockUncitedSearchAnswerMessages,
		),
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", {
				name: "Worked for 14s (3 steps)",
			}),
		);
	},
};

export const LabeledNarrationFolds: Story = {
	args: {
		parsedMessages: parseMessagesWithMergedTools([
			MockWorkingMessages[0],
			{
				...MockChatMessage,
				id: 2,
				role: "assistant",
				created_at: workingFixtureTime(4),
				content: [
					{
						type: "reasoning",
						text: "Finding the docs page",
						created_at: workingFixtureTime(1),
						completed_at: workingFixtureTime(2),
					},
					{ type: "text", text: "Checking the docs.", narration: true },
					{ type: "text", text: "The docs cover workspace sharing." },
				],
			},
		]),
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Worked for 1s" }),
		);
	},
};

export const LeadInBeforeSearchFolds: Story = {
	args: {
		parsedMessages: parseMessagesWithMergedTools(MockLeadInSearchMessages),
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", {
				name: "Worked for 2s (1 step)",
			}),
		);
	},
};

export const TextBetweenStepsFolds: Story = {
	args: {
		parsedMessages: parseMessagesWithMergedTools([
			...MockWorkingMessages.slice(0, 3),
			MockTextBeforeSecondStepMessage,
			...MockWorkingMessages.slice(4),
		]),
	},
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

// The file's block may continue from older history.
export const PaginatedFromAttachedFile: Story = {
	args: {
		hasMoreMessages: true,
		parsedMessages: parseMessagesWithMergedTools(
			MockPageFromAttachedFileMessages,
		),
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", {
				name: "Worked for at least 11s (1 step or more)",
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

export const AttachmentFoldsIntoBlock: Story = {
	args: {
		parsedMessages: parseMessagesWithMergedTools(MockAttachmentWorkingMessages),
	},
};

export const AttachmentFoldsIntoBlockExpanded: Story = {
	args: {
		parsedMessages: parseMessagesWithMergedTools(MockAttachmentWorkingMessages),
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: /Worked for/ }),
		);
	},
};

const openAttachmentsPill: Story["play"] = async ({ canvasElement }) => {
	await userEvent.click(
		within(canvasElement).getByRole("button", { name: "Attachments (4)" }),
	);
};

export const TurnAttachmentsPill: Story = {
	args: {
		parsedMessages: parseMessagesWithMergedTools(MockMixedAttachmentMessages),
	},
};

export const TurnAttachmentsPillOpen: Story = {
	args: {
		parsedMessages: parseMessagesWithMergedTools(MockMixedAttachmentMessages),
	},
	play: openAttachmentsPill,
};

export const TurnAttachmentsPillOpenMobile: Story = {
	globals: { viewport: { value: "mobile1", isRotated: false } },
	// Pixel ignores the viewport global and needs its own phone width.
	parameters: { pixel: { matrix: { viewports: ["phone"] } } },
	args: {
		parsedMessages: parseMessagesWithMergedTools(MockMixedAttachmentMessages),
	},
	play: openAttachmentsPill,
};

// The preview opens over the popover, which stays open beneath it.
export const TurnAttachmentsPillViewText: Story = {
	parameters: {
		queries: [
			{ key: preferenceSettingsKey, data: MockCollapsedStepsPreferences },
			{
				key: chatFileTextKey("findings-file"),
				data: "# Findings\n\nThe dashboard matches the spec.",
			},
		],
	},
	args: {
		parsedMessages: parseMessagesWithMergedTools(MockMixedAttachmentMessages),
	},
	play: async (context) => {
		await openAttachmentsPill(context);
		await userEvent.click(
			await screen.findByRole("button", { name: "findings.md View file" }),
		);
	},
};

const mockSpecFileMetadata = { ...MockChatFileMetadata, id: "spec-file" };

// Only spec.pdf is still linked to the chat, so findings.md has expired.
export const TurnAttachmentsPillExpiredFile: Story = {
	args: {
		parsedMessages: parseMessagesWithMergedTools(MockMixedAttachmentMessages),
		chatFiles: [mockSpecFileMetadata],
	},
	play: openAttachmentsPill,
};

export const TurnAttachmentsPillBrokenImage: Story = {
	args: {
		parsedMessages: parseMessagesWithMergedTools(
			MockBrokenImageAttachmentMessages,
		),
	},
};

// The running turn gets its pill once it settles; the answered turn before it
// keeps its own.
export const TurnAttachmentsPillRunningTurn: Story = {
	args: {
		parsedMessages: parseMessagesWithMergedTools(
			MockUnansweredAttachmentMessages,
		),
		chatStatus: "running",
	},
};

// A manual compaction runs the chat after the answer, which keeps its pill.
export const TurnAttachmentsPillCompacting: Story = {
	args: {
		parsedMessages: parseMessagesWithMergedTools(MockMixedAttachmentMessages),
		chatStatus: "running",
		...buildStreamRenderState([
			{
				type: "tool-call",
				tool_call_id: "compact",
				tool_name: "chat_summarized",
			},
		]),
	},
};

// Text streaming after the answer can still change the turn, so it shows no
// pill until the chat stops.
export const TurnAttachmentsPillContinuedTurn: Story = {
	args: {
		parsedMessages: parseMessagesWithMergedTools(MockMixedAttachmentMessages),
		chatStatus: "running",
		...buildStreamRenderState([{ type: "text", text: "One more check." }]),
	},
};

const mockTrailingPendingToolMessage = { ...MockPendingToolMessage, id: 23 };

// The pending client tool's result resumes the turn, so it has no pill yet.
export const TurnAttachmentsPillRequiresAction: Story = {
	args: {
		chatStatus: "requires_action",
		parsedMessages: parseMessagesWithMergedTools(
			[...MockUnansweredAttachmentMessages, mockTrailingPendingToolMessage],
			{ pendingToolCallIDs: new Set(["editor"]) },
		),
	},
};

// The context-cleared row after the answer leaves the pill on the answer.
export const TurnAttachmentsPillContextCleared: Story = {
	args: {
		parsedMessages: parseMessagesWithMergedTools(MockClearedAttachmentMessages),
	},
};

// A turn that ends inside its working block shows the pill under the block.
export const TurnAttachmentsPillWithoutAnswer: Story = {
	args: {
		parsedMessages: parseMessagesWithMergedTools(
			MockUnansweredAttachmentMessages,
		),
	},
};

// Expanded, the block of a turn without an answer keeps the pill under it.
export const TurnAttachmentsPillWithoutAnswerExpanded: Story = {
	args: {
		parsedMessages: parseMessagesWithMergedTools(
			MockUnansweredAttachmentMessages,
		),
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", {
				name: "Worked for 7s (4 steps)",
			}),
		);
	},
};

// With steps shown, a turn that ends on an attachment hosts the pill there.
export const TurnAttachmentsPillStepsShown: Story = {
	parameters: {
		queries: [{ key: preferenceSettingsKey, data: MockUserPreferenceSettings }],
	},
	args: {
		parsedMessages: parseMessagesWithMergedTools(
			MockUnansweredAttachmentMessages,
		),
	},
};

// Expanded, the block shows the answer's steps; the pill stays under the
// answer text below it.
export const TurnAttachmentsPillBlockExpanded: Story = {
	args: {
		parsedMessages: parseMessagesWithMergedTools(
			MockSearchAnswerAttachmentMessages,
		),
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", {
				name: "Worked for 15s (4 steps)",
			}),
		);
	},
};

// Until the streamed answer persists, a pill here would sit under the last
// attachment.
export const TurnAttachmentsPillLiveAnswer: Story = {
	parameters: {
		queries: [{ key: preferenceSettingsKey, data: MockUserPreferenceSettings }],
	},
	args: {
		parsedMessages: parseMessagesWithMergedTools(
			MockUnansweredAttachmentMessages,
		),
		chatStatus: "waiting",
		...buildStreamRenderState([
			{ type: "text", text: "The dashboard matches the spec." },
		]),
	},
};

export const PlanAttachmentHidden: Story = {
	parameters: {
		queries: [
			{ key: preferenceSettingsKey, data: MockCollapsedStepsPreferences },
			{
				key: chatFileTextKey("plan-file"),
				data: "# Plan\n\nShip the attachments panel.",
			},
		],
	},
	args: {
		onImplementPlan: fn(),
		parsedMessages: parseMessagesWithMergedTools(MockPlanAttachmentMessages),
	},
};

export const QuestionStaysVisible: Story = {
	args: {
		isChatCompleted: true,
		onSendAskUserQuestionResponse: fn(),
		...buildQuestionTimelineArgs([
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
											{ label: "Yes, deploy now." },
											{ label: "No, keep the changes local." },
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

export const EditingBeforeAnswerWorkFolds: Story = {
	args: {
		parsedMessages: parseMessagesWithMergedTools(MockWebSearchAnswerMessages),
		editingMessageId: 1,
	},
};
