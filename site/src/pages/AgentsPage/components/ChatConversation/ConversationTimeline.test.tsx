import { act, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "react-query";
import { defaultUrlTransform } from "streamdown";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
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

type FakeAnimation = {
	element: Element;
	keyframes: Keyframe[];
	options: KeyframeAnimationOptions;
	finish: () => void;
};

// jsdom has no Web Animations API; stub it the way test/setup/domStubs.ts
// stubs other missing element methods.
const trackAnimations = () => {
	const animations: FakeAnimation[] = [];
	HTMLElement.prototype.animate = vi.fn().mockImplementation(function (
		this: Element,
		keyframes: Keyframe[],
		options: KeyframeAnimationOptions,
	) {
		const animation: { onfinish: (() => void) | null; cancel: () => void } = {
			onfinish: null,
			cancel: vi.fn(),
		};
		animations.push({
			element: this,
			keyframes,
			options,
			finish: () => act(() => animation.onfinish?.()),
		});
		return animation;
	});
	return animations;
};

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

const entersLabel = ({ keyframes }: FakeAnimation) =>
	keyframes[0].opacity === 0 && keyframes[0].height === "0px";

describe("ConversationTimeline reply folding", () => {
	afterEach(() => {
		Reflect.deleteProperty(HTMLElement.prototype, "animate");
		vi.useRealTimers();
	});

	it("slides the label in, pauses, then folds the reply under it", () => {
		vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
		const animations = trackAnimations();
		const prompt = MockWorkingMessages.slice(0, 1);
		const { rerenderStage } = renderTimeline(
			liveStage(prompt, [mockReplyPart]),
		);
		expect(animations).toHaveLength(0);

		rerenderStage(liveStage(prompt, [mockReplyPart, mockFirstToolCallPart]));
		expect(animations).toHaveLength(1);
		const [label] = animations;
		expect(entersLabel(label)).toBe(true);
		expect(label.options.duration).toBe(300);
		expect(label.element.textContent).not.toContain(mockReplyText);
		expect(screen.getByRole("status")).toHaveTextContent("");

		label.finish();
		expect(screen.getByRole("status")).toHaveTextContent("Working");
		act(() => vi.advanceTimersByTime(59));
		expect(animations).toHaveLength(1);

		act(() => vi.advanceTimersByTime(1));
		const [, collapse, fade] = animations;
		expect(collapse.element.textContent).toContain(mockReplyText);
		expect(collapse.keyframes.at(-1)?.height).toBe("0px");
		expect(fade.keyframes.at(-1)?.opacity).toBe(0);
		expect(fade.options.duration).toBeLessThan(
			Number(collapse.options.duration),
		);
	});

	it("folds into a label that is already showing", () => {
		const animations = trackAnimations();
		const firstStep = MockWorkingMessages.slice(0, 3);
		const { rerenderStage } = renderTimeline(
			liveStage(firstStep, [mockReplyPart]),
		);

		rerenderStage(
			liveStage(firstStep, [mockReplyPart, mockSecondToolCallPart]),
		);
		expect(animations.some(entersLabel)).toBe(false);
		expect(animations[0].element.textContent).toContain(mockReplyText);
	});

	it("folds reply text when a web search follows it", () => {
		const animations = trackAnimations();
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
		expect(animations.some(entersLabel)).toBe(false);
		expect(animations[0]?.element.textContent).toContain(mockReplyText);
	});

	it("enters the label without a fold when a tool call starts first", () => {
		const animations = trackAnimations();
		const prompt = MockWorkingMessages.slice(0, 1);
		const { rerenderStage } = renderTimeline({
			messages: prompt,
			chatStatus: "running",
			liveStatus: { phase: "starting", hasAccumulatedOutput: false },
		});

		rerenderStage(liveStage(prompt, [mockFirstToolCallPart]));
		expect(animations).toHaveLength(1);
		expect(entersLabel(animations[0])).toBe(true);

		animations[0].finish();
		expect(animations).toHaveLength(1);
	});

	it("queues a second fold until the first finishes", () => {
		vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
		const animations = trackAnimations();
		const prompt = MockWorkingMessages.slice(0, 1);
		const { rerenderStage } = renderTimeline(
			liveStage(prompt, [mockReplyPart]),
		);
		rerenderStage(liveStage(prompt, [mockReplyPart, mockFirstToolCallPart]));
		animations[0].finish();
		act(() => vi.advanceTimersByTime(60));
		expect(animations).toHaveLength(3);

		const nextReply = "Now I'll read the configuration file.";
		const firstStep = MockWorkingMessages.slice(0, 3);
		rerenderStage(liveStage(firstStep, [{ type: "text", text: nextReply }]));
		rerenderStage(
			liveStage(firstStep, [
				{ type: "text", text: nextReply },
				mockSecondToolCallPart,
			]),
		);
		expect(animations).toHaveLength(3);

		animations[1].finish();
		animations[2].finish();
		expect(animations).toHaveLength(5);
		expect(animations[3].element.textContent).toContain(nextReply);
	});

	it("crossfades without moving anything under reduced motion", () => {
		vi.spyOn(window, "matchMedia").mockImplementation((query) => ({
			matches: query === "(prefers-reduced-motion: reduce)",
			media: query,
			onchange: null,
			addListener: vi.fn(),
			removeListener: vi.fn(),
			addEventListener: vi.fn(),
			removeEventListener: vi.fn(),
			dispatchEvent: vi.fn(),
		}));
		const animations = trackAnimations();
		const prompt = MockWorkingMessages.slice(0, 1);
		const { rerenderStage } = renderTimeline(
			liveStage(prompt, [mockReplyPart]),
		);

		rerenderStage(liveStage(prompt, [mockReplyPart, mockFirstToolCallPart]));
		expect(animations).toHaveLength(2);
		for (const animation of animations) {
			expect(animation.options.duration).toBe(150);
			for (const keyframe of animation.keyframes) {
				expect(keyframe.height).toBeUndefined();
			}
		}
		expect(animations[1].element.textContent).toContain(mockReplyText);
	});
});
