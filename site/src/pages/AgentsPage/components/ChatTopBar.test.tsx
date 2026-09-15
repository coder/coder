import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { type FC, type PropsWithChildren, useEffect } from "react";
import { QueryClientProvider } from "react-query";
import { MemoryRouter, Outlet, Route, Routes, useLocation } from "react-router";
import { describe, expect, it, vi } from "vitest";
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

const LocationProbe: FC<
	PropsWithChildren<{ readonly onLocationChange: (pathname: string) => void }>
> = ({ children, onLocationChange }) => {
	const location = useLocation();
	useEffect(() => {
		onLocationChange(location.pathname);
	}, [location.pathname, onLocationChange]);
	return children;
};

const Wrapper: FC<
	PropsWithChildren<{ readonly onLocationChange: (pathname: string) => void }>
> = ({ children, onLocationChange }) => {
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
							<Route
								path="/agents/:agentId"
								element={
									<LocationProbe onLocationChange={onLocationChange}>
										{children}
									</LocationProbe>
								}
							/>
							<Route
								path="/agents/settings/memory"
								element={<LocationProbe onLocationChange={onLocationChange} />}
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
		const onLocationChange = vi.fn();
		render(
			<Wrapper onLocationChange={onLocationChange}>
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

		await waitFor(() => {
			expect(onLocationChange).toHaveBeenLastCalledWith(
				"/agents/settings/memory",
			);
		});
	});

	it("keeps project chats out of personal memory settings", async () => {
		const user = userEvent.setup();
		const onLocationChange = vi.fn();
		render(
			<Wrapper onLocationChange={onLocationChange}>
				<ChatTopBar
					chat={{ ...MockChat, project_id: "project-1" }}
					panel={{ showSidebarPanel: false, onToggleSidebar: () => {} }}
				/>
			</Wrapper>,
		);

		await waitFor(() => {
			expect(onLocationChange).toHaveBeenCalledWith(`/agents/${MockChat.id}`);
		});
		await user.click(
			screen.getByRole("button", { name: "Open agent actions" }),
		);

		expect(onLocationChange).toHaveBeenCalledTimes(1);
	});
});
