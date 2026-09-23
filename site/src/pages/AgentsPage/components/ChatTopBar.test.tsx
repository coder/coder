import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { Outlet, useParams } from "react-router";
import { toast } from "sonner";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { archiveAndDeleteChatKey, chatEntityKey } from "#/api/queries/chats";
import type { Chat, WorkspaceBuild } from "#/api/typesGenerated";
import { MockChat, MockChatDiffStatus } from "#/testHelpers/chatEntities";
import { createDeferred } from "#/testHelpers/deferred";
import {
	MockWorkspace,
	MockWorkspaceBuildDelete,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { rightPanelTabStorageKeyPrefix } from "../utils/rightPanelTabStorage";
import {
	getPersistedSidebarTabId,
	savePersistedSidebarTabId,
} from "../utils/sidebarTabStorage";
import { ChatTopBar } from "./ChatTopBar";

const chat = { ...MockChat, workspace_id: "workspace-1" };
const navigateAfterArchive = vi.fn();

const renderTopBar = (currentChat = chat, liveChatStatus?: Chat["status"]) =>
	renderWithAuth(<Outlet context={{ navigateAfterArchive }} />, {
		route: `/agents/${chat.id}`,
		path: "/agents",
		children: [
			{
				path: ":agentId",
				element: (
					<ChatTopBar
						chat={currentChat}
						liveChatStatus={liveChatStatus}
						panel={{ showSidebarPanel: false, onToggleSidebar: vi.fn() }}
					/>
				),
			},
		],
	});

const mockArchiveAndDeleteApi = (workspaceCreatedAt: string) => {
	vi.spyOn(API, "checkAuthorization").mockResolvedValue({
		canShareChat: false,
	});
	vi.spyOn(API, "getWorkspace").mockResolvedValue({
		...MockWorkspace,
		id: "workspace-1",
		created_at: workspaceCreatedAt,
	});
	vi.spyOn(API, "getWorkspaceBuilds").mockResolvedValue([]);
	vi.spyOn(API.experimental, "updateChat").mockResolvedValue(undefined);
	vi.spyOn(API, "deleteWorkspace").mockResolvedValue(MockWorkspaceBuildDelete);
};

const clickArchiveAndDelete = async (
	user: ReturnType<typeof userEvent.setup>,
) => {
	await user.click(
		await screen.findByRole("button", { name: "Open agent actions" }),
	);
	await user.click(
		await screen.findByRole("menuitem", { name: "Archive & delete workspace" }),
	);
};

afterEach(() => {
	vi.restoreAllMocks();
	localStorage.clear();
	navigateAfterArchive.mockClear();
});

describe("ChatTopBar pin state", () => {
	it.each([
		{ action: "Pin agent", pinOrder: 0, expectedOrder: 1 },
		{ action: "Unpin agent", pinOrder: 1, expectedOrder: 0 },
	])(
		"$action updates the chat without layout callbacks",
		async ({ action, pinOrder, expectedOrder }) => {
			const user = userEvent.setup();
			mockArchiveAndDeleteApi(chat.created_at);
			const { queryClient } = renderTopBar({ ...chat, pin_order: pinOrder });
			queryClient.setQueryDefaults(chatEntityKey(chat.id), {
				gcTime: Number.POSITIVE_INFINITY,
			});
			queryClient.setQueryData(chatEntityKey(chat.id), {
				...chat,
				pin_order: pinOrder,
			});
			await user.click(
				await screen.findByRole("button", { name: "Open agent actions" }),
			);
			await user.click(await screen.findByRole("menuitem", { name: action }));
			await waitFor(() => {
				expect(API.experimental.updateChat).toHaveBeenCalledWith(chat.id, {
					pin_order: expectedOrder,
				});
				expect(queryClient.isMutating()).toBe(0);
			});
			expect(
				queryClient.getQueryData<Chat>(chatEntityKey(chat.id))?.pin_order,
			).toBe(expectedOrder);
		},
	);

	it.each([
		{ action: "Pin agent", pinOrder: 0 },
		{ action: "Unpin agent", pinOrder: 1 },
	])("$action reports failure and rolls back", async ({ action, pinOrder }) => {
		const user = userEvent.setup();
		mockArchiveAndDeleteApi(chat.created_at);
		vi.mocked(API.experimental.updateChat).mockRejectedValue(
			new Error("Pin update rejected"),
		);
		const errorToast = vi.spyOn(toast, "error");
		const { queryClient } = renderTopBar({ ...chat, pin_order: pinOrder });
		queryClient.setQueryDefaults(chatEntityKey(chat.id), {
			gcTime: Number.POSITIVE_INFINITY,
		});
		queryClient.setQueryData(chatEntityKey(chat.id), {
			...chat,
			pin_order: pinOrder,
		});
		await user.click(
			await screen.findByRole("button", { name: "Open agent actions" }),
		);
		await user.click(await screen.findByRole("menuitem", { name: action }));
		await waitFor(() =>
			expect(errorToast).toHaveBeenCalledWith("Pin update rejected"),
		);
		expect(
			queryClient.getQueryData<Chat>(chatEntityKey(chat.id))?.pin_order,
		).toBe(pinOrder);
	});
});

describe("ChatTopBar archive state", () => {
	it.each([
		{ action: "Archive agent", archived: false },
		{ action: "Unarchive agent", archived: true },
	])("$action uses its own mutation", async ({ action, archived }) => {
		const user = userEvent.setup();
		mockArchiveAndDeleteApi(chat.created_at);
		const { router, queryClient } = renderTopBar({ ...chat, archived });
		queryClient.setQueryDefaults(chatEntityKey(chat.id), {
			gcTime: Number.POSITIVE_INFINITY,
		});
		queryClient.setQueryData(chatEntityKey(chat.id), { ...chat, archived });
		savePersistedSidebarTabId(chat.id, "terminal");
		const panelKey = `${rightPanelTabStorageKeyPrefix}${chat.id}`;
		localStorage.setItem(panelKey, "saved panel");

		await user.click(
			await screen.findByRole("button", { name: "Open agent actions" }),
		);
		await user.click(await screen.findByRole("menuitem", { name: action }));

		await waitFor(() => {
			expect(API.experimental.updateChat).toHaveBeenCalledWith(chat.id, {
				archived: !archived,
			});
			expect(queryClient.isMutating()).toBe(0);
		});
		expect(
			queryClient.getQueryData<Chat>(chatEntityKey(chat.id))?.archived,
		).toBe(!archived);
		expect(router.state.location.pathname).toBe(`/agents/${chat.id}`);
		expect(getPersistedSidebarTabId(chat.id)).toBe(
			archived ? "terminal" : null,
		);
		expect(localStorage.getItem(panelKey)).toBe(
			archived ? "saved panel" : null,
		);
	});

	it("cleans up an archived chat even after the surface unmounts", async () => {
		const user = userEvent.setup();
		mockArchiveAndDeleteApi(chat.created_at);
		const pendingUpdate = createDeferred<undefined>();
		vi.mocked(API.experimental.updateChat).mockReturnValue(
			pendingUpdate.promise,
		);
		savePersistedSidebarTabId(chat.id, "terminal");
		const { unmount, queryClient } = renderTopBar();

		try {
			await user.click(
				await screen.findByRole("button", { name: "Open agent actions" }),
			);
			await user.click(
				await screen.findByRole("menuitem", { name: "Archive agent" }),
			);
			await waitFor(() =>
				expect(API.experimental.updateChat).toHaveBeenCalled(),
			);
			unmount();
		} finally {
			pendingUpdate.resolve(undefined);
			await waitFor(() => expect(queryClient.isMutating()).toBe(0));
		}
		expect(getPersistedSidebarTabId(chat.id)).toBeNull();
	});

	it.each([
		{ action: "Archive agent", archived: false },
		{ action: "Unarchive agent", archived: true },
	])(
		"$action restores state and reports failure",
		async ({ action, archived }) => {
			const user = userEvent.setup();
			mockArchiveAndDeleteApi(chat.created_at);
			vi.mocked(API.experimental.updateChat).mockRejectedValue(
				new Error("Request rejected"),
			);
			const errorToast = vi.spyOn(toast, "error");
			savePersistedSidebarTabId(chat.id, "terminal");
			const { queryClient } = renderTopBar({ ...chat, archived });
			queryClient.setQueryDefaults(chatEntityKey(chat.id), {
				gcTime: Number.POSITIVE_INFINITY,
			});
			queryClient.setQueryData(chatEntityKey(chat.id), { ...chat, archived });

			await user.click(
				await screen.findByRole("button", { name: "Open agent actions" }),
			);
			await user.click(await screen.findByRole("menuitem", { name: action }));

			await waitFor(() => {
				expect(errorToast).toHaveBeenCalledWith("Request rejected");
			});
			expect(
				queryClient.getQueryData<Chat>(chatEntityKey(chat.id))?.archived,
			).toBe(archived);
			expect(getPersistedSidebarTabId(chat.id)).toBe("terminal");
		},
	);

	it("does not archive a running chat", async () => {
		const user = userEvent.setup();
		mockArchiveAndDeleteApi(chat.created_at);
		renderTopBar(chat, "running");

		await user.click(
			await screen.findByRole("button", { name: "Open agent actions" }),
		);
		await user.click(
			await screen.findByRole("menuitem", { name: "Archive agent" }),
		);

		expect(API.experimental.updateChat).not.toHaveBeenCalled();
	});

	it("tracks pending archives across navigation without blocking other chats", async () => {
		const user = userEvent.setup();
		mockArchiveAndDeleteApi(chat.created_at);
		const pendingUpdate = createDeferred<undefined>();
		vi.mocked(API.experimental.updateChat).mockReturnValue(
			pendingUpdate.promise,
		);
		const ChatRoute = () => {
			const { agentId = "" } = useParams();
			return (
				<ChatTopBar
					chat={{ ...chat, id: agentId }}
					panel={{ showSidebarPanel: false, onToggleSidebar: vi.fn() }}
				/>
			);
		};
		const { router, queryClient } = renderWithAuth(
			<Outlet context={{ navigateAfterArchive }} />,
			{
				route: "/agents/chat-alpha",
				path: "/agents",
				children: [{ path: ":agentId", element: <ChatRoute /> }],
			},
		);

		try {
			for (const id of ["chat-alpha", "chat-beta"]) {
				await act(() => router.navigate(`/agents/${id}`));
				await user.click(
					await screen.findByRole("button", { name: "Open agent actions" }),
				);
				await user.click(
					await screen.findByRole("menuitem", { name: "Archive agent" }),
				);
				await waitFor(() => {
					expect(API.experimental.updateChat).toHaveBeenCalledWith(id, {
						archived: true,
					});
				});
			}
			await act(() => router.navigate("/agents/chat-alpha"));
			await user.click(
				await screen.findByRole("button", { name: "Open agent actions" }),
			);
			await user.click(
				await screen.findByRole("menuitem", { name: "Archive agent" }),
			);
			expect(API.experimental.updateChat).toHaveBeenCalledTimes(2);
			await user.keyboard("{Escape}");
		} finally {
			pendingUpdate.resolve(undefined);
			await waitFor(() => expect(queryClient.isMutating()).toBe(0));
		}
		expect(router.state.location.pathname).toBe("/agents/chat-alpha");
	});
});

describe("ChatTopBar archive and delete", () => {
	it("archives the chat and notifies the layout after deleting its workspace", async () => {
		const user = userEvent.setup();
		mockArchiveAndDeleteApi(chat.created_at);

		renderTopBar();
		await clickArchiveAndDelete(user);

		await waitFor(() => {
			expect(API.deleteWorkspace).toHaveBeenCalledWith("workspace-1");
		});
		expect(API.experimental.updateChat).toHaveBeenCalledWith(chat.id, {
			archived: true,
		});
		await waitFor(() => {
			expect(navigateAfterArchive).toHaveBeenCalledWith(chat.id);
		});
	});

	it("deletes a workspace that predates the chat only after confirmation", async () => {
		const user = userEvent.setup();
		mockArchiveAndDeleteApi("2000-01-01T00:00:00.000Z");

		renderTopBar();
		await clickArchiveAndDelete(user);

		const nameField = await screen.findByLabelText(
			"Name of the workspace to delete",
		);
		expect(API.deleteWorkspace).not.toHaveBeenCalled();
		await user.type(nameField, MockWorkspace.name);
		await user.click(screen.getByRole("button", { name: "Delete" }));

		await waitFor(() => {
			expect(API.deleteWorkspace).toHaveBeenCalledWith("workspace-1");
		});
	});

	it("keeps an in-flight workspace lookup attached to its original chat", async () => {
		const user = userEvent.setup();
		mockArchiveAndDeleteApi(chat.created_at);
		const pendingDelete = createDeferred<WorkspaceBuild>();
		vi.mocked(API.deleteWorkspace).mockReturnValue(pendingDelete.promise);
		const pendingWorkspace = createDeferred<typeof MockWorkspace>();
		vi.mocked(API.getWorkspace).mockReturnValue(pendingWorkspace.promise);
		const otherChat = {
			...MockChat,
			id: "other-chat",
			workspace_id: "other-workspace",
		};
		const SwitchingTopBar = () => {
			const [activeChat, setActiveChat] = useState(chat);
			return (
				<>
					<button type="button" onClick={() => setActiveChat(otherChat)}>
						Switch chat
					</button>
					<ChatTopBar
						chat={activeChat}
						panel={{ showSidebarPanel: false, onToggleSidebar: vi.fn() }}
					/>
				</>
			);
		};
		const { queryClient } = renderWithAuth(
			<Outlet context={{ navigateAfterArchive }} />,
			{
				route: `/agents/${chat.id}`,
				path: "/agents",
				children: [{ path: ":agentId", element: <SwitchingTopBar /> }],
			},
		);
		await clickArchiveAndDelete(user);
		await user.click(screen.getByRole("button", { name: "Switch chat" }));
		try {
			await act(async () =>
				pendingWorkspace.resolve({
					...MockWorkspace,
					id: chat.workspace_id,
					created_at: chat.created_at,
				}),
			);
			await waitFor(() =>
				expect(API.deleteWorkspace).toHaveBeenCalledWith(chat.workspace_id),
			);
			expect(API.experimental.updateChat).toHaveBeenCalledWith(chat.id, {
				archived: true,
			});
			expect(
				queryClient.isMutating({
					mutationKey: archiveAndDeleteChatKey(chat.id),
				}),
			).toBe(1);
			expect(
				queryClient.isMutating({
					mutationKey: archiveAndDeleteChatKey(otherChat.id),
				}),
			).toBe(0);
		} finally {
			await act(async () => pendingDelete.resolve(MockWorkspaceBuildDelete));
			await waitFor(() => expect(queryClient.isMutating()).toBe(0));
		}
	});
});

describe("ChatTopBar PR chip", () => {
	it("links every tracked PR as a menu anchor when the chat tracks several", async () => {
		const user = userEvent.setup();

		// Both PRs share a title, so the numbers must name them apart.
		const primary = { ...MockChatDiffStatus };
		const secondary = {
			...MockChatDiffStatus,
			url: "https://github.com/coder/coder/pull/456",
			pr_number: 456,
			git_branch: "feat/two",
		};
		renderTopBar({
			...chat,
			diff_statuses: [primary, secondary],
		});

		await user.click(await screen.findByRole("button", { name: /2 PRs/ }));
		const menu = await screen.findByRole("menu");

		// Real anchors: middle-click and copy-link work, and the
		// destination is announced.
		expect(
			within(menu).getByRole("menuitem", { name: /PR #123/ }),
		).toHaveAttribute("href", "https://github.com/coder/coder/pull/123");
		expect(
			within(menu).getByRole("menuitem", { name: /PR #456/ }),
		).toHaveAttribute("href", "https://github.com/coder/coder/pull/456");
	});

	it("names the repository when two origins carry the same PR", async () => {
		const user = userEvent.setup();

		// The PR number repeats across repositories, so only the
		// repository names keep the entries apart.
		const forked = {
			...MockChatDiffStatus,
			remote_origin: "https://github.com/coder/other-project.git",
			git_branch: "feat/two",
			url: "https://github.com/coder/other-project/pull/123",
		};
		renderTopBar({
			...MockChat,
			diff_statuses: [MockChatDiffStatus, forked],
		});

		await user.click(screen.getByRole("button", { name: /2 PRs/ }));
		const menu = await screen.findByRole("menu");

		expect(
			within(menu).getByRole("menuitem", { name: /coder\/coder · PR #123/ }),
		).toHaveAttribute("href", "https://github.com/coder/coder/pull/123");
		expect(
			within(menu).getByRole("menuitem", {
				name: /coder\/other-project · PR #123/,
			}),
		).toHaveAttribute(
			"href",
			"https://github.com/coder/other-project/pull/123",
		);
	});
});
