import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "react-query";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import { DashboardContext } from "#/modules/dashboard/DashboardProvider";
import { MockChat } from "#/testHelpers/chatEntities";
import {
	MockAppearanceConfig,
	MockBuildInfo,
	MockChatProject,
	MockDefaultOrganization,
	MockEntitlements,
	MockUserOwner,
} from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { ChatTopBar } from "./ChatTopBar";

vi.mock("#/hooks/useAuthenticated", () => ({
	useAuthenticated: () => ({
		user: MockUserOwner,
		permissions: {},
		signOut: vi.fn(),
	}),
}));

const LocationProbe: React.FC = () => {
	const location = useLocation();
	return (
		<div data-testid="location">{`${location.pathname}${location.search}`}</div>
	);
};

const renderTopBar = (chat: TypesGen.Chat) =>
	render(
		<QueryClientProvider client={createTestQueryClient()}>
			<MemoryRouter initialEntries={[`/agents/${chat.id}?archived=archived`]}>
				<DashboardContext.Provider
					value={{
						entitlements: MockEntitlements,
						experiments: ["chat-projects"],
						appearance: MockAppearanceConfig,
						buildInfo: MockBuildInfo,
						organizations: [MockDefaultOrganization],
						showOrganizations: false,
						canViewOrganizationSettings: false,
					}}
				>
					<Routes>
						<Route
							path="/agents/:agentId"
							element={
								<ChatTopBar
									chat={chat}
									panel={{ showSidebarPanel: false, onToggleSidebar: vi.fn() }}
								/>
							}
						/>
						<Route
							path="/agents/projects/:projectId"
							element={<LocationProbe />}
						/>
					</Routes>
				</DashboardContext.Provider>
			</MemoryRouter>
		</QueryClientProvider>,
	);

const expectProjectLinkNavigates = async () => {
	const user = userEvent.setup();
	await user.click(
		await screen.findByRole("link", { name: MockChatProject.name }),
	);
	expect(await screen.findByTestId("location")).toHaveTextContent(
		`/agents/projects/${MockChatProject.id}?archived=archived`,
	);
};

describe("ChatTopBar", () => {
	beforeEach(() => {
		vi.spyOn(API, "checkAuthorization").mockResolvedValue({});
		vi.spyOn(API.experimental, "getChatProjects").mockResolvedValue([
			MockChatProject,
		]);
	});

	it("links a project chat to its project page, keeping the search", async () => {
		renderTopBar({ ...MockChat, project_id: MockChatProject.id });
		await expectProjectLinkNavigates();
	});

	it("links a nested delegated chat to its root chat's project", async () => {
		const rootChat = {
			...MockChat,
			id: "root-chat",
			project_id: MockChatProject.id,
		};
		// Delegated chats do not store project_id, so the parent has none.
		const parentChat = {
			...MockChat,
			id: "parent-chat",
			parent_chat_id: rootChat.id,
			root_chat_id: rootChat.id,
		};
		const chatsById = new Map<string, TypesGen.Chat>([
			[rootChat.id, rootChat],
			[parentChat.id, parentChat],
		]);
		vi.spyOn(API.experimental, "getChat").mockImplementation(async (id) => {
			const chat = chatsById.get(id);
			if (!chat) {
				throw new Error(`unexpected chat ${id}`);
			}
			return chat;
		});
		renderTopBar({
			...MockChat,
			id: "child-chat",
			parent_chat_id: parentChat.id,
			root_chat_id: rootChat.id,
		});
		await expectProjectLinkNavigates();
	});
});
