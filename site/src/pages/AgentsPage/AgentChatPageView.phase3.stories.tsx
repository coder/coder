import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import {
	type ComponentProps,
	type FC,
	useEffect,
	useRef,
	useState,
} from "react";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import type * as TypesGen from "#/api/typesGenerated";
import { MockUserOwner } from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
	withProxyProvider,
} from "#/testHelpers/storybook";
import { AgentChatPageView } from "./AgentChatPageView";
import { ChatSessionsProvider } from "./chatSession/ChatSessionsProvider";
import { useChatSession, useChatSessionSelector } from "./chatSession/hooks";
import type { ChatSessionSnapshot } from "./chatSession/types";
import type { ChatStoreState } from "./components/ChatConversation/chatStore";
import { useChatSelector } from "./components/ChatConversation/chatStore";
import type { ModelSelectorOption } from "./components/ChatElements";
import type { ChatDetailError } from "./utils/usageLimitMessage";

const PHASE3_CHAT_A_ID = "phase3-chat-a";
const PHASE3_CHAT_B_ID = "phase3-chat-b";
const CHAT_BOTTOM_THRESHOLD_PX = 70;
const PHASE3_BASE_TIME = Date.UTC(2026, 0, 1, 0, 0, 0);
const defaultModelConfigID = "phase3-model-config";

const defaultModelOptions: ModelSelectorOption[] = [
	{
		id: defaultModelConfigID,
		provider: "openai",
		model: "gpt-4o",
		displayName: "GPT-4o",
	},
];

const selectStoryFollowMode = (snapshot: ChatSessionSnapshot) =>
	snapshot.followMode;

const selectStoryHasNewOffscreenContent = (snapshot: ChatSessionSnapshot) =>
	snapshot.hasNewOffscreenContent;

const selectStoryMessageCount = (state: ChatStoreState) =>
	state.messagesByID.size;

type AgentChatPageViewProps = ComponentProps<typeof AgentChatPageView>;
type MockWebSocketEventType = "message" | "error" | "open" | "close";
type MockWebSocketEvent = MessageEvent<string> | Event | CloseEvent;
type MockWebSocketListener = EventListenerOrEventListenerObject;

type MockSocket = WebSocket & {
	emitData: (event: TypesGen.ChatStreamEvent) => void;
};

const mockSocketsByChatId = new Map<string, MockSocket>();

const isMockWebSocketEventType = (
	eventType: string,
): eventType is MockWebSocketEventType =>
	eventType === "message" ||
	eventType === "error" ||
	eventType === "open" ||
	eventType === "close";

const dispatchMockWebSocketEvent = (
	listener: MockWebSocketListener,
	event: MockWebSocketEvent,
): void => {
	if (typeof listener === "function") {
		listener(event);
		return;
	}
	listener.handleEvent(event);
};

const getChatIdFromWebSocketURL = (url: string): string | undefined => {
	const match = url.match(/\/api\/experimental\/chats\/([^/?]+)\/stream/);
	return match?.[1] ? decodeURIComponent(match[1]) : undefined;
};

