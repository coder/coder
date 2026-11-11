import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type {
	Chat,
	ChatProjectArchivePreview,
	Workspace,
} from "#/api/typesGenerated";
import { toast } from "#/components/Toaster/toast";
import { MockChat } from "#/testHelpers/chatEntities";
import { createDeferred } from "#/testHelpers/deferred";
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
	// The confirm button waits for the preview and the workspace list.
	await screen.findByText(/will be archived/);
	await waitFor(() =>
		expect(
			screen.getByRole("button", {
				name: /^(Archive project|Stop chats? and archive)$/,
			}),
		).toBeEnabled(),
	);
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
			// A refetch would show the emptied project while the dialog closes.
			expect(
				API.experimental.getChatProjectArchivePreview,
			).toHaveBeenCalledTimes(1);
			expect(API.experimental.getChats).toHaveBeenCalledTimes(1);
		},
	);

	it("archives a project without chats when the retention period fails to load", async () => {
		const user = userEvent.setup();
		const { archive } = mockApi({
			preview: { chat_count: 0, active_chat_count: 0 },
		});
		vi.mocked(API.experimental.getChatRetentionDays).mockRejectedValue(
			mockApiError({ message: "Failed to load the retention period." }),
		);
		renderWithAuth(
			<ArchiveProjectDialog project={MockChatProject} open onClose={vi.fn()} />,
		);
		await screen.findByText(/has no chats to archive/);
		await waitFor(() =>
			expect(
				screen.getByRole("button", { name: "Archive project" }),
			).toBeEnabled(),
		);

		await user.click(screen.getByRole("button", { name: "Archive project" }));

		await waitFor(() => expect(archive).toHaveBeenCalledTimes(1));
	});

	it("waits for the refreshed preview before archiving again after a 409", async () => {
		const user = userEvent.setup();
		const { archive } = mockApi();
		const refreshedPreview = createDeferred<ChatProjectArchivePreview>();
		vi.mocked(API.experimental.getChatProjectArchivePreview)
			.mockResolvedValueOnce({ chat_count: 2, active_chat_count: 0 })
			.mockReturnValueOnce(refreshedPreview.promise);
		archive.mockRejectedValueOnce({
			...mockApiError({ message: "Chat project has 1 active chat." }),
			status: 409,
		});
		await renderDialog();

		await user.click(screen.getByRole("button", { name: "Archive project" }));
		await waitFor(() =>
			expect(
				API.experimental.getChatProjectArchivePreview,
			).toHaveBeenCalledTimes(2),
		);
		await user.click(screen.getByRole("button", { name: "Archive project" }));

		expect(archive).toHaveBeenCalledTimes(1);
		refreshedPreview.resolve({ chat_count: 2, active_chat_count: 1 });
	});

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
			await screen.findByRole("button", { name: "Stop chat and archive" }),
		);

		await waitFor(() =>
			expect(archive).toHaveBeenLastCalledWith(
				MockChatProject.organization_id,
				MockChatProject.id,
				{ stop_active_chats: true },
			),
		);
		expect(errorToast).not.toHaveBeenCalled();
		// The new chat may use a workspace the first list didn't have.
		expect(API.experimental.getChats).toHaveBeenCalledTimes(2);
	});

	it("stops workspaces running at archive time that no other chat uses", async () => {
		const user = userEvent.setup();
		const { stop } = mockApi({
			workspaces: [
				mockRunningWorkspace,
				mockInUseWorkspace,
				mockStoppedWorkspace,
			],
			workspacesInUse: [mockInUseWorkspace.id],
		});
		const successToast = vi.spyOn(toast, "success");
		const warningToast = vi.spyOn(toast, "warning");
		await renderDialog();

		await user.click(await screen.findByRole("radio", { name: "Stop them" }));
		// Both workspaces change state while the dialog is open: the running
		// one stops and the stopped one starts. Only the one running at
		// archive time is stopped.
		vi.mocked(API.getWorkspace).mockImplementation(async (id) => {
			switch (id) {
				case mockRunningWorkspace.id:
					return {
						...mockRunningWorkspace,
						latest_build: mockStoppedWorkspace.latest_build,
					};
				case mockStoppedWorkspace.id:
					return {
						...mockStoppedWorkspace,
						latest_build: mockRunningWorkspace.latest_build,
					};
				case mockInUseWorkspace.id:
					return mockInUseWorkspace;
				default:
					throw new Error(`unexpected workspace ${id}`);
			}
		});
		await user.click(screen.getByRole("button", { name: "Archive project" }));

		await waitFor(() => expect(warningToast).toHaveBeenCalled());
		expect(stop.mock.calls).toEqual([[mockStoppedWorkspace.id]]);
		// The skip replaces the archived toast instead of stacking under it.
		expect(successToast).toHaveBeenCalledWith("Project archived");
		expect(warningToast).toHaveBeenCalledWith(
			expect.anything(),
			expect.objectContaining({
				id: successToast.mock.results[0]?.value,
				duration: Number.POSITIVE_INFINITY,
			}),
		);
	});

	it("waits for the workspace list before archiving", async () => {
		const user = userEvent.setup();
		const { archive } = mockApi();
		const chats = createDeferred<Chat[]>();
		vi.mocked(API.experimental.getChats).mockReturnValue(chats.promise);
		renderWithAuth(
			<ArchiveProjectDialog project={MockChatProject} open onClose={vi.fn()} />,
		);
		await screen.findByText(/will be archived/);

		await user.click(screen.getByRole("button", { name: "Archive project" }));
		expect(archive).not.toHaveBeenCalled();

		chats.resolve([]);
		await waitFor(() =>
			expect(
				screen.getByRole("button", { name: "Archive project" }),
			).toBeEnabled(),
		);
		await user.click(screen.getByRole("button", { name: "Archive project" }));
		await waitFor(() => expect(archive).toHaveBeenCalledTimes(1));
	});

	it("reports each archive's skipped workspaces in that archive's toast", async () => {
		const user = userEvent.setup();
		mockApi({
			workspaces: [mockInUseWorkspace],
			workspacesInUse: [mockInUseWorkspace.id],
		});
		const usageLookup = createDeferred<Record<string, string>>();
		vi.mocked(API.experimental.getChatsByWorkspace).mockReturnValue(
			usageLookup.promise,
		);
		const successToast = vi.spyOn(toast, "success");
		const warningToast = vi.spyOn(toast, "warning");
		const mockOtherProject = {
			...MockChatProject,
			id: "other-project",
			name: "Other",
		};
		// Archiving the first project opens the dialog for the second.
		const Harness: React.FC = () => {
			const [project, setProject] = useState(MockChatProject);
			return (
				<ArchiveProjectDialog
					project={project}
					open
					onClose={() => setProject(mockOtherProject)}
				/>
			);
		};
		const archiveWithStop = async () => {
			await user.click(await screen.findByRole("radio", { name: "Stop them" }));
			await waitFor(() =>
				expect(
					screen.getByRole("button", { name: "Archive project" }),
				).toBeEnabled(),
			);
			await user.click(screen.getByRole("button", { name: "Archive project" }));
		};
		renderWithAuth(<Harness />);

		await archiveWithStop();
		await waitFor(() => expect(successToast).toHaveBeenCalledTimes(1));
		await archiveWithStop();
		await waitFor(() => expect(successToast).toHaveBeenCalledTimes(2));
		usageLookup.resolve({ [mockInUseWorkspace.id]: "other-chat" });

		await waitFor(() => expect(warningToast).toHaveBeenCalledTimes(2));
		expect(warningToast.mock.calls.map(([, data]) => data?.id)).toEqual(
			successToast.mock.results.map(({ value }) => value),
		);
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
