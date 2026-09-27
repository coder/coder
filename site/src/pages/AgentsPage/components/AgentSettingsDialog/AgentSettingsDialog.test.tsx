import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { FC } from "react";
import { QueryClientProvider } from "react-query";
import { MemoryRouter, useLocation } from "react-router";
import { describe, expect, it, vi } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
import { ThemeOverride } from "#/contexts/ThemeProvider";
import { DashboardContext } from "#/modules/dashboard/DashboardProvider";
import {
	MockAppearanceConfig,
	MockBuildInfo,
	MockDefaultOrganization,
	MockEntitlements,
} from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import themes, { DEFAULT_THEME } from "#/theme";
import { AgentSettingsDialog } from "./AgentSettingsDialog";

const dashboardValue = {
	entitlements: MockEntitlements,
	experiments: [] as TypesGen.Experiment[],
	appearance: MockAppearanceConfig,
	buildInfo: MockBuildInfo,
	organizations: [MockDefaultOrganization],
	showOrganizations: false,
	canViewOrganizationSettings: false,
};

const SearchProbe: FC = () => {
	const location = useLocation();
	return <div data-testid="location-search">{location.search}</div>;
};

const renderDialog = (onClose = vi.fn()) => {
	render(
		<QueryClientProvider client={createTestQueryClient()}>
			<ThemeOverride theme={themes[DEFAULT_THEME]}>
				<TooltipProvider>
					<MemoryRouter
						initialEntries={[
							"/agents/chat-1?archived=archived&settings=general",
						]}
					>
						<DashboardContext.Provider value={dashboardValue}>
							<AgentSettingsDialog
								section="general"
								onClose={onClose}
								isAdmin
								isPersonalModelOverridesEnabled
								canManageAgentSettings={false}
							/>
							<SearchProbe />
						</DashboardContext.Provider>
					</MemoryRouter>
				</TooltipProvider>
			</ThemeOverride>
		</QueryClientProvider>,
	);
	return { onClose };
};

describe("AgentSettingsDialog", () => {
	it("switches section without leaving the chat route", async () => {
		const user = userEvent.setup();
		renderDialog();

		await user.click(
			await screen.findByRole("link", { name: "Secrets (API keys)" }),
		);

		await waitFor(() => {
			expect(screen.getByTestId("location-search").textContent).toBe(
				"?archived=archived&settings=api-keys",
			);
		});
	});

	it("closes on the close button", async () => {
		const user = userEvent.setup();
		const { onClose } = renderDialog();

		await user.click(
			await screen.findByRole("button", { name: "Close settings" }),
		);

		expect(onClose).toHaveBeenCalled();
	});
});
