import { MessageScroller } from "@shadcn/react/message-scroller";
import { screen, within } from "@testing-library/react";
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
	buildStreamRenderState,
	buildWorkingConversation,
	MockCollapsedStepsPreferences,
	MockLongTurnPages,
	WORKING_FIXTURE_START,
	workingFixtureTime,
} from "./storyFixtures";

const time = workingFixtureTime;
const MockWorkingMessages = buildWorkingConversation();

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
		rerenderStage: (stage: TimelineStage) => rerender(renderStage(stage)),
	};
}

describe("ConversationTimeline working blocks", () => {
	it("keeps an open block mounted as older pages join it", async () => {
		const user = userEvent.setup();
		const { rerenderStage } = renderTimeline({
			messages: MockWorkingMessages.slice(3),
			hasMoreMessages: true,
		});
		await user.click(
			screen.getByRole("button", {
				name: "Worked for at least 8s (1 step or more)",
			}),
		);
		const copyCommand = within(
			screen.getByTestId("chat-message-message:4"),
		).getByRole("button", { name: "Copy command" });
		copyCommand.focus();

		rerenderStage({
			messages: MockWorkingMessages.slice(1),
			hasMoreMessages: true,
		});
		expect(copyCommand).toHaveFocus();
		rerenderStage({ messages: MockWorkingMessages });
		expect(copyCommand).toHaveFocus();
	});

	it("keeps an existing step mounted when older rows are prepended", async () => {
		const user = userEvent.setup();
		const { rerenderStage } = renderTimeline({
			messages: MockLongTurnPages[0],
			hasMoreMessages: true,
		});
		await user.click(
			screen.getByRole("button", { name: /Worked for at least/ }),
		);
		rerenderStage({ messages: MockLongTurnPages[1], hasMoreMessages: true });
		const copyCommand = within(
			screen.getByTestId("chat-message-message:130"),
		).getByRole("button", { name: "Copy command" });
		copyCommand.focus();

		rerenderStage({ messages: MockLongTurnPages[2] });
		expect(copyCommand).toHaveFocus();
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

	it("keeps an open live-only block mounted when its prompt page arrives", async () => {
		const user = userEvent.setup();
		const assistantNote: ChatMessage = {
			...MockChatMessage,
			id: 2,
			role: "assistant",
			created_at: time(1),
			content: [{ type: "text", text: "Looking around first." }],
		};
		const stream = buildStreamRenderState([
			{
				type: "reasoning",
				text: "Planning the inspection",
				created_at: time(2),
			},
		]);
		const { rerenderStage } = renderTimeline({
			messages: [assistantNote],
			hasMoreMessages: true,
			chatStatus: "running",
			...stream,
		});
		const summary = screen.getByRole("button", { name: /^Working/ });
		await user.click(summary);
		expect(summary).toHaveFocus();

		rerenderStage({
			messages: [MockWorkingMessages[0], assistantNote],
			hasMoreMessages: true,
			chatStatus: "running",
			...stream,
		});
		expect(summary).toHaveFocus();
	});
});
