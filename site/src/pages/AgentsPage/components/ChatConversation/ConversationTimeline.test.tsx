import { MessageScroller } from "@shadcn/react/message-scroller";
import {
	screen,
	waitForElementToBeRemoved,
	within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ComponentProps } from "react";
import { QueryClientProvider } from "react-query";
import { describe, expect, it, vi } from "vitest";
import { preferenceSettingsKey } from "#/api/queries/users";
import type { ChatMessage } from "#/api/typesGenerated";
import { MockChatMessage } from "#/testHelpers/chatEntities";
import {
	createTestQueryClient,
	renderComponent,
} from "#/testHelpers/renderHelpers";
import { ConversationTimeline } from "./ConversationTimeline";
import {
	getPendingToolCallIDs,
	parseMessagesWithMergedTools,
} from "./messageParsing";
import {
	buildWorkingConversation,
	MockCollapsedStepsPreferences,
	MockLongTurnPages,
	WORKING_FIXTURE_START,
	workingFixtureTime,
} from "./storyFixtures";

const time = workingFixtureTime;
const MockWorkingMessages = buildWorkingConversation();
const MockFailedMessages: ChatMessage[] = MockWorkingMessages.map((message) =>
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
);
const MockQuestionMessages: ChatMessage[] = [
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
];

type TimelineStage = {
	messages?: ChatMessage[];
	pendingToolCallIDs?: ReadonlySet<string>;
} & Partial<
	Pick<
		ComponentProps<typeof ConversationTimeline>,
		| "hasMoreMessages"
		| "chatStatus"
		| "liveStatus"
		| "streamState"
		| "streamTools"
	>
>;

function renderTimeline(initial: TimelineStage = {}) {
	const queryClient = createTestQueryClient();
	queryClient.setQueryDefaults(preferenceSettingsKey, {
		staleTime: Number.POSITIVE_INFINITY,
	});
	queryClient.setQueryData(
		preferenceSettingsKey,
		MockCollapsedStepsPreferences,
	);
	const renderStage = ({
		messages = MockWorkingMessages,
		pendingToolCallIDs,
		...props
	}: TimelineStage) => (
		<QueryClientProvider client={queryClient}>
			<MessageScroller.Provider autoScroll defaultScrollPosition="end">
				<MessageScroller.Root>
					<MessageScroller.Viewport>
						<MessageScroller.Content>
							<ConversationTimeline
								organizationId="organization-id"
								subagentTitles={new Map()}
								parsedMessages={parseMessagesWithMergedTools(messages, {
									pendingToolCallIDs,
								})}
								now={WORKING_FIXTURE_START + 13000}
								isChatCompleted
								onSendAskUserQuestionResponse={vi.fn()}
								{...props}
							/>
						</MessageScroller.Content>
					</MessageScroller.Viewport>
				</MessageScroller.Root>
			</MessageScroller.Provider>
		</QueryClientProvider>
	);
	const { rerender } = renderComponent(renderStage(initial));
	return {
		queryClient,
		rerenderStage: (stage: TimelineStage) => rerender(renderStage(stage)),
	};
}

