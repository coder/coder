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
		agent_runtime_hours: { enabled: true, entitlement: "entitled" },
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

it("links organization group member readers to the Spend page", async () => {
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
	await user.click(await screen.findByRole("link", { name: "Spend" }));
	expect(router.state.location.pathname).toBe("/ai/settings/spend");
	expect(router.state.location.search).toBe("?org=second");
});

it("links organization instruction readers to the Instructions page", async () => {
	const user = userEvent.setup();
	vi.spyOn(API, "getOrganizations").mockResolvedValue([MockOrganization]);
	vi.spyOn(API, "checkAuthorization").mockResolvedValue({
		[`${MockOrganization.id}.viewChatModelConfigs`]: true,
	});
	vi.spyOn(API.experimental, "getChatModels").mockResolvedValue({
		models: [],
		providers: [],
		unsupported_providers: [],
	});
	const router = createMemoryRouter(
		[
			{ path: "/ai/settings", element: <AISettingsSidebar /> },
			{ path: "/ai/settings/instructions", element: <div /> },
		],
		{ initialEntries: ["/ai/settings?org=second"] },
	);
	renderWithRouter(router);
	await user.click(await screen.findByRole("link", { name: "Instructions" }));
	expect(router.state.location.pathname).toBe("/ai/settings/instructions");
	expect(router.state.location.search).toBe("?org=second");
});

it("links organization group managers to the Agent Hours page", async () => {
	const user = userEvent.setup();
	vi.spyOn(API, "getOrganizations").mockResolvedValue([MockOrganization]);
	vi.spyOn(API, "checkAuthorization").mockImplementation(async ({ checks }) =>
		Object.fromEntries(
			Object.entries(checks).map(([key, check]) => [
				key,
				check.object.resource_type === "group" && check.action === "update",
			]),
		),
	);
	vi.spyOn(API.experimental, "getChatModels").mockResolvedValue({
		models: [],
		providers: [],
		unsupported_providers: [],
	});
	const router = createMemoryRouter(
		[
			{ path: "/ai/settings", element: <AISettingsSidebar /> },
			{ path: "/ai/settings/agent-hours", element: <div /> },
		],
		{ initialEntries: ["/ai/settings?org=second"] },
	);
	renderWithRouter(router);
	await user.click(await screen.findByRole("link", { name: "Agent Hours" }));
	expect(router.state.location.pathname).toBe("/ai/settings/agent-hours");
	expect(router.state.location.search).toBe("?org=second");
});
