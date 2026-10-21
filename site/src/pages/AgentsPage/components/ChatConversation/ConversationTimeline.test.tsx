import { fireEvent, screen, within } from "@testing-library/react";
import userEvent, { type UserEvent } from "@testing-library/user-event";
import { QueryClientProvider } from "react-query";
import { defaultUrlTransform } from "streamdown";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { preferenceSettingsKey } from "#/api/queries/users";
import type { ChatMessage, ChatMessagePart } from "#/api/typesGenerated";
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
	attachmentStepMessages,
	buildLiveStatus,
	buildStreamRenderState,
	MockCollapsedStepsPreferences,
	MockLongTurnPageLoads,
	MockTextBeforeSecondStepMessage,
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

/** Text between MockWorkingMessages' two steps, in a row of its own. */
const mockNote: ChatMessage = {
	...MockChatMessage,
	id: 4,
	role: "assistant",
	created_at: workingFixtureTime(5),
	content: [{ type: "text", text: "Found the config." }],
};

// MockWorkingMessages' second step, its result, and the answer, with IDs above
// mockNote's, as rows persisted after it.
const mockLiveStep: ChatMessage = { ...MockWorkingMessages[3], id: 5 };
const mockLiveStepResult: ChatMessage = { ...MockWorkingMessages[4], id: 6 };
const mockAnswerAfterNote: ChatMessage = { ...MockWorkingMessages[5], id: 7 };

/** MockWorkingMessages' second step, called after text with a link. */
const mockLinkBeforeSecondStep: ChatMessage = {
	...MockWorkingMessages[3],
	content: [
		{ type: "text", text: "See [the trial plan](https://docs.example.com)." },
		...(MockWorkingMessages[3].content ?? []),
	],
};

const focusCopyCommand = (messageId: number) => {
	const button = within(
		screen.getByTestId(`chat-message-message:${messageId}`),
	).getByRole("button", { name: "Copy command" });
	button.focus();
	return button;
};

