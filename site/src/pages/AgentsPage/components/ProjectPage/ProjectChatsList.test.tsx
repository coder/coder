import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "react-query";
import { MemoryRouter, Outlet, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type { Chat } from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import { MockChatProject } from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import type { AgentsPageOutletContext } from "../../AgentsPageLayout";
import { ProjectChatsList } from "./ProjectChatsList";

vi.mock("#/hooks/useAuthenticated", async () => {
	const { MockUserOwner } = await import("#/testHelpers/entities");
	return {
		useAuthenticated: () => ({ user: MockUserOwner, permissions: {} }),
	};
});

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
	requestArchiveAgent: vi.fn(),
	requestUnarchiveAgent: vi.fn(),
	requestArchiveAndDeleteWorkspace: vi.fn(),
	requestPinAgent: vi.fn(),
	requestUnpinAgent: vi.fn(),
	isArchiving: false,
	archivingChatId: undefined,
	activeChatChildren: undefined,
	isSidebarCollapsed: false,
	onToggleSidebarCollapsed: vi.fn(),
	onExpandSidebar: vi.fn(),
	onChatReady: vi.fn(),
});

const renderList = (outletContext = buildOutletContext()) => {
	render(
		<QueryClientProvider client={createTestQueryClient()}>
			<MemoryRouter initialEntries={["/agents/projects/project"]}>
				<Routes>
					<Route element={<Outlet context={outletContext} />}>
						<Route
							path="/agents/projects/project"
							element={<ProjectChatsList project={MockChatProject} />}
						/>
					</Route>
				</Routes>
			</MemoryRouter>
		</QueryClientProvider>,
	);
	return outletContext;
};

describe("ProjectChatsList", () => {
	beforeEach(() => {
		intersectionCallback = undefined;
		vi.stubGlobal("IntersectionObserver", MockIntersectionObserver);
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

		await screen.findByRole("heading", { name: "50+ Chats" });
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

	it("archives a chat through the agents page", async () => {
		const user = userEvent.setup();
		vi.spyOn(API.experimental, "getChats").mockResolvedValue(buildChats(1));

		const outletContext = renderList();

		await user.click(
			await screen.findByRole("button", {
				name: "Open chat actions for Chat 0",
			}),
		);
		await user.click(
			await screen.findByRole("menuitem", { name: "Archive agent" }),
		);

		expect(outletContext.requestArchiveAgent).toHaveBeenCalledWith("chat-0");
	});
});
