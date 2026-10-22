import { act, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type * as motionReact from "motion/react";
import { useReducedMotion } from "motion/react";
import { QueryClientProvider } from "react-query";
import { defaultUrlTransform } from "streamdown";
import {
	afterAll,
	afterEach,
	beforeAll,
	beforeEach,
	describe,
	expect,
	it,
	vi,
} from "vitest";
import { preferenceSettingsKey } from "#/api/queries/users";
import type { ChatMessage } from "#/api/typesGenerated";
import { MockChatMessage } from "#/testHelpers/chatEntities";
import {
	createTestQueryClient,
	renderComponent,
} from "#/testHelpers/renderHelpers";
import { MessageScroller } from "#/vendor/message-scroller";
import { ConversationTimeline } from "./ConversationTimeline";
import {
	getPendingToolCallIDs,
	parseMessagesWithMergedTools,
} from "./messageParsing";
import {
	buildLiveStatus,
	buildStreamRenderState,
	MockCollapsedStepsPreferences,
	MockLongTurnPageLoads,
	MockWebSearchAnswerMessages,
	MockWorkingMessages,
	pinFixtureClock,
	workingFixtureTime,
} from "./storyFixtures";
import { createEmptyStreamState } from "./streamState";

type TimelineStage = {
	messages?: ChatMessage[];
	pendingToolCallIDs?: ReadonlySet<string>;
} & Partial<React.ComponentProps<typeof ConversationTimeline>>;

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
								urlTransform={defaultUrlTransform}
								mcpServers={[]}
								streamTools={[]}
								liveStatus={buildLiveStatus()}
								subagentStatusOverrides={new Map()}
								subagentTitles={new Map()}
								subagentVariants={new Map()}
								parsedMessages={parseMessagesWithMergedTools(messages, {
									pendingToolCallIDs,
								})}
								chatStatus={null}
								hasMoreMessages={false}
								isChatCompleted
								showDesktopPreviews={false}
								hasActiveStream={false}
								isAwaitingFirstStreamChunk={false}
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

// Motion keeps the requestAnimationFrame it finds when it loads, so route it
// through setTimeout first; then fake timers step Motion's frames.
vi.hoisted(() => {
	vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) =>
		setTimeout(() => callback(performance.now()), 16),
	);
});

vi.mock("motion/react", async (importOriginal) => ({
	...(await importOriginal<typeof motionReact>()),
	useReducedMotion: vi.fn(() => false),
}));

beforeEach(() => pinFixtureClock());

const idleLive = { phase: "idle", hasAccumulatedOutput: false } as const;

const mockAssistantNote: ChatMessage = {
	...MockChatMessage,
	id: 2,
	role: "assistant",
	created_at: workingFixtureTime(1),
	content: [{ type: "text", text: "Looking around first." }],
};

const focusCopyCommand = (messageId: number) => {
	const button = within(
		screen.getByTestId(`chat-message-message:${messageId}`),
	).getByRole("button", { name: "Copy command" });
	button.focus();
	return button;
};

