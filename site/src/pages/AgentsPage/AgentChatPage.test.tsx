import { act, cleanup, render, waitFor } from "@testing-library/react";
import type { RefObject } from "react";
import {
	type InfiniteData,
	QueryClient,
	QueryClientProvider,
} from "react-query";
import { createMemoryRouter, Outlet, RouterProvider } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { chatKey, chatMessagesKey } from "#/api/queries/chats";
import { workspacesKey } from "#/api/queries/workspaces";
import type * as TypesGen from "#/api/typesGenerated";
import AgentChatPage from "./AgentChatPage";
import type { AgentsOutletContext } from "./AgentsPage";
import { ChatSessionManager } from "./chatSession/ChatSessionManager";
import { ChatSessionsProvider } from "./chatSession/ChatSessionsProvider";
import type { ChatStore } from "./components/ChatConversation/chatStore";

interface AgentChatPageViewMockProps {
	agentId: string;
	store: ChatStore;
}

type NoopSocket = {
	url: string;
	addEventListener: (
		event: string,
		listener: (...args: unknown[]) => void,
	) => void;
	removeEventListener: (
		event: string,
		listener: (...args: unknown[]) => void,
	) => void;
	close: () => void;
};

type LifecycleCall = {
	type: "mark" | "release";
	chatId: string;
	order: number;
};

const agentChatPageViewProps = vi.hoisted(
	() => [] as AgentChatPageViewMockProps[],
);
const apiMocks = vi.hoisted(() => ({
	watchChat: vi.fn(),
	watchWorkspace: vi.fn(),
}));

vi.mock("#/api/api", async (importOriginal) => {
	const actual = await importOriginal<typeof import("#/api/api")>();
	return {
		...actual,
		watchChat: apiMocks.watchChat,
		watchWorkspace: apiMocks.watchWorkspace,
	};
});

vi.mock("#/contexts/ProxyContext", () => ({
	useProxy: () => ({ proxy: { preferredWildcardHostname: "" } }),
}));

vi.mock("./hooks/useGitWatcher", () => ({
	useGitWatcher: () => ({
		repositories: new Map(),
		everDirty: new Set(),
		isConnected: false,
		refresh: () => false,
	}),
}));

vi.mock("./components/ChatConversation/useWorkspaceCreationWatcher", () => ({
	useWorkspaceCreationWatcher: () => {},
}));

vi.mock("./AgentChatPageView", () => ({
	AgentChatPageView: (props: AgentChatPageViewMockProps) => {
		agentChatPageViewProps.push(props);
		return null;
	},
	AgentChatPageLoadingView: () => null,
	AgentChatPageNotFoundView: () => null,
}));

const createNoopSocket = (): NoopSocket => ({
	url: "ws://example.test/stream",
	addEventListener: () => {},
	removeEventListener: () => {},
	close: vi.fn(),
});

const createPageTestQueryClient = (): QueryClient =>
	new QueryClient({
		defaultOptions: {
			queries: {
				retry: false,
				gcTime: Number.POSITIVE_INFINITY,
				refetchOnWindowFocus: false,
				networkMode: "offlineFirst",
				staleTime: Number.POSITIVE_INFINITY,
			},
		},
	});

const makeChat = (
	chatId: string,
	status: TypesGen.ChatStatus = "completed",
): TypesGen.Chat => ({
	id: chatId,
	organization_id: "test-org-id",
	owner_id: "owner-1",
	last_model_config_id: "model-1",
	mcp_server_ids: [],
	labels: {},
	title: `chat ${chatId}`,
	status,
	created_at: "2025-01-01T00:00:00.000Z",
	updated_at: "2025-01-01T00:00:00.000Z",
	archived: false,
	pin_order: 0,
	has_unread: false,
	client_type: "ui",
	last_error: null,
	children: [],
});

const makeMessage = (
	chatId: string,
	id: number,
	text = `message ${id}`,
): TypesGen.ChatMessage => ({
	id,
	chat_id: chatId,
	created_at: `2025-01-01T00:00:00.${String(id).padStart(3, "0")}Z`,
	role: id % 2 === 0 ? "assistant" : "user",
	content: [{ type: "text", text }],
});

const makeMessagesData = (
	messages: readonly TypesGen.ChatMessage[],
	queuedMessages: readonly TypesGen.ChatQueuedMessage[] = [],
): TypesGen.ChatMessagesResponse => ({
	messages,
	queued_messages: queuedMessages,
	has_more: false,
});