const installMockWebSocket = (): (() => void) => {
	const originalWebSocket = window.WebSocket;

	class Phase3WebSocket {
		public static readonly CONNECTING = 0;
		public static readonly OPEN = 1;
		public static readonly CLOSING = 2;
		public static readonly CLOSED = 3;

		public binaryType: BinaryType = "blob";
		public readonly bufferedAmount = 0;
		public readonly extensions = "";
		public readonly protocol = "";
		public readyState = Phase3WebSocket.OPEN;
		public readonly url: string;

		private readonly listeners: Record<
			MockWebSocketEventType,
			Set<MockWebSocketListener>
		> = {
			message: new Set(),
			error: new Set(),
			open: new Set(),
			close: new Set(),
		};

		public constructor(url?: string | URL) {
			this.url = String(url ?? "");
			const chatId = getChatIdFromWebSocketURL(this.url);
			if (chatId) {
				mockSocketsByChatId.set(chatId, this as unknown as MockSocket);
			}
		}

		public addEventListener(
			eventType: string,
			listener: MockWebSocketListener | null,
		): void {
			if (!listener || !isMockWebSocketEventType(eventType)) {
				return;
			}
			this.listeners[eventType].add(listener);
		}

		public removeEventListener(
			eventType: string,
			listener: MockWebSocketListener | null,
		): void {
			if (!listener || !isMockWebSocketEventType(eventType)) {
				return;
			}
			this.listeners[eventType].delete(listener);
		}

		public dispatchEvent(event: Event): boolean {
			if (!isMockWebSocketEventType(event.type)) {
				return true;
			}
			this.emit(event.type, event);
			return true;
		}

		public send(): void {}

		public close(): void {
			if (this.readyState === Phase3WebSocket.CLOSED) {
				return;
			}
			this.readyState = Phase3WebSocket.CLOSED;
			this.emit("close", new CloseEvent("close"));
		}

		public emitData(event: TypesGen.ChatStreamEvent): void {
			this.emit(
				"message",
				new MessageEvent<string>("message", {
					data: JSON.stringify([event]),
				}),
			);
		}

		private emit(
			eventType: MockWebSocketEventType,
			event: MockWebSocketEvent,
		): void {
			for (const listener of this.listeners[eventType]) {
				dispatchMockWebSocketEvent(listener, event);
			}
		}
	}

	window.WebSocket = Phase3WebSocket as unknown as typeof window.WebSocket;
	return () => {
		window.WebSocket = originalWebSocket;
	};
};

const waitForSocket = async (chatId: string): Promise<MockSocket> => {
	let socket: MockSocket | undefined;
	await waitFor(() => {
		socket = mockSocketsByChatId.get(chatId);
		expect(socket).toBeDefined();
	});
	if (!socket) {
		throw new Error(`Expected a mock socket for ${chatId}.`);
	}
	return socket;
};

const makeChat = (
	chatId: string,
	status: TypesGen.ChatStatus = "running",
): TypesGen.Chat => ({
	id: chatId,
	organization_id: "test-org-id",
	owner_id: "owner-1",
	title: `Phase 3 ${chatId}`,
	status,
	last_model_config_id: defaultModelConfigID,
	mcp_server_ids: [],
	labels: {},
	created_at: new Date(PHASE3_BASE_TIME).toISOString(),
	updated_at: new Date(PHASE3_BASE_TIME).toISOString(),
	archived: false,
	pin_order: 0,
	has_unread: false,
	client_type: "ui",
	last_error: null,
	children: [],
});

const makeMessage = ({
	chatId,
	id,
	role,
	text,
}: {
	chatId: string;
	id: number;
	role: TypesGen.ChatMessageRole;
	text: string;
}): TypesGen.ChatMessage => ({
	id,
	chat_id: chatId,
	created_at: new Date(PHASE3_BASE_TIME + id * 60_000).toISOString(),
	role,
	content: [{ type: "text", text }],
});

const makeHistoryMessages = (chatId: string): TypesGen.ChatMessage[] => {
	const messages: TypesGen.ChatMessage[] = [];
	for (let id = 1; id <= 30; id += 1) {
		const role: TypesGen.ChatMessageRole = id % 2 === 1 ? "user" : "assistant";
		const turn = Math.ceil(id / 2);
		messages.push(
			makeMessage({
				chatId,
				id,
				role,
				text:
					role === "user"
						? `Phase 3 ${chatId} question ${turn}. Please explain the scrolling behavior in detail.`
						: `Phase 3 ${chatId} answer ${turn}. `.repeat(8),
			}),
		);
	}
	return messages;
};

const makeMessagesData = (
	messages: readonly TypesGen.ChatMessage[],
): TypesGen.ChatMessagesResponse => ({
	messages,
	queued_messages: [],
	has_more: false,
});

