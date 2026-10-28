import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter } from "react-router";
import { API } from "#/api/api";
import {
	MockDefaultOrganization,
	MockOrganization2,
} from "#/testHelpers/entities";
import { renderWithRouter } from "#/testHelpers/renderHelpers";
import {
	MockFoundation,
	MockFoundationPermissions,
} from "#/testHelpers/templateResources";
import FoundationsPage from "./FoundationsPage";

vi.mock("#/modules/dashboard/useDashboard", () => ({
	useDashboard: () => ({
		experiments: ["template-resources"],
		organizations: [MockDefaultOrganization, MockOrganization2],
	}),
}));
beforeEach(() => {
	vi.spyOn(API, "checkAuthorization").mockResolvedValue(
		MockFoundationPermissions,
	);
	vi.spyOn(API, "getFoundations").mockImplementation(async (organization) =>
		organization === MockOrganization2.id
			? [{ ...MockFoundation, organization_id: MockOrganization2.id }]
			: [],
	);
});
afterEach(() => vi.restoreAllMocks());

it("lists all dashboard organizations and opens a Foundation using its canonical organization ID", async () => {
	const router = createMemoryRouter(
		[
			{ path: "/foundations", element: <FoundationsPage /> },
			{
				path: "/organizations/:organization/foundations/:foundation",
				element: <div />,
			},
		],
		{ initialEntries: ["/foundations"] },
	);
	renderWithRouter(router);
	const user = userEvent.setup();
	await user.click(
		await screen.findByRole("link", { name: MockFoundation.name }),
	);
	expect(API.getFoundations).toHaveBeenCalledWith(MockDefaultOrganization.id);
	expect(API.getFoundations).toHaveBeenCalledWith(MockOrganization2.id);
	expect(router.state.location.pathname).toBe(
		`/organizations/${MockOrganization2.id}/foundations/${MockFoundation.id}`,
	);
});

it("preserves the organization-specific list and preselects that organization for creation", async () => {
	const router = createMemoryRouter(
		[
			{
				path: "/organizations/:organization/foundations",
				element: <FoundationsPage />,
			},
			{ path: "/foundations/new", element: <div /> },
		],
		{
			initialEntries: [`/organizations/${MockOrganization2.name}/foundations`],
		},
	);
	renderWithRouter(router);
	const user = userEvent.setup();
	await user.click(await screen.findByRole("link", { name: "New Foundation" }));
	await waitFor(() =>
		expect(router.state.location.search).toBe(
			`?organization=${MockOrganization2.id}`,
		),
	);
	expect(API.getFoundations).toHaveBeenCalledWith(MockOrganization2.id);
	expect(API.getFoundations).not.toHaveBeenCalledWith(
		MockDefaultOrganization.id,
	);
});
