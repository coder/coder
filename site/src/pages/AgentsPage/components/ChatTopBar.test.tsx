import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { FC, PropsWithChildren } from "react";
import { QueryClientProvider } from "react-query";
import { MemoryRouter, Outlet, Route, Routes, useLocation } from "react-router";
import { describe, expect, it } from "vitest";
import { DashboardContext } from "#/modules/dashboard/DashboardProvider";
import { MockChat } from "#/testHelpers/chatEntities";
import {
	MockAppearanceConfig,
	MockBuildInfo,
	MockDefaultOrganization,
	MockEntitlements,
} from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { ChatTopBar } from "./ChatTopBar";

const LocationProbe: FC = () => {
	const location = useLocation();
	return <output aria-label="Location">{location.pathname}</output>;
};

const Wrapper: FC<PropsWithChildren> = ({ children }) => {
	const queryClient = createTestQueryClient();
	return (
		<QueryClientProvider client={queryClient}>
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
				<MemoryRouter initialEntries={[`/agents/${MockChat.id}`]}>
					<Routes>
						<Route element={<Outlet />}>
							<Route path="/agents/:agentId" element={children} />
							<Route
								path="/agents/settings/memory"
								element={<LocationProbe />}
							/>
						</Route>
					</Routes>
				</MemoryRouter>
			</DashboardContext.Provider>
		</QueryClientProvider>
	);
};

describe("ChatTopBar", () => {
	it("navigates root chats to personal memory settings", async () => {
		const user = userEvent.setup();
		render(
			<Wrapper>
				<ChatTopBar
					chat={MockChat}
					panel={{ showSidebarPanel: false, onToggleSidebar: () => {} }}
				/>
			</Wrapper>,
		);

		await user.click(
			screen.getByRole("button", { name: "Open agent actions" }),
		);
		await user.click(screen.getByRole("menuitem", { name: "Memory" }));

		expect(screen.getByRole("status", { name: "Location" })).toHaveTextContent(
			"/agents/settings/memory",
		);
	});

	it("omits personal memory from project chat actions", async () => {
		const user = userEvent.setup();
		render(
			<Wrapper>
				<ChatTopBar
					chat={{ ...MockChat, project_id: "project-1" }}
					panel={{ showSidebarPanel: false, onToggleSidebar: () => {} }}
				/>
			</Wrapper>,
		);

		await user.click(
			screen.getByRole("button", { name: "Open agent actions" }),
		);

		expect(screen.queryByRole("menuitem", { name: "Memory" })).toBeNull();
	});
});
