import { MessageScroller } from "@shadcn/react/message-scroller";
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ComponentProps } from "react";
import { QueryClientProvider } from "react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";
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
	MockCollapsedStepsPreferences,
	MockLongTurnPageLoads,
	MockWorkingMessages,
	pinFixtureClock,
	workingFixtureTime,
} from "./storyFixtures";
import { buildStreamTools, createEmptyStreamState } from "./streamState";
import type { StreamState } from "./types";

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
								chatStatus={null}
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

beforeEach(() => pinFixtureClock());

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
			messages: MockLongTurnPageLoads[0],
			hasMoreMessages: true,
		});
		await user.click(
			screen.getByRole("button", { name: /Worked for at least/ }),
		);
		rerenderStage({
			messages: MockLongTurnPageLoads[1],
			hasMoreMessages: true,
		});
		const copyCommand = within(
			screen.getByTestId("chat-message-message:130"),
		).getByRole("button", { name: "Copy command" });
		copyCommand.focus();

		rerenderStage({ messages: MockLongTurnPageLoads[2] });
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
			created_at: workingFixtureTime(1),
			content: [{ type: "text", text: "Looking around first." }],
		};
		const stream = buildStreamRenderState([
			{
				type: "reasoning",
				text: "Planning the inspection",
				created_at: workingFixtureTime(2),
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

const streamingStep = (
	id: string,
	command: string,
	at: string,
): StreamState => ({
	startedAt: at,
	blocks: [{ type: "tool", id }],
	toolCalls: {
		[id]: { id, name: "execute", args: { command } },
	},
	toolResults: {},
	sources: [],
});

const streamingStage = (
	messages: ChatMessage[],
	stream: StreamState,
): TimelineStage => ({
	messages,
	chatStatus: "running",
	streamState: stream,
	streamTools: buildStreamTools(stream.toolCalls, stream.toolResults),
	liveStatus: { phase: "streaming", hasAccumulatedOutput: true },
});

const idleLive = { phase: "idle", hasAccumulatedOutput: false } as const;

describe("ConversationTimeline live working blocks", () => {
	it("keeps an open block mounted from streaming steps through durable completion", async () => {
		const user = userEvent.setup();
		const { rerenderStage } = renderTimeline(
			streamingStage(
				MockWorkingMessages.slice(0, 1),
				streamingStep("first", "echo first", workingFixtureTime(1)),
			),
		);
		const summary = screen.getByRole("button", { name: "Working for 12s" });
		await user.click(summary);

		rerenderStage(
			streamingStage(
				MockWorkingMessages.slice(0, 3),
				streamingStep("second", "echo second", workingFixtureTime(5)),
			),
		);
		expect(summary).toHaveFocus();
		const copyCommand = within(
			screen.getByTestId("chat-message-message:2"),
		).getByRole("button", { name: "Copy command" });
		copyCommand.focus();

		rerenderStage({
			messages: MockWorkingMessages,
			chatStatus: "waiting",
			streamState: null,
			streamTools: [],
			liveStatus: idleLive,
		});
		expect(copyCommand).toHaveFocus();
	});

	it("keeps an open block mounted when a running turn without a stream completes", async () => {
		const user = userEvent.setup();
		const messages = MockWorkingMessages.slice(0, 4);
		const { rerenderStage } = renderTimeline({
			messages,
			pendingToolCallIDs: getPendingToolCallIDs(messages, "running"),
			chatStatus: "running",
			liveStatus: idleLive,
		});
		await user.click(screen.getByRole("button", { name: "Working for 12s" }));
		const copyCommand = within(
			screen.getByTestId("chat-message-message:2"),
		).getByRole("button", { name: "Copy command" });
		copyCommand.focus();

		rerenderStage({ messages, chatStatus: "waiting", liveStatus: idleLive });
		expect(copyCommand).toHaveFocus();
	});

	it("keeps the live disclosure mounted while the next step starts", () => {
		const messages = MockWorkingMessages.slice(0, 5);
		const { rerenderStage } = renderTimeline({
			messages,
			chatStatus: "running",
			streamState: null,
			streamTools: [],
			liveStatus: { phase: "starting", hasAccumulatedOutput: false },
		});
		const summary = screen.getByRole("button", { name: "Working for 12s" });
		summary.focus();

		rerenderStage({
			messages,
			chatStatus: "running",
			streamState: createEmptyStreamState(),
			streamTools: [],
			liveStatus: { phase: "streaming", hasAccumulatedOutput: false },
		});
		expect(summary).toHaveFocus();

		rerenderStage({
			messages,
			chatStatus: "running",
			...buildStreamRenderState([
				{
					type: "reasoning",
					text: "Planning the inspection",
					created_at: workingFixtureTime(1),
				},
			]),
		});
		expect(summary).toHaveFocus();
	});

	it("keeps an open promptless block mounted through completion and prompt prepend", async () => {
		const user = userEvent.setup();
		const { rerenderStage } = renderTimeline({
			messages: MockWorkingMessages.slice(1, 4),
			pendingToolCallIDs: new Set(["second"]),
			hasMoreMessages: true,
			chatStatus: "running",
			liveStatus: idleLive,
		});
		await user.click(
			screen.getByRole("button", { name: "Working for at least 12s" }),
		);
		const copyCommand = within(
			screen.getByTestId("chat-message-message:2"),
		).getByRole("button", { name: "Copy command" });
		copyCommand.focus();

		rerenderStage({
			messages: MockWorkingMessages.slice(1),
			hasMoreMessages: true,
			chatStatus: "waiting",
			liveStatus: idleLive,
		});
		expect(copyCommand).toHaveFocus();

		rerenderStage({
			messages: MockWorkingMessages,
			chatStatus: "waiting",
			liveStatus: idleLive,
		});
		expect(copyCommand).toHaveFocus();
	});
});