const seedCommonQueries = (queryClient: QueryClient) => {
	queryClient.setQueryData<TypesGen.ChatModelsResponse>(["chat-models"], {
		providers: [],
	});
	queryClient.setQueryData<readonly TypesGen.ChatModelConfig[]>(
		["chat-model-configs"],
		[],
	);
	queryClient.setQueryData<TypesGen.UserChatCompactionThresholds>(
		["chat-user-compaction-thresholds"],
		{ thresholds: [] },
	);
	queryClient.setQueryData<TypesGen.ChatDesktopEnabledResponse>(
		["chat-desktop-enabled"],
		{ enable_desktop: false },
	);
	queryClient.setQueryData<TypesGen.UserChatDebugLoggingSettings>(
		["user-chat-debug-logging"],
		{
			debug_logging_enabled: false,
			forced_by_deployment: false,
			user_toggle_allowed: true,
		},
	);
	queryClient.setQueryData<readonly TypesGen.MCPServerConfig[]>(
		["mcp-server-configs"],
		[],
	);
	queryClient.setQueryData<TypesGen.WorkspacesResponse>(
		workspacesKey({ q: "owner:me", limit: 0 }),
		{ workspaces: [], count: 0 },
	);
	queryClient.setQueryData<TypesGen.SSHConfigResponse>(
		["deployment", "sshConfig"],
		{
			hostname_prefix: "coder",
			hostname_suffix: "coder.example.test",
			ssh_config_options: {},
		},
	);
};

const seedChatQueries = ({
	queryClient,
	chatId,
	messages,
	status = "completed",
}: {
	queryClient: QueryClient;
	chatId: string;
	messages: readonly TypesGen.ChatMessage[];
	status?: TypesGen.ChatStatus;
}) => {
	queryClient.setQueryData<TypesGen.Chat>(
		chatKey(chatId),
		makeChat(chatId, status),
	);
	queryClient.setQueryData<InfiniteData<TypesGen.ChatMessagesResponse>>(
		chatMessagesKey(chatId),
		{
			pages: [makeMessagesData(messages)],
			pageParams: [undefined],
		},
	);
};

const makeOutletContext = (): AgentsOutletContext => {
	const scrollContainerRef: RefObject<HTMLDivElement | null> = {
		current: null,
	};
	return {
		chatErrorReasons: {},
		setChatErrorReason: vi.fn<AgentsOutletContext["setChatErrorReason"]>(),
		clearChatErrorReason: vi.fn<AgentsOutletContext["clearChatErrorReason"]>(),
		requestArchiveAgent: vi.fn<AgentsOutletContext["requestArchiveAgent"]>(),
		requestUnarchiveAgent:
			vi.fn<AgentsOutletContext["requestUnarchiveAgent"]>(),
		requestArchiveAndDeleteWorkspace:
			vi.fn<AgentsOutletContext["requestArchiveAndDeleteWorkspace"]>(),
		requestPinAgent: vi.fn<AgentsOutletContext["requestPinAgent"]>(),
		requestUnpinAgent: vi.fn<AgentsOutletContext["requestUnpinAgent"]>(),
		regeneratingTitleChatIds: [],
		isSidebarCollapsed: false,
		onToggleSidebarCollapsed:
			vi.fn<AgentsOutletContext["onToggleSidebarCollapsed"]>(),
		onExpandSidebar: vi.fn<AgentsOutletContext["onExpandSidebar"]>(),
		onChatReady: vi.fn<AgentsOutletContext["onChatReady"]>(),
		scrollContainerRef,
	};
};

const AgentChatRouteParent = () => <Outlet context={makeOutletContext()} />;

const renderAgentChatPage = ({
	initialChatId,
	seededChats,
}: {
	initialChatId: string;
	seededChats: readonly {
		chatId: string;
		messages: readonly TypesGen.ChatMessage[];
		status?: TypesGen.ChatStatus;
	}[];
}) => {
	const queryClient = createPageTestQueryClient();
	seedCommonQueries(queryClient);
	for (const chat of seededChats) {
		seedChatQueries({ queryClient, ...chat });
	}

	const router = createMemoryRouter(
		[
			{
				path: "/agents",
				element: <AgentChatRouteParent />,
				children: [{ path: ":agentId", element: <AgentChatPage /> }],
			},
		],
		{ initialEntries: [`/agents/${initialChatId}`] },
	);

	return {
		...render(
			<QueryClientProvider client={queryClient}>
				<ChatSessionsProvider
					setChatErrorReason={vi.fn()}
					clearChatErrorReason={vi.fn()}
				>
					<RouterProvider router={router} />
				</ChatSessionsProvider>
			</QueryClientProvider>,
		),
		queryClient,
		router,
	};
};

const latestViewProps = (): AgentChatPageViewMockProps => {
	const props = agentChatPageViewProps[agentChatPageViewProps.length - 1];
	if (!props) {
		throw new Error("Expected AgentChatPageView to have rendered.");
	}
	return props;
};

const waitForViewProps = async (
	agentId: string,
): Promise<AgentChatPageViewMockProps> => {
	await waitFor(() => {
		expect(latestViewProps().agentId).toBe(agentId);
	});
	return latestViewProps();
};

const textFromMessage = (
	message: TypesGen.ChatMessage | undefined,
): string | undefined => {
	const part = message?.content?.[0];
	return part?.type === "text" ? part.text : undefined;
};

const makeLifecycleCall = (
	type: LifecycleCall["type"],
	chatId: string,
	order: number,
): LifecycleCall => ({ type, chatId, order });

