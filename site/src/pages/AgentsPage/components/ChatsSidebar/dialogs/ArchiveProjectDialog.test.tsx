import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type { ChatProjectArchivePreview } from "#/api/typesGenerated";
import { toast } from "#/components/Toaster/toast";
import { createDeferred } from "#/testHelpers/deferred";
import {
	MockArchivedChatProject,
	MockChatProject,
	mockApiError,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { ArchiveProjectDialog } from "./ArchiveProjectDialog";

const mockApi = ({
	preview = { chat_count: 2, active_chat_count: 0 },
}: {
	preview?: ChatProjectArchivePreview;
} = {}) => {
	vi.spyOn(API.experimental, "getChatProjectArchivePreview").mockResolvedValue(
		preview,
	);
	vi.spyOn(API.experimental, "getChatRetentionDays").mockResolvedValue({
		retention_days: 30,
	});
	const archive = vi
		.spyOn(API.experimental, "archiveChatProject")
		.mockResolvedValue(MockArchivedChatProject);
	return { archive };
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
			// A refetch would show the emptied project while the dialog closes.
			expect(
				API.experimental.getChatProjectArchivePreview,
			).toHaveBeenCalledTimes(1);
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
			expect(API.experimental.getChatRetentionDays).toHaveBeenCalled(),
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
	});
});
