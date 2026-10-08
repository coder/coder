import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "react-query";
import { MemoryRouter, Outlet, Route, Routes, useLocation } from "react-router";
import { toast } from "sonner";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type { Chat } from "#/api/typesGenerated";
import { ThemeOverride } from "#/contexts/ThemeProvider";
import { MockChat, mockChatCost } from "#/testHelpers/chatEntities";
import {
	MockChatProject,
	MockUserMember,
	MockUserOwner,
	MockWorkspace,
	MockWorkspaceBuildDelete,
} from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import themes, { DEFAULT_THEME } from "#/theme";
import type { AgentsPageOutletContext } from "../../AgentsPageLayout";
import { ProjectChatsList } from "./ProjectChatsList";

vi.mock("#/hooks/useAuthenticated", async () => {
	const { MockUserOwner } = await import("#/testHelpers/entities");
	return {
		useAuthenticated: () => ({ user: MockUserOwner, permissions: {} }),
	};
});

const featureVisibility = vi.hoisted(() => ({ aibridge: true }));
vi.mock("#/modules/dashboard/useFeatureVisibility", () => ({
	useFeatureVisibility: () => featureVisibility,
}));
type IntersectionCallback = (
	entries: Array<{ isIntersecting: boolean }>,
) => void;
let intersectionCallback: IntersectionCallback | undefined;

class MockIntersectionObserver {
	observe = vi.fn();
	disconnect = vi.fn();
	unobserve = vi.fn();

	constructor(callback: IntersectionCallback) {
		intersectionCallback = callback;
	}
}

const buildChats = (count: number, offset = 0): Chat[] =>
	Array.from({ length: count }, (_, index) => ({
		...MockChat,
		id: `chat-${offset + index}`,
		title: `Chat ${offset + index}`,
	}));

const buildOutletContext = (): AgentsPageOutletContext => ({
	chatErrorReasons: {},
	setChatErrorReason: vi.fn(),
	clearChatErrorReason: vi.fn(),
	navigateAfterArchive: vi.fn(),
	requestPinAgent: vi.fn(),
	requestUnpinAgent: vi.fn(),
	activeChatChildren: undefined,
	isSidebarCollapsed: false,
	onToggleSidebarCollapsed: vi.fn(),
	onExpandSidebar: vi.fn(),
	onChatReady: vi.fn(),
});

const LocationDisplay: React.FC = () => {
	const location = useLocation();
	return <output>{`${location.pathname}${location.search}`}</output>;
};

const renderList = (
	outletContext = buildOutletContext(),
	initialEntry = "/agents/projects/project",
) => {
	render(
		<ThemeOverride theme={themes[DEFAULT_THEME]}>
			<QueryClientProvider client={createTestQueryClient()}>
				<MemoryRouter initialEntries={[initialEntry]}>
					<Routes>
						<Route element={<Outlet context={outletContext} />}>
							<Route
								path="/agents/projects/project"
								element={<ProjectChatsList project={MockChatProject} />}
							/>
						</Route>
						<Route path="/agents/:agentId" element={<LocationDisplay />} />
					</Routes>
				</MemoryRouter>
			</QueryClientProvider>
		</ThemeOverride>,
	);
	return outletContext;
};

