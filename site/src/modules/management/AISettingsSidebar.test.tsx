import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter, useLocation } from "react-router";
import { afterEach, expect, it, vi } from "vitest";
import { API, withDefaultFeatures } from "#/api/api";
import {
	MockEntitlements,
	MockNoPermissions,
	MockOrganization,
	MockUserMember,
} from "#/testHelpers/entities";
import { renderWithRouter } from "#/testHelpers/renderHelpers";
import { AISettingsSidebar } from "./AISettingsSidebar";

const entitlementsFor = (aibridgeEnabled: boolean) => ({
	...MockEntitlements,
	features: withDefaultFeatures({
		aibridge: { enabled: aibridgeEnabled, entitlement: "entitled" },
	}),
});
const dashboard = { entitlements: entitlementsFor(true) };

vi.mock("#/hooks/useAuthenticated", () => ({
	useAuthenticated: () => ({
		user: MockUserMember,
		permissions: MockNoPermissions,
	}),
}));
vi.mock("#/modules/dashboard/useDashboard", () => ({
	useDashboard: () => ({
		entitlements: dashboard.entitlements,
		organizations: [MockOrganization],
	}),
}));

afterEach(() => {
	dashboard.entitlements = entitlementsFor(true);
});

const grantSpendOrganization = () => {
	vi.spyOn(API, "getOrganizations").mockResolvedValue([MockOrganization]);
	vi.spyOn(API, "checkAuthorization").mockResolvedValue({
		[MockOrganization.id]: true,
	});
	vi.spyOn(API.experimental, "getChatModels").mockResolvedValue({
		models: [],
		providers: [],
		unsupported_providers: [],
	});
};

// Renders a fresh sidebar element on every navigation, so a flipped
// entitlement reaches the container instead of React bailing out on the
// unchanged route element.
const SidebarOnLocation = () => {
	useLocation();
	return <AISettingsSidebar />;
};

it("links organization group member readers to the Spend page", async () => {
	const user = userEvent.setup();
	grantSpendOrganization();
	const router = createMemoryRouter(
		[
			{ path: "/ai/settings", element: <AISettingsSidebar /> },
			{ path: "/ai/settings/spend", element: <div /> },
		],
		{ initialEntries: ["/ai/settings"] },
	);
	renderWithRouter(router);
	await user.click(await screen.findByRole("link", { name: "Spend" }));
	expect(router.state.location.pathname).toBe("/ai/settings/spend");
});

// The organization query keeps its cached result once disabled, so the link
// must follow the entitlement itself rather than the query being enabled.
it("hides the Spend link once AI Gateway is disabled after the organizations loaded", async () => {
	grantSpendOrganization();
	const router = createMemoryRouter(
		[{ path: "/ai/settings", element: <SidebarOnLocation /> }],
		{ initialEntries: ["/ai/settings"] },
	);
	renderWithRouter(router);
	await screen.findByRole("link", { name: "Spend" });

	dashboard.entitlements = entitlementsFor(false);
	await router.navigate("/ai/settings?refresh=1");
	await waitFor(() =>
		expect(screen.queryByRole("link", { name: "Spend" })).toBeNull(),
	);
});
