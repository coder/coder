import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import {
	MockDefaultOrganization,
	MockOrganization,
	MockOrganization2,
	MockOrganization3,
	MockUserMember,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { MockSkill } from "#/testHelpers/skills";
import AgentSettingsSkillsPage from "./AgentSettingsSkillsPage";

describe("AgentSettingsSkillsPage", () => {
	it("lists skills only from organizations the user belongs to", async () => {
		vi.spyOn(API, "getAuthenticatedUser").mockResolvedValue({
			...MockUserMember,
			organization_ids: [MockOrganization.id],
		});
		vi.spyOn(API, "getOrganizations").mockResolvedValue([
			MockOrganization,
			MockOrganization2,
		]);
		vi.spyOn(API.experimental, "getUserSkills").mockResolvedValue([]);
		const getOrganizationSkills = vi
			.spyOn(API.experimental, "getOrganizationSkills")
			.mockResolvedValue([{ ...MockSkill, name: "review-sql" }]);

		renderWithAuth(<AgentSettingsSkillsPage />);

		await within(
			await screen.findByRole("table", { name: MockOrganization.display_name }),
		).findByText("review-sql");
		expect(
			getOrganizationSkills.mock.calls.map(
				([organizationId]) => organizationId,
			),
		).not.toContain(MockOrganization2.id);
	});

	it("shows one organization at a time, starting with the default", async () => {
		const user = userEvent.setup();
		vi.spyOn(API, "getAuthenticatedUser").mockResolvedValue({
			...MockUserMember,
			organization_ids: [MockOrganization2.id, MockDefaultOrganization.id],
		});
		vi.spyOn(API, "getOrganizations").mockResolvedValue([
			MockOrganization2,
			MockDefaultOrganization,
			MockOrganization3,
		]);
		vi.spyOn(API.experimental, "getUserSkills").mockResolvedValue([]);
		const skillName = (organizationId: string) =>
			organizationId === MockOrganization2.id
				? "second-skill"
				: "default-skill";
		const getOrganizationSkills = vi
			.spyOn(API.experimental, "getOrganizationSkills")
			.mockImplementation(async (organizationId) => [
				{ ...MockSkill, name: skillName(organizationId) },
			]);
		const getOrganizationSkillByName = vi
			.spyOn(API.experimental, "getOrganizationSkillByName")
			.mockImplementation(async (_organizationId, name) => ({
				...MockSkill,
				name,
				content: `---\nname: ${name}\n---\nBody\n`,
			}));
		const viewSkill = async (organizationName: string) => {
			const table = await screen.findByRole("table", {
				name: organizationName,
			});
			await user.click(
				await within(table).findByRole("button", { name: "Open menu" }),
			);
			await user.click(await screen.findByRole("menuitem", { name: /View/ }));
			await user.click(await screen.findByRole("button", { name: "Close" }));
		};

		renderWithAuth(<AgentSettingsSkillsPage />);

		await viewSkill(MockDefaultOrganization.display_name);
		expect(getOrganizationSkillByName).toHaveBeenLastCalledWith(
			MockDefaultOrganization.id,
			"default-skill",
		);

		await user.click(
			screen.getByRole("combobox", {
				name: `Organization ${MockDefaultOrganization.display_name}`,
			}),
		);
		await user.click(screen.getByRole("option", { name: /My Organization 2/ }));
		await viewSkill(MockOrganization2.display_name);
		expect(getOrganizationSkillByName).toHaveBeenLastCalledWith(
			MockOrganization2.id,
			"second-skill",
		);
		expect(
			getOrganizationSkills.mock.calls.map(
				([organizationId]) => organizationId,
			),
		).not.toContain(MockOrganization3.id);
	});
});