describe("ProjectChatsList", () => {
	beforeEach(() => {
		featureVisibility.aibridge = true;
		intersectionCallback = undefined;
		vi.stubGlobal("IntersectionObserver", MockIntersectionObserver);
		vi.spyOn(API.experimental, "getChatCost").mockImplementation(
			async (chatId) => mockChatCost(chatId, 1_230_000),
		);
	});

	afterEach(() => {
		vi.unstubAllGlobals();
		vi.restoreAllMocks();
	});

	it("requests the unarchived chats of the project", async () => {
		const getChats = vi
			.spyOn(API.experimental, "getChats")
			.mockResolvedValue(buildChats(1));

		renderList();

		await waitFor(() => {
			expect(getChats).toHaveBeenCalledWith(
				expect.objectContaining({
					project_id: MockChatProject.id,
					q: "archived:false",
					offset: 0,
				}),
				expect.anything(),
			);
		});
	});

	it("loads the next page when the end of the list scrolls into view", async () => {
		const getChats = vi
			.spyOn(API.experimental, "getChats")
			.mockImplementation(async (request) =>
				request?.offset ? buildChats(1, 50) : buildChats(50),
			);

		renderList();

		await screen.findByRole("link", { name: /Chat 49/ });
		intersectionCallback?.([{ isIntersecting: true }]);

		await waitFor(() => {
			expect(getChats).toHaveBeenCalledWith(
				expect.objectContaining({
					project_id: MockChatProject.id,
					offset: 50,
				}),
				expect.anything(),
			);
		});
	});

	it("shows each chat's cost", async () => {
		vi.spyOn(API.experimental, "getChats").mockResolvedValue(buildChats(1));

		renderList();

		expect(
			await screen.findByRole("link", { name: /Chat 0.*\$1\.23/ }),
		).toBeInTheDocument();
		expect(API.experimental.getChatCost).toHaveBeenCalledWith(
			"chat-0",
			expect.anything(),
		);
	});

	it("hides the cost without the AI Gateway", async () => {
		featureVisibility.aibridge = false;
		vi.spyOn(API.experimental, "getChats").mockResolvedValue(buildChats(1));

		renderList();

		await screen.findByRole("link", { name: /Chat 0/ });
		expect(screen.queryByText("$1.23")).not.toBeInTheDocument();
		expect(API.experimental.getChatCost).not.toHaveBeenCalled();
	});

	it("keeps the sidebar filters when opening a chat", async () => {
		const user = userEvent.setup();
		vi.spyOn(API.experimental, "getChats").mockResolvedValue(buildChats(1));

		renderList(
			buildOutletContext(),
			"/agents/projects/project?archived=archived",
		);

		await user.click(await screen.findByRole("link", { name: /Chat 0/ }));

		expect(await screen.findByRole("status")).toHaveTextContent(
			"/agents/chat-0?archived=archived",
		);
	});

	it.each([MockUserOwner, MockUserMember])(
		"copies a chat ID without a branch (owner: $username)",
		async (owner) => {
			const user = userEvent.setup();
			const writeText = vi
				.spyOn(navigator.clipboard, "writeText")
				.mockResolvedValue();
			vi.spyOn(API.experimental, "getChats").mockResolvedValue([
				{ ...MockChat, owner_id: owner.id },
			]);
			renderList();

			await user.click(
				await screen.findByRole("button", {
					name: `Open chat actions for ${MockChat.title}`,
				}),
			);
			await user.click(
				await screen.findByRole("menuitem", { name: "Copy ID" }),
			);

			await waitFor(() => {
				expect(writeText).toHaveBeenCalledWith(MockChat.id);
			});
		},
	);

	it.each([
		["Copy ID", MockChat.id],
		["Copy branch", "feature/project-chat"],
	])("copies %s from a project chat's submenu", async (label, value) => {
		const user = userEvent.setup();
		const writeText = vi
			.spyOn(navigator.clipboard, "writeText")
			.mockResolvedValue();
		vi.spyOn(API.experimental, "getChats").mockResolvedValue([
			{
				...MockChat,
				diff_status: {
					chat_id: MockChat.id,
					pull_request_title: "",
					pull_request_draft: false,
					changes_requested: false,
					additions: 0,
					deletions: 0,
					changed_files: 0,
					head_branch: "feature/project-chat",
				},
			},
		]);
		renderList();

		await user.click(
			await screen.findByRole("button", {
				name: `Open chat actions for ${MockChat.title}`,
			}),
		);
		await user.click(await screen.findByRole("menuitem", { name: "Copy" }));
		fireEvent.click(await screen.findByRole("menuitem", { name: label }));

		await waitFor(() => {
			expect(writeText).toHaveBeenCalledWith(value);
		});
	});

	it.each([false, true])(
		"archives and deletes a project chat workspace (confirmation: %s)",
		async (requiresConfirmation) => {
			const user = userEvent.setup();
			const chat = { ...MockChat, workspace_id: MockWorkspace.id };
			vi.spyOn(API.experimental, "getChats").mockResolvedValue([chat]);
			vi.spyOn(API, "getWorkspace").mockResolvedValue({
				...MockWorkspace,
				created_at: requiresConfirmation
					? "2000-01-01T00:00:00.000Z"
					: chat.created_at,
			});
			vi.spyOn(API, "getWorkspaceBuilds").mockResolvedValue([]);
			vi.spyOn(API.experimental, "updateChat").mockResolvedValue(undefined);
			vi.spyOn(API, "deleteWorkspace").mockResolvedValue(
				MockWorkspaceBuildDelete,
			);
			const outletContext = renderList();

			await user.click(
				await screen.findByRole("button", {
					name: `Open chat actions for ${chat.title}`,
				}),
			);
			await user.click(
				await screen.findByRole("menuitem", {
					name: "Archive & delete workspace",
				}),
			);
			if (requiresConfirmation) {
				const nameField = await screen.findByLabelText(
					"Name of the workspace to delete",
				);
				expect(API.deleteWorkspace).not.toHaveBeenCalled();
				await user.type(nameField, MockWorkspace.name);
				await user.click(screen.getByRole("button", { name: "Delete" }));
			}

			await waitFor(() => {
				expect(API.deleteWorkspace).toHaveBeenCalledWith(MockWorkspace.id);
				expect(outletContext.navigateAfterArchive).toHaveBeenCalledWith(
					chat.id,
				);
			});
			expect(API.experimental.updateChat).toHaveBeenCalledWith(chat.id, {
				archived: true,
			});
		},
	);

	it("restores a project row and preserves its error when archiving fails", async () => {
		const user = userEvent.setup();
		const getChats = vi
			.spyOn(API.experimental, "getChats")
			.mockResolvedValue(buildChats(1));
		let rejectArchive!: (error: Error) => void;
		vi.spyOn(API.experimental, "updateChat").mockImplementation(
			() =>
				new Promise((_resolve, reject) => {
					rejectArchive = reject;
				}),
		);
		const errorToast = vi.spyOn(toast, "error");
		const outletContext = renderList();

		await user.click(
			await screen.findByRole("button", {
				name: "Open chat actions for Chat 0",
			}),
		);
		await user.click(
			await screen.findByRole("menuitem", { name: "Archive agent" }),
		);
		await waitFor(() => {
			expect(API.experimental.updateChat).toHaveBeenCalledWith("chat-0", {
				archived: true,
			});
			expect(
				screen.queryByRole("link", { name: /Chat 0/ }),
			).not.toBeInTheDocument();
		});
		getChats.mockRejectedValue(new Error("Refetch failed"));
		rejectArchive(new Error("Archive failed"));

		expect(
			await screen.findByRole("link", { name: /Chat 0/ }),
		).toBeInTheDocument();
		await waitFor(() =>
			expect(errorToast).toHaveBeenCalledWith("Archive failed"),
		);
		expect(outletContext.clearChatErrorReason).not.toHaveBeenCalled();
		expect(outletContext.navigateAfterArchive).not.toHaveBeenCalled();
	});

	it("archives a chat from its project row", async () => {
		const user = userEvent.setup();
		vi.spyOn(API.experimental, "getChats").mockResolvedValue(buildChats(1));
		vi.spyOn(API.experimental, "updateChat").mockResolvedValue(undefined);

		const outletContext = renderList();

		await user.click(
			await screen.findByRole("button", {
				name: "Open chat actions for Chat 0",
			}),
		);
		await user.click(
			await screen.findByRole("menuitem", { name: "Archive agent" }),
		);

		await waitFor(() => {
			expect(API.experimental.updateChat).toHaveBeenCalledWith("chat-0", {
				archived: true,
			});
			expect(outletContext.clearChatErrorReason).toHaveBeenCalledWith("chat-0");
		});
		expect(outletContext.navigateAfterArchive).not.toHaveBeenCalled();
	});
});
