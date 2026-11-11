import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useRef } from "react";
import { QueryClientProvider } from "react-query";
import { MemoryRouter, useLocation } from "react-router";
import { afterEach, describe, expect, it, onTestFinished, vi } from "vitest";
import { API } from "#/api/api";
import { chatProjectSummariesKey } from "#/api/queries/chatProjects";
import { chatEntityKey } from "#/api/queries/chats";
import { MockChat } from "#/testHelpers/chatEntities";
import {
	MockChatProject,
	MockChatProjectSummary,
} from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { focusFirstVisible } from "../utils/focus";
import { ChatActionsMenu } from "./ChatActionsMenuItems";
import { useMoveChatToProject } from "./MoveToProjectMenuItems";
import {
	MoveToUnpinnedProjectDialogProvider,
	useMoveToUnpinnedProjectDialog,
} from "./MoveToUnpinnedProjectDialog";

const MoveButton: React.FC<{
	withChatActions: boolean;
	withHiddenChatActions: boolean;
	withHiddenProjects: boolean;
}> = ({ withChatActions, withHiddenChatActions, withHiddenProjects }) => {
	const move = useMoveChatToProject();
	const location = useLocation();
	const dialog = useMoveToUnpinnedProjectDialog();
	const projectsRef = useRef<HTMLButtonElement | null>(null);
	const filterRef = useRef<HTMLButtonElement | null>(null);
	return (
		<>
			{/* The button leaves after the move, like the chat's sidebar row. */}
			{!move.isSuccess && (
				<button
					type="button"
					onClick={() =>
						move.mutate({ chatId: MockChat.id, projectId: MockChatProject.id })
					}
				>
					Move
				</button>
			)}
			{/* Stands in for the sidebar that a small screen hides behind the chat. */}
			{withHiddenChatActions && (
				<div hidden>
					<ChatActionsMenu
						chat={MockChat}
						canManage
						hasWorkspace={false}
						onArchiveAgent={vi.fn()}
						onUnarchiveAgent={vi.fn()}
					>
						<button type="button">Open hidden agent actions</button>
					</ChatActionsMenu>
				</div>
			)}
			{withChatActions && (
				<ChatActionsMenu
					chat={MockChat}
					canManage
					hasWorkspace={false}
					onArchiveAgent={vi.fn()}
					onUnarchiveAgent={vi.fn()}
				>
					<button type="button">Open agent actions</button>
				</ChatActionsMenu>
			)}
			{/* Stands in for the sidebar's Projects link, which a small screen
			    hides, and its chat filters. */}
			<div hidden={withHiddenProjects}>
				<button
					type="button"
					ref={(element) => {
						projectsRef.current = element;
						dialog?.setFocusFallback(
							element
								? () =>
										focusFirstVisible([projectsRef.current, filterRef.current])
								: undefined,
						);
					}}
				>
					Projects
				</button>
			</div>
			<button type="button" ref={filterRef}>
				Filter agents
			</button>
			<output aria-label="Location">{location.pathname}</output>
		</>
	);
};