const appendDurableMessage = (
	chatId: string,
	message: TypesGen.ChatMessage,
): void => {
	const socket = mockSocketsByChatId.get(chatId);
	if (!socket) {
		throw new Error(`Cannot append to ${chatId} before its socket is open.`);
	}
	socket.emitData({
		type: "message",
		chat_id: chatId,
		message,
	});
};

const appendStreamPart = (
	chatId: string,
	part: TypesGen.ChatMessagePart,
): void => {
	const socket = mockSocketsByChatId.get(chatId);
	if (!socket) {
		throw new Error(`Cannot stream to ${chatId} before its socket is open.`);
	}
	socket.emitData({
		type: "message_part",
		chat_id: chatId,
		message_part: { part },
	});
};

const buildEditing = (
	overrides: Partial<AgentChatPageViewProps["editing"]> = {},
): AgentChatPageViewProps["editing"] => ({
	chatInputRef: { current: null },
	editorInitialValue: "",
	initialEditorState: undefined,
	remountKey: 0,
	editingMessageId: null,
	editingFileBlocks: [],
	handleEditUserMessage: fn(),
	handleCancelHistoryEdit: fn(),
	editingQueuedMessageID: null,
	handleStartQueueEdit: fn(),
	handleCancelQueueEdit: fn(),
	handleSendFromInput: fn(),
	handleContentChange: fn(),
	...overrides,
});

const buildGitWatcher = (): AgentChatPageViewProps["gitWatcher"] => ({
	repositories: new Map(),
	everDirty: new Set(),
	refresh: fn().mockReturnValue(true),
});

const Phase3SessionSentinels: FC<{ activeChatId: string }> = ({
	activeChatId,
}) => {
	const followMode = useChatSessionSelector(
		activeChatId,
		selectStoryFollowMode,
	);
	const hasNewOffscreenContent = useChatSessionSelector(
		activeChatId,
		selectStoryHasNewOffscreenContent,
	);

	return (
		<div aria-hidden className="sr-only">
			<span data-testid="active-chat-id">{activeChatId}</span>
			<span data-testid="follow-mode">{String(followMode)}</span>
			<span data-testid="has-new">{String(hasNewOffscreenContent)}</span>
		</div>
	);
};

