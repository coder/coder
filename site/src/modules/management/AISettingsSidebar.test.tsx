import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter } from "react-router";
import { beforeEach, expect, it, vi } from "vitest";
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

const mockPermissions = vi.hoisted(() => ({
	current: {} as Record<string, boolean>,
}));

vi.mock("#/hooks/useAuthenticated", () => ({
	useAuthenticated: () => ({
		user: MockUserMember,
		permissions: { ...MockNoPermissions, ...mockPermissions.current },
	}),
}));
vi.mock("#/modules/dashboard/useDashboard", () => ({
	useDashboard: () => ({
		entitlements: mockAIGatewayEntitlements,
		organizations: [MockOrganization],
	}),
}));

beforeEach(() => {
	mockPermissions.current = {};
});

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

it.each([
	{ requested: "second", expectedSearch: "" },
	{
		requested: MockOrganization.name,
		expectedSearch: `?org=${MockOrganization.name}`,
	},
])(
	"keeps ?org=$requested on the Skills link only when the user can read its skills",
	async ({ requested, expectedSearch }) => {
		const user = userEvent.setup();
		mockPermissions.current = { viewAnyOrganizationSkills: true };
		vi.spyOn(API, "getOrganizations").mockResolvedValue([MockOrganization]);
		vi.spyOn(API, "checkAuthorization").mockResolvedValue({
			[`${MockOrganization.id}.viewOrganizationSkills`]: true,
		});
		vi.spyOn(API.experimental, "getChatModels").mockResolvedValue({
			models: [],
			providers: [],
			unsupported_providers: [],
		});
		const router = createMemoryRouter(
			[
				{ path: "/ai/settings", element: <AISettingsSidebar /> },
				{ path: "/ai/settings/skills", element: <div /> },
			],
			{ initialEntries: [`/ai/settings?org=${requested}`] },
		);
		renderWithRouter(router);
		const skillsLink = await screen.findByRole("link", { name: "Skills" });
		await waitFor(() =>
			expect(skillsLink).toHaveAttribute(
				"href",
				`/ai/settings/skills${expectedSearch}`,
			),
		);
		await user.click(skillsLink);
		expect(router.state.location.pathname).toBe("/ai/settings/skills");
		expect(router.state.location.search).toBe(expectedSearch);
	},
);
