import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "react-query";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
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

describe("ChatTopBar", () => {
	beforeEach(() => {
		vi.spyOn(API, "checkAuthorization").mockResolvedValue({});
		vi.spyOn(API.experimental, "getChatProjects").mockResolvedValue([
			MockChatProject,
		]);
	});

	it("links a project chat to its project page, keeping the search", async () => {
		const user = userEvent.setup();
		render(
			<QueryClientProvider client={createTestQueryClient()}>
				<MemoryRouter initialEntries={["/agents/chat-1?archived=archived"]}>
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
										chat={{ ...MockChat, project_id: MockChatProject.id }}
										panel={{
											showSidebarPanel: false,
											onToggleSidebar: vi.fn(),
										}}
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

		await user.click(
			await screen.findByRole("link", { name: MockChatProject.name }),
		);

		expect(await screen.findByTestId("location")).toHaveTextContent(
			`/agents/projects/${MockChatProject.id}?archived=archived`,
		);
	});
});