const Phase3MultiChatHarness: FC = () => {
	const [activeChatId, setActiveChatId] = useState(PHASE3_CHAT_A_ID);
	const chatASession = useChatSession(PHASE3_CHAT_A_ID);
	const chatBSession = useChatSession(PHASE3_CHAT_B_ID);
	const didHydrateRef = useRef(false);
	const scrollContainerRef = useRef<HTMLDivElement | null>(null);
	const scrollToBottomRef = useRef<(() => void) | null>(null);
	const activeSession =
		activeChatId === PHASE3_CHAT_A_ID ? chatASession : chatBSession;
	const messageCount = useChatSelector(
		activeSession.store,
		selectStoryMessageCount,
	);

	useEffect(() => {
		if (didHydrateRef.current) {
			return;
		}
		didHydrateRef.current = true;
		const chatAMessages = makeHistoryMessages(PHASE3_CHAT_A_ID);
		const chatBMessages = makeHistoryMessages(PHASE3_CHAT_B_ID);
		chatASession.hydrateFromRest({
			chatMessages: chatAMessages,
			chatRecord: makeChat(PHASE3_CHAT_A_ID),
			chatMessagesData: makeMessagesData(chatAMessages),
			chatQueuedMessages: [],
		});
		chatBSession.hydrateFromRest({
			chatMessages: chatBMessages,
			chatRecord: makeChat(PHASE3_CHAT_B_ID),
			chatMessagesData: makeMessagesData(chatBMessages),
			chatQueuedMessages: [],
		});
		chatASession.enterForeground({ now: 1 });
		chatBSession.enterForeground({ now: 2 });
	}, [chatASession, chatBSession]);

	const appendDurableToActive = () => {
		appendDurableMessage(
			activeChatId,
			makeMessage({
				chatId: activeChatId,
				id: activeChatId === PHASE3_CHAT_A_ID ? 150 : 250,
				role: "assistant",
				text: `Control durable message for ${activeChatId}.`,
			}),
		);
	};

	const appendStreamPartToActive = () => {
		appendStreamPart(activeChatId, {
			type: "text",
			text: `Control streamed text for ${activeChatId}.`,
		});
	};

	return (
		<div className="relative flex h-[620px] flex-col">
			<div className="absolute top-2 left-2 z-50 flex gap-2 rounded-md border border-border-default bg-surface-primary p-2 shadow-md">
				<button type="button" onClick={() => setActiveChatId(PHASE3_CHAT_A_ID)}>
					Show chat A
				</button>
				<button type="button" onClick={() => setActiveChatId(PHASE3_CHAT_B_ID)}>
					Show chat B
				</button>
				<button
					type="button"
					onClick={() =>
						appendDurableMessage(
							PHASE3_CHAT_A_ID,
							makeMessage({
								chatId: PHASE3_CHAT_A_ID,
								id: 151,
								role: "assistant",
								text: "Control durable message for chat A.",
							}),
						)
					}
				>
					Append durable to chat A
				</button>
				<button
					type="button"
					onClick={() =>
						appendStreamPart(PHASE3_CHAT_A_ID, {
							type: "text",
							text: "Control stream part for chat A.",
						})
					}
				>
					Append stream part to chat A
				</button>
				<button type="button" onClick={appendDurableToActive}>
					Append durable to active chat
				</button>
				<button type="button" onClick={appendStreamPartToActive}>
					Append stream part to active chat
				</button>
			</div>
			<Phase3SessionSentinels activeChatId={activeChatId} />
			<AgentChatPageView
				agentId={activeChatId}
				organizationId="test-org-id"
				chatTitle={`Phase 3 ${activeChatId}`}
				persistedError={undefined as ChatDetailError | undefined}
				parentChat={undefined}
				isArchived={false}
				store={activeSession.store}
				editing={buildEditing()}
				effectiveSelectedModel={defaultModelConfigID}
				setSelectedModel={fn()}
				modelOptions={defaultModelOptions}
				modelSelectorPlaceholder="Select a model"
				hasModelOptions
				compressionThreshold={undefined}
				isInputDisabled={false}
				isSubmissionPending={false}
				isInterruptPending={false}
				isSidebarCollapsed={false}
				onToggleSidebarCollapsed={fn()}
				showSidebarPanel={false}
				onSetShowSidebarPanel={fn()}
				prNumber={undefined}
				diffStatusData={undefined}
				debugLoggingEnabled={false}
				gitWatcher={buildGitWatcher()}
				sshCommand={undefined}
				handleCommit={fn()}
				handleInterrupt={fn()}
				handleDeleteQueuedMessage={fn()}
				handlePromoteQueuedMessage={fn()}
				handleArchiveAgentAction={fn()}
				handleUnarchiveAgentAction={fn()}
				handleArchiveAndDeleteWorkspaceAction={fn()}
				handleRegenerateTitle={fn()}
				scrollContainerRef={scrollContainerRef}
				scrollToBottomRef={scrollToBottomRef}
				hasMoreMessages={false}
				isFetchingMoreMessages={false}
				onFetchMoreMessages={fn()}
				messageCount={messageCount}
				mcpServers={[]}
				selectedMCPServerIds={[]}
				onMCPSelectionChange={fn()}
				onMCPAuthComplete={fn()}
			/>
		</div>
	);
};

const withChatSessionsProvider: Decorator = (Story, context) => (
	<ChatSessionsProvider
		key={context.id}
		setChatErrorReason={fn()}
		clearChatErrorReason={fn()}
	>
		<Story />
	</ChatSessionsProvider>
);

