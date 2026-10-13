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
		vi.spyOn(API.experimental, "getOrganizationSkills").mockImplementation(
			async (organizationId) => [
				{
					...MockSkill,
					name:
						organizationId === MockOrganization2.id
							? "second-skill"
							: "default-skill",
				},
			],
		);

		renderWithAuth(<AgentSettingsSkillsPage />);

		await within(
			await screen.findByRole("table", {
				name: MockDefaultOrganization.display_name,
			}),
		).findByText("default-skill");
		await user.click(
			screen.getByRole("combobox", {
				name: `Organization ${MockDefaultOrganization.display_name}`,
			}),
		);
		screen.getByRole("option", { name: /My Organization$/ });
		screen.getByRole("option", { name: /My Organization 2/ });
		expect(
			screen.queryByRole("option", { name: /My Organization 3/ }),
		).toBeNull();
		await user.click(screen.getByRole("option", { name: /My Organization 2/ }));
		await within(
			await screen.findByRole("table", {
				name: MockOrganization2.display_name,
			}),
		).findByText("second-skill");
		expect(
			screen.queryByRole("table", {
				name: MockDefaultOrganization.display_name,
			}),
		).toBeNull();
	});
});
