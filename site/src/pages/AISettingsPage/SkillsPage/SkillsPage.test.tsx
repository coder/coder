import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter } from "react-router";
import { expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { organizationSkillSettingsKey } from "#/api/queries/organizationSkills";
import {
	MockNoPermissions,
	MockOrganization,
	MockOrganization2,
	MockUserOwner,
} from "#/testHelpers/entities";
import { renderWithRouter } from "#/testHelpers/renderHelpers";
import { MockOrganizationSkillSettings, MockSkill } from "#/testHelpers/skills";
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
	const mockReviewSkill = { ...MockSkill, name: "review-sql", enabled: true };
	vi.spyOn(API.experimental, "getOrganizationSkills").mockResolvedValue([
		mockReviewSkill,
	]);
	vi.spyOn(API.experimental, "getOrganizationSkillByName").mockResolvedValue({
		...mockReviewSkill,
		content: "---\nname: review-sql\ndescription: Review SQL.\n---\n\nBody\n",
	});
	vi.spyOn(API.experimental, "getOrganizationSkillSettings").mockResolvedValue(
		MockOrganizationSkillSettings,
	);
	const updateSkill = vi
		.spyOn(API.experimental, "updateOrganizationSkill")
		.mockResolvedValue({ ...mockReviewSkill, enabled: false, content: "" });
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

	await user.click(screen.getByRole("button", { name: "Open menu" }));
	await user.click(screen.getByRole("menuitem", { name: "Edit" }));
	await waitFor(() =>
		expect(screen.getByRole("textbox", { name: "Body" })).toHaveValue("Body"),
	);
	await user.type(
		screen.getByRole("textbox", { name: "Description" }),
		"Review SQL changes.",
	);
	await user.click(screen.getByRole("button", { name: "Save skill" }));
	await waitFor(() =>
		expect(updateSkill).toHaveBeenLastCalledWith(
			MockOrganization.id,
			"review-sql",
			{
				content:
					'---\nname: review-sql\ndescription: "Review SQL changes."\n---\nBody\n',
			},
		),
	);
});

it("lets an organization admin allow members to turn off skills", async () => {
	const user = userEvent.setup();
	vi.spyOn(API, "checkAuthorization").mockImplementation(async ({ checks }) =>
		Object.fromEntries(Object.keys(checks).map((key) => [key, true])),
	);
	vi.spyOn(API.experimental, "getOrganizationSkills").mockResolvedValue([]);
	vi.spyOn(API.experimental, "getOrganizationSkillSettings").mockResolvedValue(
		MockOrganizationSkillSettings,
	);
	const updateSettings = vi
		.spyOn(API.experimental, "updateOrganizationSkillSettings")
		.mockResolvedValue({ members_can_disable_skills: true });
	const router = createMemoryRouter(
		[{ path: "/ai/settings/skills", element: <SkillsPage /> }],
		{ initialEntries: ["/ai/settings/skills"] },
	);
	renderWithRouter(router);

	await user.click(
		await screen.findByRole("switch", {
			name: "Members can turn off organization skills",
		}),
	);
	await waitFor(() =>
		expect(updateSettings).toHaveBeenCalledWith(MockOrganization.id, {
			members_can_disable_skills: true,
		}),
	);
});

it("retries loading the organization setting after a failed reload", async () => {
	const user = userEvent.setup();
	vi.spyOn(API, "checkAuthorization").mockImplementation(async ({ checks }) =>
		Object.fromEntries(Object.keys(checks).map((key) => [key, true])),
	);
	vi.spyOn(API.experimental, "getOrganizationSkills").mockResolvedValue([]);
	const getSettings = vi
		.spyOn(API.experimental, "getOrganizationSkillSettings")
		.mockResolvedValue(MockOrganizationSkillSettings);
	const router = createMemoryRouter(
		[{ path: "/ai/settings/skills", element: <SkillsPage /> }],
		{ initialEntries: ["/ai/settings/skills"] },
	);
	const { queryClient } = renderWithRouter(router);
	await screen.findByRole("switch", {
		name: "Members can turn off organization skills",
	});

	getSettings.mockRejectedValue(new Error("Settings are unavailable."));
	await act(() =>
		queryClient.invalidateQueries({
			queryKey: organizationSkillSettingsKey(MockOrganization.id),
		}),
	);
	const retry = await screen.findByRole("button", {
		name: "Retry loading the organization setting",
	});

	getSettings.mockClear();
	await user.click(retry);
	await waitFor(() =>
		expect(getSettings).toHaveBeenCalledWith(MockOrganization.id),
	);
});
