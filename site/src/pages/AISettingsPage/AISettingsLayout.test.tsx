import { screen, waitFor } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { API, withDefaultFeatures } from "#/api/api";
import { MockEntitlements, MockOrganization } from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import AISettingsLayout from "./AISettingsLayout";

const aiGatewayEntitlements = (enabled: boolean) => ({
	...MockEntitlements,
	features: withDefaultFeatures({
		aibridge: { enabled, entitlement: enabled ? "entitled" : "not_entitled" },
	}),
});

// Entitlements are cached for the whole session, so the gate must be
// refreshed on navigation for a license change to reach the sidebar without
// a full page load.
it("drops the User spend link on the next navigation after AI Gateway is disabled", async () => {
	const getEntitlements = vi
		.spyOn(API, "getEntitlements")
		.mockResolvedValue(aiGatewayEntitlements(true));
	vi.spyOn(API, "getOrganizations").mockResolvedValue([MockOrganization]);
	vi.spyOn(API, "checkAuthorization").mockResolvedValue({
		[MockOrganization.id]: true,
	});
	vi.spyOn(API.experimental, "getChatModels").mockResolvedValue({
		models: [],
		providers: [],
		unsupported_providers: [],
	});

	const { router } = renderWithAuth(<AISettingsLayout />, {
		path: "/ai/settings",
		route: "/ai/settings/spend",
		children: [
			{ path: "spend", element: <div /> },
			{ path: "models", element: <div /> },
		],
	});
	await screen.findByRole("link", { name: "User spend" });

	getEntitlements.mockResolvedValue(aiGatewayEntitlements(false));
	await router.navigate("/ai/settings/models");

	await waitFor(() =>
		expect(screen.queryByRole("link", { name: "User spend" })).toBeNull(),
	);
});