describe("ConversationTimeline working blocks", () => {
	it("records keyboard expansion changes for a working block", async () => {
		const user = userEvent.setup();
		renderTimeline();
		const summary = screen.getByRole("button", {
			name: "Worked for 12s (2 steps)",
		});
		summary.focus();
		await user.keyboard("{Enter}");
		expect(summary.getAttribute("aria-expanded")).toBe("true");
		await user.keyboard(" ");
		expect(summary.getAttribute("aria-expanded")).toBe("false");
	});

	it("preserves disclosure identity and expansion as older pages join a block", async () => {
		const user = userEvent.setup();
		const { rerenderStage } = renderTimeline({
			messages: MockWorkingMessages.slice(3),
			hasMoreMessages: true,
		});
		const summary = screen.getByRole("button", {
			name: "Worked for at least 8s (1 step or more)",
		});
		await user.click(summary);
		rerenderStage({
			messages: MockWorkingMessages.slice(1),
			hasMoreMessages: true,
		});
		const grown = screen.getByRole("button", {
			name: "Worked for at least 12s (2 steps or more)",
		});
		expect(grown).toBe(summary);
		expect(grown.getAttribute("aria-expanded")).toBe("true");
		rerenderStage({ messages: MockWorkingMessages });
		const complete = screen.getByRole("button", {
			name: "Worked for 12s (2 steps)",
		});
		expect(complete).toBe(summary);
		expect(complete.getAttribute("aria-expanded")).toBe("true");
	});

	it("preserves an existing step node when older rows are prepended", async () => {
		const user = userEvent.setup();
		const { rerenderStage } = renderTimeline({
			messages: MockLongTurnPages[0],
			hasMoreMessages: true,
		});
		await user.click(
			screen.getByRole("button", { name: /Worked for at least/ }),
		);
		rerenderStage({ messages: MockLongTurnPages[1], hasMoreMessages: true });
		const step = screen.getByText(/echo step-15$/);
		rerenderStage({ messages: MockLongTurnPages[2] });
		expect(screen.getByText(/echo step-15$/)).toBe(step);
	});

	it("restores the previous expansion decision after grouping is reenabled", async () => {
		const user = userEvent.setup();
		const { queryClient } = renderTimeline();
		const summary = screen.getByRole("button", {
			name: "Worked for 12s (2 steps)",
		});
		await user.click(summary);
		queryClient.setQueryData(preferenceSettingsKey, {
			...MockCollapsedStepsPreferences,
			collapse_assistant_steps: false,
		});
		await waitForElementToBeRemoved(summary);
		queryClient.setQueryData(
			preferenceSettingsKey,
			MockCollapsedStepsPreferences,
		);
		const restored = await screen.findByRole("button", {
			name: "Worked for 12s (2 steps)",
		});
		expect(restored.getAttribute("aria-expanded")).toBe("true");
	});

	it.each([
		{
			scenario: "completed working blocks",
			messages: MockWorkingMessages,
			name: "Worked for 12s (2 steps)",
		},
		{
			scenario: "failed working blocks",
			messages: MockFailedMessages,
			name: /Worked for 12s \(2 steps\)\s*1 failed step/,
		},
		{
			scenario: "the block preceding an interactive question",
			messages: MockQuestionMessages,
			name: "Worked for 3s (1 step)",
		},
	])("starts $scenario collapsed", ({ messages, name }) => {
		renderTimeline({ messages });
		const summary = screen.getByRole("button", { name });
		expect(summary.getAttribute("aria-expanded")).toBe("false");
	});

	it("keeps focus inside an open block across the live-to-durable handoff", async () => {
		const user = userEvent.setup();
		const messages = MockWorkingMessages.slice(0, 4);
		const { rerenderStage } = renderTimeline({
			messages,
			pendingToolCallIDs: getPendingToolCallIDs(messages, "running"),
			chatStatus: "running",
			liveStatus: { phase: "idle", hasAccumulatedOutput: false },
		});
		await user.click(screen.getByRole("button", { name: "Working for 12s" }));
		const copyCommand = within(
			screen.getByTestId("chat-message-message:2"),
		).getByRole("button", { name: "Copy command" });
		copyCommand.focus();

		rerenderStage({
			messages: MockWorkingMessages,
			chatStatus: "waiting",
			liveStatus: { phase: "idle", hasAccumulatedOutput: false },
		});
		expect(copyCommand).toHaveFocus();
	});

	it("keeps focus inside an open live block when its prompt page arrives", async () => {
		const user = userEvent.setup();
		const messages = MockWorkingMessages.slice(1, 4);
		const pendingToolCallIDs = getPendingToolCallIDs(messages, "running");
		const { rerenderStage } = renderTimeline({
			messages,
			pendingToolCallIDs,
			hasMoreMessages: true,
			chatStatus: "running",
			liveStatus: { phase: "idle", hasAccumulatedOutput: false },
		});
		await user.click(
			screen.getByRole("button", { name: "Working for at least 12s" }),
		);
		const copyCommand = within(
			screen.getByTestId("chat-message-message:2"),
		).getByRole("button", { name: "Copy command" });
		copyCommand.focus();

		rerenderStage({
			messages: MockWorkingMessages.slice(0, 4),
			pendingToolCallIDs,
			hasMoreMessages: true,
			chatStatus: "running",
			liveStatus: { phase: "idle", hasAccumulatedOutput: false },
		});
		expect(copyCommand).toHaveFocus();
	});
});
