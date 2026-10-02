import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
	type InfiniteData,
	InfiniteQueryObserver,
	useQuery,
} from "react-query";
import { useOutletContext, useParams } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import * as apiModule from "#/api/api";
import { API } from "#/api/api";
import {
	chat as chatById,
	chatEntityKey,
	chatListFamilyKey,
	infiniteChats,
	readInfiniteChatsCache,
} from "#/api/queries/chats";
import type {
	Chat,
	ChatWatchEvent,
	WorkspaceBuild,
} from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import { MockUnsetUserChatPersonalModelOverrides } from "#/testHelpers/chatModels";
import { createDeferred } from "#/testHelpers/deferred";
import {
	MockWorkspace,
	MockWorkspaceBuildDelete,
	MockWorkspaceQuota,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import {
	createMockWebSocket,
	type MockWebSocketServer,
} from "#/testHelpers/websockets";
import { OneWayWebSocket } from "#/utils/OneWayWebSocket";
import AgentsPageLayout, {
	type AgentsPageOutletContext,
} from "./AgentsPageLayout";
import { emptyInputStorageKey } from "./components/AgentCreateForm";
import { ChatTopBar } from "./components/ChatTopBar";

const ChatPage = () => {
	const { agentId = "" } = useParams();
	const { data: chat } = useQuery(chatById(agentId));
	return (
		<ChatTopBar
			key={agentId}
			chat={chat}
			panel={{ showSidebarPanel: false, onToggleSidebar: vi.fn() }}
		/>
	);
};

const renderLayout = (route = "/agents") =>
	renderWithAuth(<AgentsPageLayout />, {
		path: "/agents",
		route,
		children: [
			{ index: true, element: null },
			{ path: ":agentId", element: <ChatPage /> },
		],
		extraRoutes: [{ path: "/workspaces", element: null }],
	});

beforeEach(() => {
	vi.spyOn(API, "getWorkspaceQuota").mockResolvedValue(MockWorkspaceQuota);
	vi.spyOn(API.experimental, "getChatModels").mockResolvedValue([]);
	vi.spyOn(
		API.experimental,
		"getUserChatPersonalModelOverrides",
	).mockResolvedValue(MockUnsetUserChatPersonalModelOverrides);
});

afterEach(() => {
	vi.restoreAllMocks();
	localStorage.clear();
});

describe("AgentsPageLayout New chat", () => {
	// A prompt link always replaces the draft in the composer, so New chat
	// keeps it. A debug link can fall back to the draft-backed composer, so
	// New chat clears the draft as it does on a plain composer.
	it.each([
		["prompt", { prompt: "hi" }, "draft the user typed earlier"],
		["debug", { debugWorkspaceBuildId: "build-id" }, null],
	])(
		"handles the saved draft on New chat after a %s link",
		async (_, linkState, expectedDraft) => {
			vi.spyOn(API.experimental, "getChats").mockResolvedValue([]);
			localStorage.setItem(
				emptyInputStorageKey,
				"draft the user typed earlier",
			);
			const user = userEvent.setup();

			const { router } = renderLayout();
			// AgentCreatePage leaves a deep link's value in history state.
			await router.navigate("/agents", { state: linkState });
			await user.click(await screen.findByRole("link", { name: "New chat" }));

			await waitFor(() => expect(router.state.location.state).toBeNull());
			expect(localStorage.getItem(emptyInputStorageKey)).toBe(expectedDraft);
		},
	);
});

describe("AgentsPageLayout manual read state", () => {
	it("keeps a pending toggle scoped to its chat after the row remounts", async () => {
		const user = userEvent.setup();
		let chats: Chat[] = [
			{ ...MockChat, id: "read-alpha", title: "Alpha agent", has_unread: true },
			{ ...MockChat, id: "read-beta", title: "Beta agent", has_unread: true },
		];
		vi.spyOn(API.experimental, "getChats").mockImplementation(
			async () => chats,
		);
		const pendingUpdate = createDeferred<undefined>();
		const pendingBeta = createDeferred<undefined>();
		const update = vi
			.spyOn(API.experimental, "updateChat")
			.mockImplementation(async (id, request) => {
				if (id === "read-alpha" && request.read) {
					await pendingUpdate.promise;
				} else if (id === "read-beta") {
					await pendingBeta.promise;
				}
				chats = chats.map((chat) =>
					chat.id === id ? { ...chat, has_unread: !request.read } : chat,
				);
			});
		const { router, queryClient } = renderLayout();
		queryClient.setQueryDefaults(chatListFamilyKey, {
			gcTime: Number.POSITIVE_INFINITY,
			staleTime: Number.POSITIVE_INFINITY,
		});
		const unreadList = new InfiniteQueryObserver(queryClient, {
			...infiniteChats({
				chatStatus: "unread",
				sources: ["created_by_me"],
			}),
			enabled: false,
		});
		const unsubscribe = unreadList.subscribe(() => {});
		const openActions = async (title: string) => {
			await user.click(
				await screen.findByRole("button", {
					name: `Open actions for ${title}`,
				}),
			);
		};

		try {
			await screen.findByRole("button", {
				name: "Open actions for Alpha agent",
			});
			await act(() => router.navigate("/agents?unread=true"));
			await openActions("Alpha agent");
			await user.click(
				await screen.findByRole("menuitem", { name: "Mark as read" }),
			);
			await waitFor(() => {
				expect(update).toHaveBeenCalledWith("read-alpha", { read: true });
				expect(
					unreadList
						.getCurrentResult()
						.data?.pages.flat()
						.map((chat) => chat.id),
				).toEqual(["read-beta"]);
			});
			await act(() => router.navigate("/agents"));
			await openActions("Alpha agent");
			await user.click(
				await screen.findByRole("menuitem", { name: "Mark as unread" }),
			);
			expect(update).toHaveBeenCalledTimes(1);
			await user.keyboard("{Escape}");
			await openActions("Beta agent");
			await user.click(
				await screen.findByRole("menuitem", { name: "Mark as read" }),
			);
			await waitFor(() =>
				expect(update).toHaveBeenCalledWith("read-beta", { read: true }),
			);
			pendingUpdate.resolve(undefined);
			await waitFor(() => expect(queryClient.isMutating()).toBe(1));
			expect(
				queryClient
					.getQueryData<InfiniteData<Chat[]>>(
						infiniteChats({ sources: ["created_by_me"] }).queryKey,
					)
					?.pages.flat()
					.find((chat) => chat.id === "read-beta")?.has_unread,
			).toBe(false);
			await openActions("Alpha agent");
			await user.click(
				await screen.findByRole("menuitem", { name: "Mark as unread" }),
			);
			await waitFor(() => expect(update).toHaveBeenCalledTimes(3));
		} finally {
			pendingUpdate.resolve(undefined);
			pendingBeta.resolve(undefined);
			unsubscribe();
			await waitFor(() => expect(queryClient.isMutating()).toBe(0));
		}
	});
});

describe("AgentsPageLayout archive state", () => {
	it.each([
		{ surface: "sidebar", succeeds: true },
		{ surface: "top bar", succeeds: true },
		{ surface: "sidebar", succeeds: false },
		{ surface: "top bar", succeeds: false },
	])(
		"$surface clears chat errors only on archive success ($succeeds)",
		async ({ surface, succeeds }) => {
			const user = userEvent.setup();
			const error = { message: "Agent failed", kind: "generic" as const };
			const readError = vi.fn();
			const ChatRoute = () => {
				const { chatErrorReasons, setChatErrorReason } =
					useOutletContext<AgentsPageOutletContext>();
				return (
					<>
						<button
							type="button"
							onClick={() => setChatErrorReason(MockChat.id, error)}
						>
							Set error
						</button>
						<button
							type="button"
							onClick={() => readError(chatErrorReasons[MockChat.id])}
						>
							Read error
						</button>
						<ChatTopBar
							chat={MockChat}
							panel={{ showSidebarPanel: false, onToggleSidebar: vi.fn() }}
						/>
					</>
				);
			};
			vi.spyOn(API.experimental, "getChats").mockResolvedValue([MockChat]);
			const update = vi.spyOn(API.experimental, "updateChat");
			if (succeeds) {
				update.mockResolvedValue(undefined);
			} else {
				update.mockRejectedValue(new Error("Archive rejected"));
			}
			const { queryClient } = renderWithAuth(<AgentsPageLayout />, {
				path: "/agents",
				route: `/agents/${MockChat.id}`,
				children: [{ path: ":agentId", element: <ChatRoute /> }],
			});
			await user.click(
				await screen.findByRole("button", { name: "Set error" }),
			);
			await user.click(screen.getByRole("button", { name: "Read error" }));
			expect(readError).toHaveBeenLastCalledWith(error);
			await user.click(
				await screen.findByRole("button", {
					name:
						surface === "sidebar"
							? `Open actions for ${MockChat.title}`
							: "Open agent actions",
				}),
			);
			await user.click(
				await screen.findByRole("menuitem", { name: "Archive agent" }),
			);
			await waitFor(() => {
				expect(update).toHaveBeenCalledWith(MockChat.id, { archived: true });
				expect(queryClient.isMutating()).toBe(0);
			});
			await user.click(screen.getByRole("button", { name: "Read error" }));
			expect(readError).toHaveBeenLastCalledWith(succeeds ? undefined : error);
		},
	);

	it.each([
		{ action: "Archive agent", archived: false },
		{ action: "Unarchive agent", archived: true },
	])("$action does not block another chat", async ({ action, archived }) => {
		const user = userEvent.setup();
		vi.spyOn(API.experimental, "getChats").mockResolvedValue([
			{ ...MockChat, id: "chat-alpha", title: "Alpha agent", archived },
			{ ...MockChat, id: "chat-beta", title: "Beta agent", archived },
		]);
		const pendingUpdate = createDeferred<undefined>();
		vi.spyOn(API.experimental, "updateChat").mockReturnValue(
			pendingUpdate.promise,
		);
		const { queryClient } = renderLayout(
			archived ? "/agents?archived=archived" : "/agents",
		);

		try {
			for (const [id, title] of [
				["chat-alpha", "Alpha agent"],
				["chat-beta", "Beta agent"],
			]) {
				await user.click(
					await screen.findByRole("button", {
						name: `Open actions for ${title}`,
					}),
				);
				await user.click(await screen.findByRole("menuitem", { name: action }));
				await waitFor(() => {
					expect(API.experimental.updateChat).toHaveBeenCalledWith(id, {
						archived: !archived,
					});
				});
			}
		} finally {
			pendingUpdate.resolve(undefined);
			await waitFor(() => expect(queryClient.isMutating()).toBe(0));
		}
	});
});

describe("AgentsPageLayout archive and delete", () => {
	it.each(["sidebar", "top bar"])(
		"shares pending archive state from the %s with the other surface",
		async (surface) => {
			const user = userEvent.setup();
			const chat = {
				...MockChat,
				title: "Archive target",
				workspace_id: MockWorkspace.id,
			};
			vi.spyOn(API.experimental, "getChats").mockResolvedValue([chat]);
			vi.spyOn(API, "getWorkspace").mockResolvedValue({
				...MockWorkspace,
				created_at: chat.created_at,
			});
			vi.spyOn(API, "getWorkspaceBuilds").mockResolvedValue([]);
			vi.spyOn(API, "deleteWorkspace").mockResolvedValue(
				MockWorkspaceBuildDelete,
			);
			const pendingUpdate = createDeferred<undefined>();
			vi.spyOn(API.experimental, "updateChat").mockReturnValue(
				pendingUpdate.promise,
			);
			const { queryClient } = renderWithAuth(<AgentsPageLayout />, {
				path: "/agents",
				route: `/agents/${chat.id}`,
				children: [
					{
						path: ":agentId",
						element: (
							<ChatTopBar
								chat={chat}
								panel={{ showSidebarPanel: false, onToggleSidebar: vi.fn() }}
							/>
						),
					},
				],
			});
			const sidebarTrigger = `Open actions for ${chat.title}`;
			const topBarTrigger = "Open agent actions";
			try {
				await user.click(
					await screen.findByRole("button", {
						name: surface === "sidebar" ? sidebarTrigger : topBarTrigger,
					}),
				);
				await user.click(
					await screen.findByRole("menuitem", {
						name: "Archive & delete workspace",
					}),
				);
				await waitFor(() =>
					expect(API.experimental.updateChat).toHaveBeenCalledTimes(1),
				);
				if (surface === "sidebar") {
					await user.click(
						await screen.findByRole("button", { name: topBarTrigger }),
					);
				} else {
					await user.pointer({
						target: await screen.findByRole("link", {
							name: /^Archive target/,
						}),
						keys: "[MouseRight]",
					});
				}
				await user.click(
					await screen.findByRole("menuitem", { name: "Archive agent" }),
				);
				expect(API.experimental.updateChat).toHaveBeenCalledTimes(1);
				await user.keyboard("{Escape}");
			} finally {
				pendingUpdate.resolve(undefined);
				await waitFor(() => expect(queryClient.isMutating()).toBe(0));
			}
		},
	);

	const chat = {
		...MockChat,
		id: "chat-alpha",
		title: "Alpha agent",
		workspace_id: "ws-alpha",
	};
	const otherChat = {
		...MockChat,
		id: "chat-beta",
		title: "Beta agent",
		workspace_id: "ws-beta",
	};
	const childChat = {
		...MockChat,
		id: "chat-child",
		parent_chat_id: chat.id,
		root_chat_id: chat.id,
	};
	let chatWatchServer: MockWebSocketServer | undefined;

	beforeEach(() => {
		vi.spyOn(API.experimental, "getChats").mockResolvedValue([chat, otherChat]);
		vi.spyOn(API.experimental, "getChat").mockImplementation(async (chatId) => {
			const result = [chat, otherChat, childChat].find(
				(chat) => chat.id === chatId,
			);
			if (!result) throw new Error(`Unexpected chat: ${chatId}`);
			return result;
		});
		vi.spyOn(API.experimental, "updateChat").mockResolvedValue(undefined);
		vi.spyOn(API, "checkAuthorization").mockResolvedValue({
			canShareChat: false,
		});
		vi.spyOn(API, "getWorkspace").mockImplementation(async (workspaceId) => ({
			...MockWorkspace,
			id: workspaceId,
			created_at: chat.created_at,
		}));
		vi.spyOn(API, "getWorkspaceBuilds").mockResolvedValue([]);
		vi.spyOn(API, "deleteWorkspace").mockResolvedValue(
			MockWorkspaceBuildDelete,
		);
		chatWatchServer = undefined;
		vi.spyOn(apiModule, "watchChats").mockImplementation(
			() =>
				new OneWayWebSocket<ChatWatchEvent>({
					apiRoute: "/api/v2/chats/watch",
					websocketInit: (url, protocol) => {
						const [socket, server] = createMockWebSocket(url, protocol);
						chatWatchServer = server;
						return socket;
					},
				}),
		);
	});

	it("deletes two workspaces without waiting for the first", async () => {
		const user = userEvent.setup({ delay: null });
		const pendingDelete = createDeferred<WorkspaceBuild>();
		vi.mocked(API.deleteWorkspace).mockReturnValue(pendingDelete.promise);
		const { queryClient } = renderLayout();
		try {
			await user.click(
				await screen.findByRole("button", {
					name: "Open actions for Alpha agent",
				}),
			);
			await user.click(
				await screen.findByRole("menuitem", {
					name: "Archive & delete workspace",
				}),
			);
			await waitFor(
				() => {
					expect(API.deleteWorkspace).toHaveBeenCalledWith("ws-alpha");
				},
				{ timeout: 4000 },
			);
			await user.click(
				await screen.findByRole(
					"button",
					{ name: "Open actions for Beta agent" },
					{ timeout: 4000 },
				),
			);
			await user.click(
				await screen.findByRole("menuitem", {
					name: "Archive & delete workspace",
				}),
			);
			await waitFor(
				() => {
					expect(API.deleteWorkspace).toHaveBeenCalledWith("ws-beta");
				},
				{ timeout: 4000 },
			);
		} finally {
			await act(async () => pendingDelete.resolve(MockWorkspaceBuildDelete));
			await waitFor(() => expect(queryClient.isMutating()).toBe(0));
		}
	});

	describe.each(["top bar", "sidebar"])("from the %s", (surface) => {
		const clickArchiveAndDelete = async (
			user: ReturnType<typeof userEvent.setup>,
		) => {
			await user.click(
				await screen.findByRole("button", {
					name:
						surface === "top bar"
							? "Open agent actions"
							: "Open actions for Alpha agent",
				}),
			);
			await user.click(
				await screen.findByRole("menuitem", {
					name: "Archive & delete workspace",
				}),
			);
		};

		it.each([
			{ name: "the archived chat", chatId: chat.id, expectedPath: "/agents" },
			{
				name: "an unrelated chat",
				chatId: otherChat.id,
				expectedPath: `/agents/${otherChat.id}`,
			},
			{
				name: "a subagent of the archived chat",
				chatId: childChat.id,
				expectedPath: "/agents",
			},
			{ name: "another page", chatId: undefined, expectedPath: "/workspaces" },
		])(
			"respects the current route when viewing $name after deletion completes",
			async ({ chatId, expectedPath }) => {
				const user = userEvent.setup();
				const pendingDelete = createDeferred<WorkspaceBuild>();
				vi.mocked(API.deleteWorkspace).mockReturnValue(pendingDelete.promise);
				const { router, queryClient } = renderLayout(`/agents/${chat.id}`);
				try {
					await clickArchiveAndDelete(user);
					await waitFor(() =>
						expect(API.deleteWorkspace).toHaveBeenCalledWith(chat.workspace_id),
					);

					// Archiving emits this event before the workspace deletion finishes,
					// removing the sidebar row that started the mutation.
					vi.mocked(API.experimental.getChats).mockResolvedValue([otherChat]);
					if (!chatWatchServer)
						throw new Error("Chat watch connection was not opened");
					const event: ChatWatchEvent = {
						kind: "deleted",
						chat: { ...chat, archived: true },
					};
					act(() =>
						chatWatchServer?.publishMessage(
							new MessageEvent("message", { data: JSON.stringify(event) }),
						),
					);
					await waitFor(() =>
						expect(readInfiniteChatsCache(queryClient)).toEqual([otherChat]),
					);

					const destination = chatId ? `/agents/${chatId}` : "/workspaces";
					await act(async () => {
						await router.navigate(`${destination}?group_by=chat_status`);
					});
					if (chatId) {
						await waitFor(() =>
							expect(
								queryClient.getQueryData(chatEntityKey(chatId)),
							).toMatchObject({ id: chatId }),
						);
					}
					await act(async () =>
						pendingDelete.resolve(MockWorkspaceBuildDelete),
					);
					await waitFor(() => expect(queryClient.isMutating()).toBe(0));
					await waitFor(() =>
						expect(router.state.location.pathname).toBe(expectedPath),
					);
					expect(router.state.location.search).toBe("?group_by=chat_status");
				} finally {
					await act(async () =>
						pendingDelete.resolve(MockWorkspaceBuildDelete),
					);
					await waitFor(() => expect(queryClient.isMutating()).toBe(0));
				}
			},
		);

		it.each([404, 410])(
			"leaves the archived chat when workspace lookup returns %s",
			async (status) => {
				const user = userEvent.setup();
				vi.mocked(API.getWorkspace).mockRejectedValue({
					isAxiosError: true,
					response: { status },
				});
				const pendingArchive = createDeferred<undefined>();
				vi.mocked(API.experimental.updateChat).mockReturnValue(
					pendingArchive.promise,
				);
				const { router, queryClient } = renderLayout(`/agents/${chat.id}`);
				try {
					await clickArchiveAndDelete(user);
					await waitFor(() =>
						expect(API.experimental.updateChat).toHaveBeenCalledWith(chat.id, {
							archived: true,
						}),
					);
					expect(router.state.location.pathname).toBe(`/agents/${chat.id}`);

					await act(async () => pendingArchive.resolve(undefined));
					await waitFor(() => expect(queryClient.isMutating()).toBe(0));
					expect(API.deleteWorkspace).not.toHaveBeenCalled();
					await waitFor(() =>
						expect(router.state.location.pathname).toBe("/agents"),
					);
				} finally {
					await act(async () => pendingArchive.resolve(undefined));
					await waitFor(() => expect(queryClient.isMutating()).toBe(0));
				}
			},
		);

		it("does not leave a missing-workspace chat when archiving fails", async () => {
			const user = userEvent.setup();
			vi.mocked(API.getWorkspace).mockRejectedValue({
				isAxiosError: true,
				response: { status: 404 },
			});
			vi.mocked(API.experimental.updateChat).mockRejectedValue(
				new Error("Archive failed"),
			);
			const { router, queryClient } = renderLayout(`/agents/${chat.id}`);
			await clickArchiveAndDelete(user);
			await waitFor(() =>
				expect(API.experimental.updateChat).toHaveBeenCalledWith(chat.id, {
					archived: true,
				}),
			);
			await waitFor(() => expect(queryClient.isMutating()).toBe(0));
			expect(API.deleteWorkspace).not.toHaveBeenCalled();
			expect(router.state.location.pathname).toBe(`/agents/${chat.id}`);
		});
	});
});
