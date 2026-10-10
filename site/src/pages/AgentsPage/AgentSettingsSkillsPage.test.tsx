import { screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import {
	MockOrganization,
	MockOrganization2,
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
});