describe("ConversationTimeline working blocks", () => {
	it("copies the whole message from the answer that ends a row folded by its work", async () => {
		const user = userEvent.setup();
		const writeText = vi
			.spyOn(navigator.clipboard, "writeText")
			.mockResolvedValue();
		renderTimeline({ messages: MockWebSearchAnswerMessages });

		// The row's lead-in text folds into its search's block, and only the
		// answer below the block carries the row's actions.
		await user.click(
			within(screen.getByTestId("chat-message-message:6:1")).getByRole(
				"button",
				{ name: "Copy message" },
			),
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

	const textSplitTurn = [
		...MockWorkingMessages.slice(0, 3),
		MockTextBeforeSecondStepMessage,
	];

	it.each([
		[
			"the rest of the turn persists",
			MockWorkingMessages.slice(0, 4),
			MockWorkingMessages,
		],
		[
			"no further messages arrive",
			MockWorkingMessages.slice(0, 4),
			MockWorkingMessages.slice(0, 4),
		],
		[
			"text split the turn's blocks",
			textSplitTurn,
			[...textSplitTurn, ...MockWorkingMessages.slice(4)],
		],
	])(
		"collapses the open block into its turn's one block, keeping its summary focused, at the turn's end when %s",
		async (_, messages, finalMessages) => {
			const user = userEvent.setup();
			const { rerenderStage } = renderTimeline({
				messages,
				pendingToolCallIDs: getPendingToolCallIDs(messages, "running"),
				chatStatus: "running",
				liveStatus: idleLive,
			});

			const summary = screen.getByRole("button", { name: /^Working for/ });
			await user.click(summary);

			rerenderStage({
				messages: finalMessages,
				chatStatus: "waiting",
				liveStatus: idleLive,
			});
			expect(screen.getByRole("button", { name: /^Worked for/ })).toBe(summary);
			expect(summary).toHaveFocus();
			expect(summary).toHaveAttribute("aria-expanded", "false");
		},
	);

	const linkSplitTurn = [
		...MockWorkingMessages.slice(0, 3),
		mockLinkBeforeSecondStep,
	];
	const runningStage = (messages: ChatMessage[]): TimelineStage => ({
		messages,
		pendingToolCallIDs: getPendingToolCallIDs(messages, "running"),
		chatStatus: "running",
		liveStatus: idleLive,
	});

	it.each<
		[string, TimelineStage, (user: UserEvent) => Promise<void>, ChatMessage[]]
	>([
		[
			"a control in an earlier block",
			runningStage(textSplitTurn),
			async (user) => {
				await user.click(screen.getByRole("button", { name: /^Worked for/ }));
				focusCopyCommand(2);
			},
			[...textSplitTurn, ...MockWorkingMessages.slice(4)],
		],
		[
			"a link in text between steps",
			runningStage(linkSplitTurn),
			async () => screen.getByRole("link", { name: "the trial plan" }).focus(),
			[...linkSplitTurn, ...MockWorkingMessages.slice(4)],
		],
		[
			"a control in the turn's last block",
			runningStage(MockWorkingMessages.slice(0, 4)),
			async (user) => {
				await user.click(screen.getByRole("button", { name: /^Working for/ }));
				focusCopyCommand(4);
			},
			MockWorkingMessages,
		],
		[
			"the summary of a block only the stream had",
			{
				messages: [...MockWorkingMessages.slice(0, 3), mockNote],
				chatStatus: "running",
				...buildStreamRenderState(MockWorkingMessages[3].content ?? []),
			},
			async () => screen.getByRole("button", { name: /^Working for/ }).focus(),
			[
				...MockWorkingMessages.slice(0, 3),
				mockNote,
				mockLiveStep,
				mockLiveStepResult,
				mockAnswerAfterNote,
			],
		],
	])(
		"moves focus from %s to its turn's fold at the turn's end",
		async (_, running, focus, finalMessages) => {
			const user = userEvent.setup();
			const { rerenderStage } = renderTimeline(running);
			await focus(user);

			rerenderStage({
				messages: finalMessages,
				chatStatus: "waiting",
				liveStatus: idleLive,
			});
			expect(screen.getByRole("button", { name: /^Worked for/ })).toHaveFocus();
		},
	);

	it("keeps an image the reader opened in a block open as its turn folds", async () => {
		const user = userEvent.setup();
		const messages = [
			MockWorkingMessages[0],
			...attachmentStepMessages({
				id: 2,
				toolCallId: "attach",
				callAt: 1,
				toolName: "attach_file",
				args: { path: "/home/coder/chart.png" },
				file: {
					type: "file",
					media_type: "image/png",
					name: "chart.png",
					file_id: "chart-file",
				},
				result: { name: "chart.png" },
			}),
		];
		const { rerenderStage } = renderTimeline(runningStage(messages));

		await user.click(screen.getByRole("button", { name: /^Working for/ }));
		await user.click(screen.getByRole("button", { name: "View chart.png" }));

		rerenderStage({
			messages: [...messages, { ...MockWorkingMessages[5], id: 5 }],
			chatStatus: "waiting",
			liveStatus: idleLive,
		});
		expect(
			screen.getByRole("button", { name: /^Worked for/, hidden: true }),
		).toHaveAttribute("aria-expanded", "false");
		expect(
			screen.getByRole("dialog", { name: "Image preview" }),
		).toBeInTheDocument();

		await user.keyboard("{Escape}");
		expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
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

	const mockSecondStepParts = MockWorkingMessages[3].content ?? [];

	it.each<[string, readonly ChatMessagePart[]]>([
		["", mockSecondStepParts],
		[
			" that started after text",
			[
				{
					type: "reasoning",
					text: "Planning the inspection",
					created_at: workingFixtureTime(2),
					completed_at: workingFixtureTime(3),
				},
				{ type: "text", text: "Checking the second step." },
				...mockSecondStepParts,
			],
		],
	])(
		"keeps an open live-only block%s mounted when its prompt page and persisted step arrive together",
		async (_name, content) => {
			const user = userEvent.setup();
			const mockStep: ChatMessage = {
				...MockWorkingMessages[3],
				id: 3,
				content,
			};
			const { rerenderStage } = renderTimeline({
				messages: [mockAssistantNote],
				hasMoreMessages: true,
				chatStatus: "running",
				...buildStreamRenderState(content),
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
		},
	);

	it("keeps an open live block mounted when an older page reveals an earlier block of its turn", async () => {
		const user = userEvent.setup();
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
	it("keeps streamed text mounted between the block it closes and the next step's block", () => {
		const messages = MockWorkingMessages.slice(0, 3);
		const note: ChatMessagePart = { type: "text", text: "Trials are running." };
		const { rerenderStage } = renderTimeline({
			messages,
			chatStatus: "running",
			...buildStreamRenderState([note]),
		});

		const summary = screen.getByRole("button", { name: "Working for 12s" });
		summary.focus();
		const liveAssistant = screen.getByTestId("chat-message-live-assistant");

		rerenderStage({
			messages,
			chatStatus: "running",
			...buildStreamRenderState([
				note,
				...(MockWorkingMessages[3].content ?? []),
			]),
		});

		expect(summary).toHaveFocus();
		expect(screen.getByTestId("chat-message-live-assistant")).toBe(
			liveAssistant,
		);
	});

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
			chatStatus: "running",
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

	it.each([
		["focused", (summary: HTMLElement) => summary.focus()],
		// fireEvent.click leaves focus alone, as a Safari click does, so this
		// case tests the toggle alone.
		["expanded", (summary: HTMLElement) => fireEvent.click(summary)],
	])(
		"keeps a block of only reasoning that the reader %s when text follows it",
		(_, engage) => {
			const messages = MockWorkingMessages.slice(0, 5);
			const note: ChatMessagePart = { type: "text", text: "Both passed." };
			const reasoning: ChatMessagePart = {
				type: "reasoning",
				text: "Planning the summary",
				created_at: workingFixtureTime(13),
			};
			const answer: ChatMessagePart = { type: "text", text: "All good." };
			const { rerenderStage } = renderTimeline({
				messages,
				chatStatus: "running",
				...buildStreamRenderState([note, reasoning]),
			});
			const summary = screen.getByRole("button", { name: /^Working for/ });
			engage(summary);

			rerenderStage({
				messages,
				chatStatus: "running",
				...buildStreamRenderState([note, reasoning, answer]),
			});
			expect(screen.getByRole("button", { name: /^Working for/ })).toBe(
				summary,
			);
		},
	);

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

		const summary = screen.getByRole("button", {
			name: "Working for at least 12s",
		});
		await user.click(summary);

		rerenderStage({
			messages: MockWorkingMessages.slice(1),
			hasMoreMessages: true,
			chatStatus: "waiting",
			liveStatus: idleLive,
		});
		expect(summary).toHaveFocus();

		rerenderStage({
			messages: MockWorkingMessages,
			chatStatus: "waiting",
			liveStatus: idleLive,
		});
		expect(summary).toHaveFocus();
	});
});
