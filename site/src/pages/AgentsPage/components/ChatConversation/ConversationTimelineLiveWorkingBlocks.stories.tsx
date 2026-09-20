import { MessageScroller } from "@shadcn/react/message-scroller";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, userEvent, within } from "storybook/test";
import { preferenceSettingsKey } from "#/api/queries/users";
import type { ChatMessage } from "#/api/typesGenerated";
import { MockChatMessage } from "#/testHelpers/chatEntities";
import { ConversationTimeline } from "./ConversationTimeline";
import {
	getPendingToolCallIDs,
	parseMessagesWithMergedTools,
} from "./messageParsing";
import {
	buildStreamRenderState,
	MockCollapsedStepsPreferences,
	MockWorkingMessages,
	pinFixtureClock,
	workingFixtureTime,
} from "./storyFixtures";

const meta: Meta<typeof ConversationTimeline> = {
	title:
		"pages/AgentsPage/ChatConversation/ConversationTimeline/LiveWorkingBlocks",
	component: ConversationTimeline,
	beforeEach: pinFixtureClock,
	args: {
		organizationId: "organization-id",
		subagentTitles: new Map(),
		chatStatus: "running",
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

export const StreamingFirstStep: Story = {
	args: {
		parsedMessages: parseMessagesWithMergedTools(
			MockWorkingMessages.slice(0, 1),
		),
		...buildStreamRenderState([
			{
				type: "tool-call",
				tool_call_id: "first",
				tool_name: "execute",
				args: { command: "echo first" },
				created_at: workingFixtureTime(1),
			},
		]),
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Working for 12s" }),
		);
	},
};

const MockBetweenStepsMessages = MockWorkingMessages.slice(0, 4);

// Between persisted steps the stream is cleared while a tool runs, so the
// timeline only knows the turn is active from the chat status.
export const RunningBetweenSteps: Story = {
	args: {
		parsedMessages: parseMessagesWithMergedTools(MockBetweenStepsMessages, {
			pendingToolCallIDs: getPendingToolCallIDs(
				MockBetweenStepsMessages,
				"running",
			),
		}),
		liveStatus: { phase: "idle", hasAccumulatedOutput: false },
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Working for 12s" }),
		);
	},
};

// After a tool result the stream reopens empty before the next call streams.
// That moment is the same block still working, so nothing appears under its
// summary.
export const NextStepStartsInsideBlock: Story = {
	args: {
		parsedMessages: parseMessagesWithMergedTools(
			MockWorkingMessages.slice(0, 5),
		),
		streamState: null,
		streamTools: [],
		liveStatus: { phase: "starting", hasAccumulatedOutput: false },
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Working for 12s" }),
		);
	},
};

export const ReasoningBeforeFirstToolFolds: Story = {
	args: {
		parsedMessages: parseMessagesWithMergedTools(
			MockWorkingMessages.slice(0, 1),
		),
		...buildStreamRenderState([
			{
				type: "reasoning",
				text: "Planning the inspection\n\nList the files before reading any.",
				created_at: workingFixtureTime(1),
			},
		]),
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
	created_at: workingFixtureTime(5),
	content: [
		{
			type: "tool-call",
			tool_call_id: "question",
			tool_name: "ask_user_question",
			args: {},
			created_at: workingFixtureTime(5),
		},
	],
};

export const RequiresActionCompletesBlock: Story = {
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
	created_at: workingFixtureTime(5),
	content: [
		{
			type: "tool-call",
			tool_call_id: "editor",
			tool_name: "open_editor",
			args: { path: "README.md" },
			created_at: workingFixtureTime(5),
		},
	],
};

export const RequiresActionKeepsPendingToolVisible: Story = {
	args: {
		chatStatus: "requires_action",
		parsedMessages: parseMessagesWithMergedTools(
			[...MockWorkingMessages.slice(0, 3), MockParkedToolMessage],
			{ pendingToolCallIDs: new Set(["editor"]) },
		),
	},
};

// An active turn longer than the loaded page has no prompt row yet.
export const PromptlessLiveBlock: Story = {
	args: {
		hasMoreMessages: true,
		parsedMessages: parseMessagesWithMergedTools(
			MockWorkingMessages.slice(1, 4),
			{ pendingToolCallIDs: new Set(["second"]) },
		),
		liveStatus: { phase: "idle", hasAccumulatedOutput: false },
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", {
				name: "Working for at least 12s",
			}),
		);
	},
};
