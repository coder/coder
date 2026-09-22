import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter } from "react-router";
import { expect, it, vi } from "vitest";
import { API, withDefaultFeatures } from "#/api/api";
import {
	MockEntitlements,
	MockNoPermissions,
	MockOrganization,
	MockUserMember,
} from "#/testHelpers/entities";
import { renderWithRouter } from "#/testHelpers/renderHelpers";
import { AISettingsSidebar } from "./AISettingsSidebar";

const mockAIGatewayEntitlements = {
	...MockEntitlements,
	features: withDefaultFeatures({
		aibridge: { enabled: true, entitlement: "entitled" },
	}),
};

vi.mock("#/hooks/useAuthenticated", () => ({
	useAuthenticated: () => ({
		user: MockUserMember,
		permissions: MockNoPermissions,
	}),
}));
vi.mock("#/modules/dashboard/useDashboard", () => ({
	useDashboard: () => ({
		entitlements: mockAIGatewayEntitlements,
		organizations: [MockOrganization],
	}),
}));

it("links organization group member readers to the User spend page", async () => {
	const user = userEvent.setup();
	vi.spyOn(API, "getOrganizations").mockResolvedValue([MockOrganization]);
	vi.spyOn(API, "checkAuthorization").mockResolvedValue({
		[MockOrganization.id]: true,
	});
	vi.spyOn(API.experimental, "getChatModels").mockResolvedValue({
		models: [],
		providers: [],
		unsupported_providers: [],
	});
	const router = createMemoryRouter(
		[
			{ path: "/ai/settings", element: <AISettingsSidebar /> },
			{ path: "/ai/settings/spend", element: <div /> },
		],
		{ initialEntries: ["/ai/settings?org=second"] },
	);
	renderWithRouter(router);
	await user.click(await screen.findByRole("link", { name: "User spend" }));
	expect(router.state.location.pathname).toBe("/ai/settings/spend");
	expect(router.state.location.search).toBe("?org=second");
});
