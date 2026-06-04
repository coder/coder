import { render, screen } from "@testing-library/react";
import type { FC, PropsWithChildren } from "react";
import { MemoryRouter } from "react-router";
import { describe, expect, it } from "vitest";
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
import { ThemeOverride } from "#/contexts/ThemeProvider";
import { DashboardContext } from "#/modules/dashboard/DashboardProvider";
import {
	MockAppearanceConfig,
	MockBuildInfo,
	MockDefaultOrganization,
	MockEntitlements,
	MockUserOwner,
} from "#/testHelpers/entities";
import themes, { DEFAULT_THEME } from "#/theme";
import { Sidebar } from "./Sidebar";

const dashboardValue = {
	entitlements: {
		...MockEntitlements,
		features: {
			...MockEntitlements.features,
			advanced_template_scheduling: {
				...MockEntitlements.features.advanced_template_scheduling,
				enabled: true,
			},
		},
	},
	experiments: ["oauth2" as const],
	appearance: MockAppearanceConfig,
	buildInfo: MockBuildInfo,
	organizations: [MockDefaultOrganization],
	showOrganizations: false,
	canViewOrganizationSettings: false,
};

const Wrapper: FC<PropsWithChildren> = ({ children }) => (
	<ThemeOverride theme={themes[DEFAULT_THEME]}>
		<TooltipProvider>
			<MemoryRouter>
				<DashboardContext.Provider value={dashboardValue}>
					{children}
				</DashboardContext.Provider>
			</MemoryRouter>
		</TooltipProvider>
	</ThemeOverride>
);

describe("User settings Sidebar", () => {
	it("renders nav items in alphabetical order", () => {
		render(<Sidebar user={MockUserOwner} />, { wrapper: Wrapper });

		const navItems = screen.getAllByRole("link").map((item) =>
			(item.textContent ?? "")
				.replace(/\(.*?\)/g, "")
				.replace(/beta/i, "")
				.replace(/\s+/g, " ")
				.trim(),
		);

		expect(navItems.length).toBeGreaterThan(0);
		expect(navItems).toEqual([...navItems].sort((a, b) => a.localeCompare(b)));
	});
});
