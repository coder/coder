import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter } from "react-router";
import { describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import {
	MockDefaultOrganization,
	MockGroup,
	MockOrganizationSkillACL,
	MockOrganizationSkillACLAvailable,
	MockUserOwner,
} from "#/testHelpers/entities";
import { renderWithRouter } from "#/testHelpers/renderHelpers";
import { MockSkills } from "#/testHelpers/skills";
import SkillsPage from "./SkillsPage";

vi.mock("#/hooks/useAuthenticated", () => ({
	useAuthenticated: () => ({
		user: MockUserOwner,
		permissions: { viewAnyOrganizationSkills: true },
	}),
}));

vi.mock("#/modules/dashboard/useDashboard", () => ({
	useDashboard: () => ({ organizations: [MockDefaultOrganization] }),
}));

describe("SkillsPage Manage permissions", () => {
	it("returns focus to the row menu button when the dialog closes", async () => {
		vi.spyOn(API, "checkAuthorization").mockImplementation(async ({ checks }) =>
			Object.fromEntries(Object.keys(checks).map((key) => [key, true])),
		);
		vi.spyOn(API.experimental, "getOrganizationSkills").mockResolvedValue(
			MockSkills,
		);
		vi.spyOn(API.experimental, "getOrganizationSkillACL").mockResolvedValue(
			MockOrganizationSkillACL,
		);
		vi.spyOn(
			API.experimental,
			"getOrganizationSkillACLAvailable",
		).mockResolvedValue(MockOrganizationSkillACLAvailable);
		const router = createMemoryRouter(
			[{ path: "/ai/settings/skills", element: <SkillsPage /> }],
			{ initialEntries: ["/ai/settings/skills"] },
		);
		renderWithRouter(router);
		const user = userEvent.setup();

		const [menuButton] = await screen.findAllByRole("button", {
			name: "Open menu",
		});
		await user.click(menuButton);
		await user.click(
			await screen.findByRole("menuitem", { name: "Manage permissions" }),
		);
		await screen.findByRole("button", {
			name: `Remove ${MockGroup.display_name}`,
		});
		await user.keyboard("{Escape}");

		await waitFor(() => expect(menuButton).toHaveFocus());
	});
});
