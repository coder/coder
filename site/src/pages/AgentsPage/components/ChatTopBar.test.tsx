import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { Outlet } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { archiveAndDeleteChatKey } from "#/api/queries/chats";
import type { WorkspaceBuild } from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import { createDeferred } from "#/testHelpers/deferred";
import {
	MockWorkspace,
	MockWorkspaceBuildDelete,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { ChatTopBar } from "./ChatTopBar";

const chat = { ...MockChat, workspace_id: "workspace-1" };
const navigateAfterArchive = vi.fn();

const renderTopBar = () =>
	renderWithAuth(<Outlet context={{ navigateAfterArchive }} />, {
		route: `/agents/${chat.id}`,
		path: "/agents",
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
	navigateAfterArchive.mockClear();
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
