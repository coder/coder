import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type {
	ChatProjectArchivePreview,
	Workspace,
} from "#/api/typesGenerated";
import { toast } from "#/components/Toaster/toast";
import { MockChat } from "#/testHelpers/chatEntities";
import {
	MockArchivedChatProject,
	MockChatProject,
	MockStoppedWorkspace,
	MockWorkspace,
	MockWorkspaceBuildDelete,
	mockApiError,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { ArchiveProjectDialog } from "./ArchiveProjectDialog";

const chatCreatedAt = "2026-10-01T00:00:00Z";
// Created after the chats, so each counts as created by its chat.
const workspaceCreatedAt = "2026-10-02T00:00:00Z";

const mockWorkspace = (id: string): Workspace => ({
	...MockWorkspace,
	id,
	name: id,
	created_at: workspaceCreatedAt,
});
const mockRunningWorkspace = mockWorkspace("running");
const mockInUseWorkspace = mockWorkspace("in-use");
const mockStoppedWorkspace: Workspace = {
	...mockWorkspace("stopped"),
	latest_build: MockStoppedWorkspace.latest_build,
};
const mockDeletableWorkspace = mockWorkspace("deletable");
const mockFailingWorkspace = mockWorkspace("failing");

const mockApi = ({
	preview = { chat_count: 2, active_chat_count: 0 },
	workspaces = [],
	workspacesInUse = [],
}: {
	preview?: ChatProjectArchivePreview;
	workspaces?: Workspace[];
	workspacesInUse?: string[];
} = {}) => {
	vi.spyOn(API.experimental, "getChatProjectArchivePreview").mockResolvedValue(
		preview,
	);
	vi.spyOn(API.experimental, "getChatRetentionDays").mockResolvedValue({
		retention_days: 30,
	});
	vi.spyOn(API.experimental, "getChats").mockResolvedValue(
		workspaces.map((ws, index) => ({
			...MockChat,
			id: `chat-${index}`,
			created_at: chatCreatedAt,
			project_id: MockChatProject.id,
			workspace_id: ws.id,
		})),
	);
	vi.spyOn(API, "getWorkspace").mockImplementation(async (id) => {
		const found = workspaces.find((ws) => ws.id === id);
		if (!found) {
			throw new Error(`unexpected workspace ${id}`);
		}
		return found;
	});
	vi.spyOn(API, "getWorkspaceBuilds").mockResolvedValue([]);
	vi.spyOn(API.experimental, "getChatsByWorkspace").mockResolvedValue(
		Object.fromEntries(workspacesInUse.map((id) => [id, "other-chat"])),
	);
	const archive = vi
		.spyOn(API.experimental, "archiveChatProject")
		.mockResolvedValue(MockArchivedChatProject);
	const stop = vi
		.spyOn(API, "stopWorkspace")
		.mockResolvedValue(MockWorkspaceBuildDelete);
	const remove = vi
		.spyOn(API, "deleteWorkspace")
		.mockResolvedValue(MockWorkspaceBuildDelete);
	return { archive, stop, remove };
};

const renderDialog = async () => {
	const onClose = vi.fn();
	renderWithAuth(
		<ArchiveProjectDialog project={MockChatProject} open onClose={onClose} />,
	);
	// The confirm button waits for the preview.
	await screen.findByText(/will be archived/);
	return { onClose };
};

afterEach(() => {
	vi.restoreAllMocks();
});

describe("ArchiveProjectDialog", () => {
	it.each([
		{ activeChats: 0, button: "Archive project", stopActiveChats: false },
		{ activeChats: 2, button: "Stop chats and archive", stopActiveChats: true },
	])(
		"confirms with $button, sending stop_active_chats: $stopActiveChats",
		async ({ activeChats, button, stopActiveChats }) => {
			const user = userEvent.setup();
			const { archive } = mockApi({
				preview: { chat_count: 3, active_chat_count: activeChats },
			});
			const { onClose } = await renderDialog();

			await user.click(screen.getByRole("button", { name: button }));

			await waitFor(() => expect(onClose).toHaveBeenCalled());
			expect(archive).toHaveBeenCalledWith(
				MockChatProject.organization_id,
				MockChatProject.id,
				{ stop_active_chats: stopActiveChats },
			);
		},
	);

	it("asks to stop a chat that started after the preview loaded", async () => {
		const user = userEvent.setup();
		const { archive } = mockApi({
			preview: { chat_count: 2, active_chat_count: 1 },
		});
		vi.mocked(
			API.experimental.getChatProjectArchivePreview,
		).mockResolvedValueOnce({ chat_count: 2, active_chat_count: 0 });
		archive.mockRejectedValueOnce({
			...mockApiError({ message: "Chat project has 1 active chat." }),
			status: 409,
		});
		const errorToast = vi.spyOn(toast, "error");
		await renderDialog();

		await user.click(screen.getByRole("button", { name: "Archive project" }));
		await user.click(
			await screen.findByRole("button", { name: "Stop chats and archive" }),
		);

		await waitFor(() =>
			expect(archive).toHaveBeenLastCalledWith(
				MockChatProject.organization_id,
				MockChatProject.id,
				{ stop_active_chats: true },
			),
		);
		expect(errorToast).not.toHaveBeenCalled();
	});

	it("stops running workspaces no other chat uses after archiving", async () => {
		const user = userEvent.setup();
		const { stop } = mockApi({
			workspaces: [
				mockRunningWorkspace,
				mockInUseWorkspace,
				mockStoppedWorkspace,
			],
			workspacesInUse: [mockInUseWorkspace.id],
		});
		const warningToast = vi.spyOn(toast, "warning");
		await renderDialog();

		await user.click(await screen.findByRole("radio", { name: "Stop them" }));
		await user.click(screen.getByRole("button", { name: "Archive project" }));

		await waitFor(() => expect(warningToast).toHaveBeenCalled());
		expect(stop.mock.calls).toEqual([[mockRunningWorkspace.id]]);
	});

	it("archives once and still stops workspaces when confirm is double-clicked", async () => {
		// Without a delay, the second click lands before the button disables.
		const user = userEvent.setup({ delay: null });
		const { archive, stop } = mockApi({ workspaces: [mockRunningWorkspace] });
		const successToast = vi.spyOn(toast, "success");
		const { onClose } = await renderDialog();

		await user.click(await screen.findByRole("radio", { name: "Stop them" }));
		await user.dblClick(
			screen.getByRole("button", { name: "Archive project" }),
		);

		await waitFor(() => expect(stop).toHaveBeenCalled());
		expect(archive).toHaveBeenCalledTimes(1);
		expect(onClose).toHaveBeenCalled();
		expect(successToast).toHaveBeenCalledWith("Project archived");
	});

	it("keeps focus in the options when Delete is chosen with the keyboard", async () => {
		const user = userEvent.setup();
		mockApi({ workspaces: [mockRunningWorkspace] });
		await renderDialog();

		await user.click(await screen.findByRole("radio", { name: "Stop them" }));
		// Radix moves focus in a timeout and checks the option only while
		// the arrow key is still down.
		await user.keyboard("{ArrowDown>}");
		await user.keyboard("{/ArrowDown}");

		const deleteOption = screen.getByRole("radio", { name: "Delete them" });
		await waitFor(() => expect(deleteOption).toBeChecked());
		expect(screen.getByRole("textbox")).not.toHaveFocus();
		expect(deleteOption).toHaveFocus();
	});

	it("leaves every workspace alone when it can't tell which are still in use", async () => {
		const user = userEvent.setup();
		const { stop } = mockApi({ workspaces: [mockRunningWorkspace] });
		vi.mocked(API.experimental.getChatsByWorkspace).mockRejectedValue(
			mockApiError({ message: "Lookup failed" }),
		);
		const errorToast = vi.spyOn(toast, "error");
		await renderDialog();

		await user.click(await screen.findByRole("radio", { name: "Stop them" }));
		await user.click(screen.getByRole("button", { name: "Archive project" }));

		await waitFor(() => expect(errorToast).toHaveBeenCalled());
		expect(stop).not.toHaveBeenCalled();
	});

	it("deletes workspaces only once the project name is typed, and reports failures", async () => {
		const user = userEvent.setup();
		const { archive, remove } = mockApi({
			workspaces: [mockDeletableWorkspace, mockFailingWorkspace],
		});
		remove.mockImplementation(async (id) => {
			if (id === mockFailingWorkspace.id) {
				throw mockApiError({ message: "Delete refused" });
			}
			return MockWorkspaceBuildDelete;
		});
		const errorToast = vi.spyOn(toast, "error");
		await renderDialog();

		await user.click(await screen.findByRole("radio", { name: "Delete them" }));
		await user.click(screen.getByRole("button", { name: "Archive project" }));
		expect(archive).not.toHaveBeenCalled();

		await user.type(screen.getByRole("textbox"), MockChatProject.name);
		await user.click(screen.getByRole("button", { name: "Archive project" }));

		await waitFor(() => expect(errorToast).toHaveBeenCalled());
		expect(remove.mock.calls.map(([id]) => id).sort()).toEqual([
			mockDeletableWorkspace.id,
			mockFailingWorkspace.id,
		]);
		expect(errorToast.mock.calls[0][0]).toMatch(
			/1 workspace could not be deleted/,
		);
	});
});