describe("ConversationTimeline working blocks", () => {
	it("copies the whole message from the answer below its expanded block", async () => {
		const user = userEvent.setup();
		const writeText = vi
			.spyOn(navigator.clipboard, "writeText")
			.mockResolvedValue();
		renderTimeline({ messages: MockWebSearchAnswerMessages });

		await user.click(
			screen.getByRole("button", { name: "Worked for 15s (3 steps)" }),
		);
		await user.click(
			within(screen.getByTestId("chat-message-message:6")).getByRole("button", {
				name: "Copy message",
			}),
		);

		expect(writeText).toHaveBeenCalledWith(
			"Looking up the release notes.The workspace runs the latest Coder release.",
		);
	});

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
		const copyCommand = focusCopyCommand(4);

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
		const copyCommand = focusCopyCommand(130);

		rerenderStage({ messages: MockLongTurnPageLoads[2] });
		expect(copyCommand).toHaveFocus();
	});

	it.each([
		["the rest of the turn persists", MockWorkingMessages],
		["no further messages arrive", MockWorkingMessages.slice(0, 4)],
	])(
		"keeps focus inside an open block across the live-to-durable handoff when %s",
		async (_, finalMessages) => {
			const user = userEvent.setup();
			const messages = MockWorkingMessages.slice(0, 4);
			const { rerenderStage } = renderTimeline({
				messages,
				pendingToolCallIDs: getPendingToolCallIDs(messages, "running"),
				chatStatus: "running",
				liveStatus: idleLive,
			});

			await user.click(screen.getByRole("button", { name: "Working for 12s" }));
			const copyCommand = focusCopyCommand(2);

			rerenderStage({
				messages: finalMessages,
				chatStatus: "waiting",
				liveStatus: idleLive,
			});
			expect(copyCommand).toHaveFocus();
		},
	);

	it("keeps focus inside an open live block when its prompt page arrives", async () => {
		const user = userEvent.setup();
		const messages = MockWorkingMessages.slice(1, 4);
		const pendingToolCallIDs = getPendingToolCallIDs(messages, "running");
		const { rerenderStage } = renderTimeline({
			messages,
			pendingToolCallIDs,
			hasMoreMessages: true,
			chatStatus: "running",
			liveStatus: idleLive,
		});

		await user.click(
			screen.getByRole("button", { name: "Working for at least 12s" }),
		);
		const copyCommand = focusCopyCommand(2);

		rerenderStage({
			messages: MockWorkingMessages.slice(0, 4),
			pendingToolCallIDs,
			hasMoreMessages: true,
			chatStatus: "running",
			liveStatus: idleLive,
		});
		expect(copyCommand).toHaveFocus();
	});

	it("keeps an open live-only block mounted when its prompt page arrives", async () => {
		const user = userEvent.setup();
		const stream = buildStreamRenderState([
			{
				type: "reasoning",
				text: "Planning the inspection",
				created_at: workingFixtureTime(2),
			},
		]);
		const { rerenderStage } = renderTimeline({
			messages: [mockAssistantNote],
			hasMoreMessages: true,
			chatStatus: "running",
			...stream,
		});

		const summary = screen.getByRole("button", { name: /^Working/ });
		await user.click(summary);
		expect(summary).toHaveFocus();

		rerenderStage({
			messages: [MockWorkingMessages[0], mockAssistantNote],
			hasMoreMessages: true,
			chatStatus: "running",
			...stream,
		});
		expect(summary).toHaveFocus();
	});

	it("keeps an open live-only block mounted when its prompt page and persisted step arrive together", async () => {
		const user = userEvent.setup();
		const mockStep: ChatMessage = { ...MockWorkingMessages[3], id: 3 };
		const { rerenderStage } = renderTimeline({
			messages: [mockAssistantNote],
			hasMoreMessages: true,
			chatStatus: "running",
			...buildStreamRenderState(mockStep.content ?? []),
		});

		const summary = screen.getByRole("button", { name: /^Working/ });
		await user.click(summary);

		rerenderStage({
			messages: [MockWorkingMessages[0], mockAssistantNote, mockStep],
			pendingToolCallIDs: new Set(["second"]),
			hasMoreMessages: true,
			chatStatus: "running",
			liveStatus: idleLive,
		});
		expect(summary).toHaveFocus();
	});

	it("keeps an open live block mounted when an older page reveals an earlier block of its turn", async () => {
		const user = userEvent.setup();
		const mockNote: ChatMessage = {
			...MockChatMessage,
			id: 4,
			role: "assistant",
			created_at: workingFixtureTime(5),
			content: [{ type: "text", text: "Found the config." }],
		};
		const mockLiveStep: ChatMessage = { ...MockWorkingMessages[3], id: 5 };
		const stage = (messages: ChatMessage[]): TimelineStage => ({
			messages,
			pendingToolCallIDs: new Set(["second"]),
			hasMoreMessages: true,
			chatStatus: "running",
			liveStatus: idleLive,
		});

		const { rerenderStage } = renderTimeline(stage([mockLiveStep]));

		await user.click(
			screen.getByRole("button", { name: "Working for at least 8s" }),
		);
		const copyCommand = focusCopyCommand(5);

		// The earlier block takes the head ordinal the live block had.
		rerenderStage(
			stage([...MockWorkingMessages.slice(1, 3), mockNote, mockLiveStep]),
		);
		expect(copyCommand).toHaveFocus();
	});
});

const streamingStage = (
	messages: ChatMessage[],
	toolCallId: string,
	seconds: number,
): TimelineStage => ({
	messages,
	chatStatus: "running",
	...buildStreamRenderState([
		{
			type: "tool-call",
			tool_call_id: toolCallId,
			tool_name: "execute",
			args: { command: `echo ${toolCallId}` },
			created_at: workingFixtureTime(seconds),
		},
	]),
});