const agentsRouting = [
	{ path: "/agents/:agentId", useStoryElement: true },
	{ path: "/agents", useStoryElement: true },
] satisfies [
	{ path: string; useStoryElement: boolean },
	...{ path: string; useStoryElement: boolean }[],
];

const meta: Meta<typeof AgentChatPageView> = {
	title: "pages/AgentsPage/AgentChatPageView/Phase3",
	component: AgentChatPageView,
	decorators: [
		withAuthProvider,
		withDashboardProvider,
		withProxyProvider(),
		withChatSessionsProvider,
	],
	parameters: {
		layout: "fullscreen",
		user: MockUserOwner,
		reactRouter: reactRouterParameters({
			location: {
				path: `/agents/${PHASE3_CHAT_A_ID}`,
				pathParams: { agentId: PHASE3_CHAT_A_ID },
			},
			routing: agentsRouting,
		}),
	},
	beforeEach: () => {
		mockSocketsByChatId.clear();
		const restoreWebSocket = installMockWebSocket();
		return () => {
			restoreWebSocket();
			mockSocketsByChatId.clear();
		};
	},
};

export default meta;
type Story = StoryObj<typeof AgentChatPageView>;

const waitForResizeObserverTick = async () => {
	await new Promise<void>((resolve) => {
		requestAnimationFrame(() => {
			requestAnimationFrame(() => {
				resolve();
			});
		});
	});
};

const waitForScrollOverflow = async (scroller: HTMLElement) => {
	await waitFor(() => {
		expect(scroller.scrollHeight).toBeGreaterThan(scroller.clientHeight);
	});
};

const getAnchorByMessageId = (
	scroller: HTMLElement,
	messageId: number,
): HTMLElement | null =>
	scroller.querySelector<HTMLElement>(
		`[data-chat-message-anchor][data-chat-message-id="${messageId}"]`,
	);

const getTopVisibleAnchor = (scroller: HTMLElement): HTMLElement | null => {
	const scrollerRect = scroller.getBoundingClientRect();
	const anchors = scroller.querySelectorAll<HTMLElement>(
		"[data-chat-message-anchor][data-chat-message-id]",
	);
	for (const anchor of anchors) {
		const rect = anchor.getBoundingClientRect();
		const topInsideViewport =
			rect.top >= scrollerRect.top && rect.top < scrollerRect.bottom;
		const crossesViewportTop =
			rect.top < scrollerRect.top && rect.bottom > scrollerRect.top;
		if (topInsideViewport || crossesViewportTop) {
			return anchor;
		}
	}
	return null;
};

const getMessageIdFromAnchor = (anchor: HTMLElement): number => {
	const messageId = Number(anchor.dataset.chatMessageId);
	if (!Number.isFinite(messageId)) {
		throw new Error("Expected the top-visible anchor to have a message ID.");
	}
	return messageId;
};

const scrollAwayUntilAnchorCaptured = async (
	scroller: HTMLElement,
): Promise<HTMLElement> => {
	const maxDistance = Math.max(
		1,
		scroller.scrollHeight - scroller.clientHeight,
	);
	const distances = [
		Math.min(maxDistance, 240),
		Math.min(maxDistance, 480),
		Math.min(maxDistance, 720),
		Math.min(maxDistance, 960),
		maxDistance,
	];

	for (const distance of distances) {
		scroller.scrollTop = -distance;
		scroller.dispatchEvent(new Event("scroll"));
		await waitForResizeObserverTick();
		const anchor = getTopVisibleAnchor(scroller);
		if (anchor) {
			return anchor;
		}
	}

	throw new Error("Expected to capture a top-visible message anchor.");
};

