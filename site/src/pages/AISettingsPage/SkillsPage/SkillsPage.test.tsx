import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter } from "react-router";
import { expect, it, vi } from "vitest";
import { API } from "#/api/api";
import {
	MockNoPermissions,
	MockOrganization,
	MockOrganization2,
	MockUserOwner,
} from "#/testHelpers/entities";
import { renderWithRouter } from "#/testHelpers/renderHelpers";
import SkillsPage from "./SkillsPage";

vi.mock("#/hooks/useAuthenticated", () => ({
	useAuthenticated: () => ({
		user: MockUserOwner,
		permissions: { ...MockNoPermissions, viewAnyOrganizationSkills: true },
	}),
}));
vi.mock("#/modules/dashboard/useDashboard", () => ({
	useDashboard: () => ({
		organizations: [MockOrganization, MockOrganization2],
	}),
}));

it("loads the chosen organization's skills when the organization changes", async () => {
	const user = userEvent.setup();
	vi.spyOn(API, "checkAuthorization").mockImplementation(async ({ checks }) =>
		Object.fromEntries(Object.keys(checks).map((key) => [key, true])),
	);
	const getSkills = vi
		.spyOn(API.experimental, "getOrganizationSkills")
		.mockResolvedValue([]);
	const router = createMemoryRouter(
		[{ path: "/ai/settings/skills", element: <SkillsPage /> }],
		{ initialEntries: ["/ai/settings/skills"] },
	);
	renderWithRouter(router);

	await user.click(
		await screen.findByRole("combobox", {
			name: `Organization ${MockOrganization.display_name}`,
		}),
	);
	await user.click(
		await screen.findByRole("option", { name: /My Organization 2/ }),
	);

	await waitFor(() =>
		expect(getSkills).toHaveBeenLastCalledWith(MockOrganization2.id),
	);
	expect(new URLSearchParams(router.state.location.search).get("org")).toBe(
		MockOrganization2.name,
	);
});
