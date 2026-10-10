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
import { MockSkill } from "#/testHelpers/skills";
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

it("lets a role that can only update skills toggle and edit them", async () => {
	const user = userEvent.setup();
	vi.spyOn(API, "checkAuthorization").mockImplementation(async ({ checks }) =>
		Object.fromEntries(
			Object.keys(checks).map((key) => [
				key,
				!key.endsWith(".createOrganizationSkill") &&
					!key.endsWith(".deleteOrganizationSkill"),
			]),
		),
	);
	const skill = { ...MockSkill, name: "review-sql", enabled: true };
	vi.spyOn(API.experimental, "getOrganizationSkills").mockResolvedValue([
		skill,
	]);
	vi.spyOn(API.experimental, "getOrganizationSkillByName").mockResolvedValue({
		...skill,
		content: "---\nname: review-sql\ndescription: Review SQL.\n---\n\nBody\n",
	});
	const updateSkill = vi
		.spyOn(API.experimental, "updateOrganizationSkill")
		.mockResolvedValue({ ...skill, enabled: false, content: "" });
	const router = createMemoryRouter(
		[{ path: "/ai/settings/skills", element: <SkillsPage /> }],
		{ initialEntries: ["/ai/settings/skills"] },
	);
	renderWithRouter(router);

	await user.click(
		await screen.findByRole("switch", { name: "Enable review-sql" }),
	);
	await waitFor(() =>
		expect(updateSkill).toHaveBeenCalledWith(
			MockOrganization.id,
			"review-sql",
			{
				enabled: false,
			},
		),
	);
	expect(screen.queryByRole("button", { name: "Add skill" })).toBeNull();

	await user.click(screen.getByRole("button", { name: "Open menu" }));
	expect(screen.queryByRole("menuitem", { name: /Delete/ })).toBeNull();
	await user.click(screen.getByRole("menuitem", { name: "Edit" }));
	await screen.findByRole("dialog", { name: "Edit organization skill" });
});