const expectElementInScrollerViewport = (
	scroller: HTMLElement,
	element: HTMLElement,
) => {
	const scrollerRect = scroller.getBoundingClientRect();
	const elementRect = element.getBoundingClientRect();
	expect(element.getClientRects().length).toBeGreaterThan(0);
	expect(elementRect.bottom).toBeGreaterThan(scrollerRect.top);
	expect(elementRect.top).toBeLessThan(scrollerRect.bottom);
};

const waitForFollowMode = async (
	canvas: ReturnType<typeof within>,
	expected: boolean,
) => {
	await waitFor(() => {
		expect(canvas.getByTestId("follow-mode")).toHaveTextContent(
			String(expected),
		);
	});
};

const waitForHasNew = async (
	canvas: ReturnType<typeof within>,
	expected: boolean,
) => {
	await waitFor(() => {
		expect(canvas.getByTestId("has-new")).toHaveTextContent(String(expected));
	});
};

const waitForActiveChat = async (
	canvas: ReturnType<typeof within>,
	chatId: string,
) => {
	await waitFor(() => {
		expect(canvas.getByTestId("active-chat-id")).toHaveTextContent(chatId);
	});
};

export const BottomFollowingReturnPinsLatestMessages: Story = {
	parameters: { chromatic: { disableSnapshot: true } },
	render: () => <Phase3MultiChatHarness />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitForSocket(PHASE3_CHAT_A_ID);
		await waitForSocket(PHASE3_CHAT_B_ID);
		let scroller = canvas.getByTestId("scroll-container");
		await waitForScrollOverflow(scroller);
		await waitFor(() => {
			expect(Math.abs(scroller.scrollTop)).toBeLessThanOrEqual(
				CHAT_BOTTOM_THRESHOLD_PX,
			);
		});

		await userEvent.click(canvas.getByRole("button", { name: "Show chat B" }));
		await waitForActiveChat(canvas, PHASE3_CHAT_B_ID);
		appendDurableMessage(
			PHASE3_CHAT_A_ID,
			makeMessage({
				chatId: PHASE3_CHAT_A_ID,
				id: 101,
				role: "assistant",
				text: "Latest durable message while chat A was followed.",
			}),
		);
		appendDurableMessage(
			PHASE3_CHAT_A_ID,
			makeMessage({
				chatId: PHASE3_CHAT_A_ID,
				id: 102,
				role: "user",
				text: "Second durable message while chat A was followed.",
			}),
		);
		appendDurableMessage(
			PHASE3_CHAT_A_ID,
			makeMessage({
				chatId: PHASE3_CHAT_A_ID,
				id: 103,
				role: "assistant",
				text: "Final durable message while chat A was followed.",
			}),
		);
		appendStreamPart(PHASE3_CHAT_A_ID, {
			type: "text",
			text: "Followed offscreen stream part.",
		});

		await userEvent.click(canvas.getByRole("button", { name: "Show chat A" }));
		await waitForActiveChat(canvas, PHASE3_CHAT_A_ID);
		await waitForResizeObserverTick();
		scroller = canvas.getByTestId("scroll-container");
		const latestMessage = await canvas.findByText(
			"Final durable message while chat A was followed.",
		);
		await waitFor(() => {
			expect(Math.abs(scroller.scrollTop)).toBeLessThanOrEqual(
				CHAT_BOTTOM_THRESHOLD_PX,
			);
			expectElementInScrollerViewport(scroller, latestMessage);
			expect(canvas.queryByRole("button", { name: "New messages" })).toBeNull();
		});
	},
};

