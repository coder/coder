import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "react-query";
import { MemoryRouter, useLocation } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { chatProjectSummariesKey } from "#/api/queries/chatProjects";
import { chatEntityKey } from "#/api/queries/chats";
import { MockChat } from "#/testHelpers/chatEntities";
import {
	MockChatProject,
	MockChatProjectSummary,
} from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { useMoveChatToProject } from "./MoveToProjectMenuItems";
import { MoveToUnpinnedProjectDialogProvider } from "./MoveToUnpinnedProjectDialog";

const MoveButton: React.FC = () => {
	const move = useMoveChatToProject();
	const location = useLocation();
	return (
		<>
			<button
				type="button"
				onClick={() =>
					move.mutate({ chatId: MockChat.id, projectId: MockChatProject.id })
				}
			>
				Move
			</button>
			<output aria-label="Location">{location.pathname}</output>
		</>
	);
};

const moveChat = async ({ chatPinned = false, projectPinned = false }) => {
	const user = userEvent.setup();
	vi.spyOn(API.experimental, "updateChat").mockResolvedValue(undefined);
	const queryClient = createTestQueryClient();
	// The seeded caches stand in for the sidebar, whose observers keep them.
	queryClient.setDefaultOptions({
		queries: { gcTime: Number.POSITIVE_INFINITY },
	});
	queryClient.setQueryData(chatEntityKey(MockChat.id), {
		...MockChat,
		pin_order: chatPinned ? 1 : 0,
	});
	queryClient.setQueryData(chatProjectSummariesKey, [
		{
			...MockChatProjectSummary,
			pinned: projectPinned,
			pin_order: projectPinned ? 1 : 0,
		},
	]);
	render(
		<QueryClientProvider client={queryClient}>
			<MemoryRouter initialEntries={["/agents"]}>
				<MoveToUnpinnedProjectDialogProvider>
					<MoveButton />
				</MoveToUnpinnedProjectDialogProvider>
			</MemoryRouter>
		</QueryClientProvider>,
	);
	await user.click(screen.getByRole("button", { name: "Move" }));
	await waitFor(() => expect(queryClient.isMutating()).toBe(0));
	return user;
};

describe("MoveToUnpinnedProjectDialog", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("pins the project the chat moved into", async () => {
		const pin = vi
			.spyOn(API.experimental, "pinChatProject")
			.mockResolvedValue(undefined);
		const user = await moveChat({});

		await user.click(
			await screen.findByRole("button", { name: "Pin project" }),
		);

		await waitFor(() =>
			expect(pin).toHaveBeenCalledWith(
				MockChatProject.organization_id,
				MockChatProject.id,
				undefined,
			),
		);
	});

	it("goes to the project's page", async () => {
		const user = await moveChat({});

		await user.click(
			await screen.findByRole("button", { name: "Go to project" }),
		);

		expect(screen.getByRole("status", { name: "Location" })).toHaveTextContent(
			`/agents/projects/${MockChatProject.id}`,
		);
	});

	it.each([
		{ chatPinned: true, projectPinned: false },
		{ chatPinned: false, projectPinned: true },
	])(
		"stays closed when a pin keeps the chat in the sidebar (%o)",
		async (pins) => {
			await moveChat(pins);
			expect(screen.queryByRole("dialog")).toBeNull();
		},
	);
});