const moveChat = async ({
	chatPinned = false,
	projectPinned = false,
	fromUnpinnedProject = false,
	withChatActions = false,
	withHiddenChatActions = false,
	withHiddenProjects = false,
	summariesCached = true,
}) => {
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
		project_id: fromUnpinnedProject ? "previous-project" : undefined,
	});
	const summaries = [
		{
			...MockChatProjectSummary,
			pinned: projectPinned,
			pin_order: projectPinned ? 1 : 0,
		},
		{
			...MockChatProjectSummary,
			project: { ...MockChatProject, id: "previous-project" },
			pinned: false,
			pin_order: 0,
		},
	];
	if (summariesCached) {
		queryClient.setQueryData(chatProjectSummariesKey, summaries);
	} else {
		vi.spyOn(API.experimental, "getChatProjectSummaries").mockResolvedValue(
			summaries,
		);
	}
	render(
		<QueryClientProvider client={queryClient}>
			<MemoryRouter initialEntries={["/agents"]}>
				<MoveToUnpinnedProjectDialogProvider>
					<MoveButton
						withChatActions={withChatActions}
						withHiddenChatActions={withHiddenChatActions}
						withHiddenProjects={withHiddenProjects}
					/>
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

	it("keeps a later dialog open and its pin free while an earlier pin settles", async () => {
		const user = userEvent.setup();
		let settleFirstPin = () => {};
		vi.spyOn(API.experimental, "pinChatProject")
			.mockReturnValueOnce(
				new Promise((resolve) => {
					settleFirstPin = () => resolve(undefined);
				}),
			)
			.mockResolvedValue(undefined);
		const otherProject = { ...MockChatProject, id: "other", name: "Other" };
		const ShowButtons: React.FC = () => {
			const dialog = useMoveToUnpinnedProjectDialog();
			return [MockChatProject, otherProject].map((project) => (
				<button
					key={project.id}
					type="button"
					onClick={() => dialog?.show(project, `chat-${project.id}`)}
				>
					Show {project.name}
				</button>
			));
		};
		const queryClient = createTestQueryClient();
		render(
			<QueryClientProvider client={queryClient}>
				<MemoryRouter>
					<MoveToUnpinnedProjectDialogProvider>
						<ShowButtons />
					</MoveToUnpinnedProjectDialogProvider>
				</MemoryRouter>
			</QueryClientProvider>,
		);

		await user.click(screen.getByRole("button", { name: "Show Launch" }));
		await user.click(screen.getByRole("button", { name: "Pin project" }));
		await user.keyboard("{Escape}");
		await user.click(screen.getByRole("button", { name: "Show Other" }));
		expect(screen.getByRole("button", { name: /Pin project/ })).toBeEnabled();
		settleFirstPin();
		await waitFor(() => expect(queryClient.isMutating()).toBe(0));

		expect(
			screen.getByRole("dialog", { name: "Chat moved to Other" }),
		).toBeInTheDocument();
	});

	it("opens after a move before the summaries have loaded", async () => {
		await moveChat({ summariesCached: false });
		expect(
			await screen.findByRole("dialog", {
				name: `Chat moved to ${MockChatProject.name}`,
			}),
		).toBeInTheDocument();
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
		{ withChatActions: true, focused: "Open agent actions" },
		{
			withChatActions: true,
			withHiddenChatActions: true,
			focused: "Open agent actions",
		},
		{ withChatActions: false, focused: "Projects" },
		{
			withChatActions: false,
			withHiddenProjects: true,
			focused: "Filter agents",
		},
	])(
		"closes without pinning and returns focus to $focused (%o)",
		async ({
			withChatActions,
			withHiddenChatActions,
			withHiddenProjects,
			focused,
		}) => {
			// jsdom has no layout, so the hidden attribute stands in for the
			// CSS that hides the sidebar.
			Element.prototype.checkVisibility = function () {
				return !this.closest("[hidden]");
			};
			onTestFinished(() => {
				Reflect.deleteProperty(Element.prototype, "checkVisibility");
			});
			const pin = vi.spyOn(API.experimental, "pinChatProject");
			const user = await moveChat({
				withChatActions,
				withHiddenChatActions,
				withHiddenProjects,
			});

			await user.click(await screen.findByRole("button", { name: "Close" }));

			await waitFor(() =>
				expect(screen.getByRole("button", { name: focused })).toHaveFocus(),
			);
			expect(screen.queryByRole("dialog")).toBeNull();
			expect(pin).not.toHaveBeenCalled();
		},
	);

	it.each([
		{ chatPinned: true, projectPinned: false },
		{ chatPinned: false, projectPinned: true },
		{ fromUnpinnedProject: true },
	])(
		"stays closed when the chat stays in or was already out of the sidebar (%o)",
		async (pins) => {
			await moveChat(pins);
			expect(screen.queryByRole("dialog")).toBeNull();
		},
	);
});