export const AnchoredReturnPreservesTopVisibleMessage: Story = {
	parameters: { chromatic: { disableSnapshot: true } },
	render: () => <Phase3MultiChatHarness />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitForSocket(PHASE3_CHAT_A_ID);
		await waitForSocket(PHASE3_CHAT_B_ID);
		let scroller = canvas.getByTestId("scroll-container");
		await waitForScrollOverflow(scroller);
		const anchor = await scrollAwayUntilAnchorCaptured(scroller);
		const messageId = getMessageIdFromAnchor(anchor);
		expect(getAnchorByMessageId(scroller, messageId)).not.toBeNull();
		const savedOffsetTop =
			anchor.getBoundingClientRect().top - scroller.getBoundingClientRect().top;
		await waitForFollowMode(canvas, false);

		await userEvent.click(canvas.getByRole("button", { name: "Show chat B" }));
		await waitForActiveChat(canvas, PHASE3_CHAT_B_ID);
		appendDurableMessage(
			PHASE3_CHAT_A_ID,
			makeMessage({
				chatId: PHASE3_CHAT_A_ID,
				id: 104,
				role: "assistant",
				text: "Durable message below the preserved chat A anchor.",
			}),
		);

		await userEvent.click(canvas.getByRole("button", { name: "Show chat A" }));
		await waitForActiveChat(canvas, PHASE3_CHAT_A_ID);
		await waitForResizeObserverTick();
		scroller = canvas.getByTestId("scroll-container");

		await waitFor(() => {
			const restoredAnchor = getTopVisibleAnchor(scroller);
			expect(restoredAnchor).not.toBeNull();
			if (!restoredAnchor) {
				throw new Error("Expected a restored top-visible anchor.");
			}
			expect(getMessageIdFromAnchor(restoredAnchor)).toBe(messageId);
			const currentOffsetTop =
				restoredAnchor.getBoundingClientRect().top -
				scroller.getBoundingClientRect().top;
			expect(Math.abs(currentOffsetTop - savedOffsetTop)).toBeLessThanOrEqual(
				2,
			);
			expect(
				canvas.getByRole("button", { name: "New messages" }),
			).toBeVisible();
		});
	},
};

export const NewMessagesButtonHandlesDurableMessageBelowAnchor: Story = {
	parameters: { chromatic: { disableSnapshot: true } },
	render: () => <Phase3MultiChatHarness />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitForSocket(PHASE3_CHAT_A_ID);
		const scroller = canvas.getByTestId("scroll-container");
		await waitForScrollOverflow(scroller);
		await scrollAwayUntilAnchorCaptured(scroller);
		await waitForFollowMode(canvas, false);

		appendDurableMessage(
			PHASE3_CHAT_A_ID,
			makeMessage({
				chatId: PHASE3_CHAT_A_ID,
				id: 105,
				role: "assistant",
				text: "Durable message below the active anchor.",
			}),
		);
		await waitForHasNew(canvas, true);
		await userEvent.click(canvas.getByRole("button", { name: "New messages" }));

		await waitFor(() => {
			expect(Math.abs(scroller.scrollTop)).toBeLessThanOrEqual(1);
			expect(canvas.getByTestId("follow-mode")).toHaveTextContent("true");
			expect(canvas.queryByRole("button", { name: "New messages" })).toBeNull();
		});
	},
};

export const NewMessagesButtonHandlesFirstStreamPartAwayFromBottom: Story = {
	parameters: { chromatic: { disableSnapshot: true } },
	render: () => <Phase3MultiChatHarness />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitForSocket(PHASE3_CHAT_A_ID);
		const scroller = canvas.getByTestId("scroll-container");
		await waitForScrollOverflow(scroller);
		await scrollAwayUntilAnchorCaptured(scroller);
		await waitForFollowMode(canvas, false);

		appendStreamPart(PHASE3_CHAT_A_ID, {
			type: "text",
			text: "First stream part while away from bottom.",
		});
		await waitForHasNew(canvas, true);
		await userEvent.click(canvas.getByRole("button", { name: "New messages" }));

		await waitFor(() => {
			expect(Math.abs(scroller.scrollTop)).toBeLessThanOrEqual(1);
			expect(canvas.getByTestId("follow-mode")).toHaveTextContent("true");
			expect(canvas.queryByRole("button", { name: "New messages" })).toBeNull();
		});
	},
};