describe("ConversationTimeline live working blocks", () => {
	it("keeps an open block mounted from streaming steps through durable completion", async () => {
		const user = userEvent.setup();
		const { rerenderStage } = renderTimeline(
			streamingStage(MockWorkingMessages.slice(0, 1), "first", 1),
		);

		const summary = screen.getByRole("button", { name: "Working for 12s" });
		await user.click(summary);

		rerenderStage(streamingStage(MockWorkingMessages.slice(0, 3), "second", 5));
		expect(summary).toHaveFocus();

		const copyCommand = focusCopyCommand(2);

		rerenderStage({
			messages: MockWorkingMessages,
			chatStatus: "waiting",
			streamState: null,
			streamTools: [],
			liveStatus: idleLive,
		});
		expect(copyCommand).toHaveFocus();
	});

	it("keeps the live reasoning mounted in an open block when the answer starts", async () => {
		const user = userEvent.setup();
		const messages = MockWorkingMessages.slice(0, 5);
		const reasoning = {
			type: "reasoning",
			text: "Summarizing the inspection",
			created_at: workingFixtureTime(13),
		} as const;
		const { rerenderStage } = renderTimeline({
			messages,
			chatStatus: "running",
			...buildStreamRenderState([reasoning]),
		});

		await user.click(screen.getByRole("button", { name: "Working for 12s" }));
		const thinking = await screen.findByRole("button", {
			name: /Summarizing/,
		});
		thinking.focus();

		rerenderStage({
			messages,
			chatStatus: "running",
			...buildStreamRenderState([
				reasoning,
				{ type: "text", text: "The workspace looks healthy." },
			]),
		});
		expect(thinking).toHaveFocus();
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
		const copyCommand = focusCopyCommand(2);

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

const mockReplyText =
	"I'll start by listing the workspace files so I can see how the project is laid out, then read the main entry point.";

const mockReplyPart = { type: "text", text: mockReplyText } as const;

const mockFirstToolCallPart = {
	type: "tool-call",
	tool_call_id: "first",
	tool_name: "execute",
	args: { command: "echo first" },
	created_at: workingFixtureTime(1),
} as const;

const mockSecondToolCallPart = {
	...mockFirstToolCallPart,
	tool_call_id: "second",
	args: { command: "echo second" },
	created_at: workingFixtureTime(5),
} as const;

const liveStage = (
	messages: ChatMessage[],
	parts: Parameters<typeof buildStreamRenderState>[0],
): TimelineStage => ({
	messages,
	chatStatus: "running",
	...buildStreamRenderState(parts),
});

// Motion reports completion through a promise, so timers advance async.
const advance = (ms: number) =>
	act(async () => {
		await vi.advanceTimersByTimeAsync(ms);
	});

const isFolding = (text: string) => screen.queryByText(text) !== null;

const announcement = () => screen.getByRole("status").textContent;

// Timing and queueing are covered in WorkingBlockTransition.test.tsx; these
// cover which reply the timeline hands to the transition.
describe("ConversationTimeline reply folding", () => {
	// One fake clock for the whole block: Motion's frame loop keeps waiting on
	// a frame scheduled on a clock that a later test replaced.
	beforeAll(() => {
		vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout", "performance"] });
	});
	afterAll(() => {
		vi.useRealTimers();
	});
	beforeEach(() => {
		vi.mocked(useReducedMotion).mockReturnValue(false);
	});
	afterEach(async () => {
		// Let the previous test's animations finish before the next renders.
		await advance(1000);
	});

	it("folds into a label that is already showing", async () => {
		const firstStep = MockWorkingMessages.slice(0, 3);
		const { rerenderStage } = renderTimeline(
			liveStage(firstStep, [mockReplyPart]),
		);

		rerenderStage(
			liveStage(firstStep, [mockReplyPart, mockSecondToolCallPart]),
		);
		expect(isFolding(mockReplyText)).toBe(true);
		await advance(350);
		expect(isFolding(mockReplyText)).toBe(false);
		// No label entered, so there is nothing new to announce.
		expect(announcement()).toBe("");
	});

	it("folds reply text when a web search follows it", async () => {
		const firstStep = MockWorkingMessages.slice(0, 3);
		const { rerenderStage } = renderTimeline(
			liveStage(firstStep, [mockReplyPart]),
		);

		rerenderStage(
			liveStage(firstStep, [
				mockReplyPart,
				{
					type: "tool-call",
					tool_call_id: "search",
					tool_name: "web_search",
					provider_executed: true,
					created_at: workingFixtureTime(5),
				},
			]),
		);
		expect(isFolding(mockReplyText)).toBe(true);
		await advance(350);
		expect(isFolding(mockReplyText)).toBe(false);
	});

	it("enters the label without a fold when a tool call starts first", async () => {
		const prompt = MockWorkingMessages.slice(0, 1);
		const { rerenderStage } = renderTimeline({
			messages: prompt,
			chatStatus: "running",
			liveStatus: { phase: "starting", hasAccumulatedOutput: false },
		});

		rerenderStage(liveStage(prompt, [mockFirstToolCallPart]));
		expect(isFolding(mockReplyText)).toBe(false);
		await advance(350);
		expect(announcement()).toBe("Working");
	});
});