const collectLifecycleCalls = (
	markVisibleSpy: ReturnType<
		typeof vi.spyOn<ChatSessionManager, "markVisible">
	>,
	releaseVisibleSpy: ReturnType<
		typeof vi.spyOn<ChatSessionManager, "releaseVisible">
	>,
): LifecycleCall[] => {
	return [
		...markVisibleSpy.mock.calls.map(([chatId], index) =>
			makeLifecycleCall(
				"mark",
				chatId,
				markVisibleSpy.mock.invocationCallOrder[index] ?? 0,
			),
		),
		...releaseVisibleSpy.mock.calls.map(([chatId], index) =>
			makeLifecycleCall(
				"release",
				chatId,
				releaseVisibleSpy.mock.invocationCallOrder[index] ?? 0,
			),
		),
	].sort((left, right) => left.order - right.order);
};

const includesLifecycleSubsequence = (
	calls: readonly LifecycleCall[],
	expected: readonly Pick<LifecycleCall, "type" | "chatId">[],
): boolean => {
	let expectedIndex = 0;
	for (const call of calls) {
		const nextExpected = expected[expectedIndex];
		if (!nextExpected) {
			return true;
		}
		if (
			call.type === nextExpected.type &&
			call.chatId === nextExpected.chatId
		) {
			expectedIndex += 1;
		}
	}
	return expectedIndex === expected.length;
};

beforeEach(() => {
	agentChatPageViewProps.length = 0;
	apiMocks.watchChat.mockImplementation(createNoopSocket);
	apiMocks.watchWorkspace.mockImplementation(createNoopSocket);
	localStorage.clear();
});

afterEach(() => {
	cleanup();
	vi.restoreAllMocks();
	vi.clearAllMocks();
	localStorage.clear();
});

describe("AgentChatPage chat session wiring", () => {
	it("hydrates the manager-owned store from the REST chat data", async () => {
		const messages = [
			makeMessage("chat-a", 1, "first seeded message"),
			makeMessage("chat-a", 2, "second seeded message"),
		];
		renderAgentChatPage({
			initialChatId: "chat-a",
			seededChats: [{ chatId: "chat-a", messages }],
		});

		const { store } = await waitForViewProps("chat-a");
		expect(store).toBeDefined();

		await waitFor(() => {
			const snapshot = store.getSnapshot();
			expect(snapshot.orderedMessageIDs).toEqual([1, 2]);
			expect(textFromMessage(snapshot.messagesByID.get(1))).toBe(
				"first seeded message",
			);
			expect(textFromMessage(snapshot.messagesByID.get(2))).toBe(
				"second seeded message",
			);
			expect(snapshot.chatStatus).toBe("completed");
		});
		expect(apiMocks.watchChat).toHaveBeenCalledWith("chat-a", 2);
	});

	it("reuses a session store when the agentId changes and then returns", async () => {
		const { router } = renderAgentChatPage({
			initialChatId: "chat-a",
			seededChats: [
				{ chatId: "chat-a", messages: [makeMessage("chat-a", 1)] },
				{ chatId: "chat-b", messages: [makeMessage("chat-b", 10)] },
			],
		});
		const storeA1 = (await waitForViewProps("chat-a")).store;

		await act(async () => {
			await router.navigate("/agents/chat-b");
		});
		const storeB = (await waitForViewProps("chat-b")).store;

		await act(async () => {
			await router.navigate("/agents/chat-a");
		});
		const storeA3 = (await waitForViewProps("chat-a")).store;

		expect(storeA3).toBe(storeA1);
		expect(storeB).not.toBe(storeA1);
	});

	it("marks the active chat visible and releases it on route changes", async () => {
		const markVisibleSpy = vi.spyOn(
			ChatSessionManager.prototype,
			"markVisible",
		);
		const releaseVisibleSpy = vi.spyOn(
			ChatSessionManager.prototype,
			"releaseVisible",
		);
		const { router, unmount } = renderAgentChatPage({
			initialChatId: "chat-a",
			seededChats: [
				{ chatId: "chat-a", messages: [makeMessage("chat-a", 1)] },
				{ chatId: "chat-b", messages: [makeMessage("chat-b", 10)] },
			],
		});
		await waitFor(() => {
			expect(markVisibleSpy).toHaveBeenCalledWith("chat-a");
		});

		await act(async () => {
			await router.navigate("/agents/chat-b");
		});
		await waitFor(() => {
			expect(releaseVisibleSpy).toHaveBeenCalledWith("chat-a");
			expect(markVisibleSpy).toHaveBeenCalledWith("chat-b");
		});

		unmount();
		await waitFor(() => {
			expect(releaseVisibleSpy).toHaveBeenCalledWith("chat-b");
		});

		const lifecycleCalls = collectLifecycleCalls(
			markVisibleSpy,
			releaseVisibleSpy,
		);
		expect(
			includesLifecycleSubsequence(lifecycleCalls, [
				{ type: "mark", chatId: "chat-a" },
				{ type: "release", chatId: "chat-a" },
				{ type: "mark", chatId: "chat-b" },
				{ type: "release", chatId: "chat-b" },
			]),
		).toBe(true);
	});
});
