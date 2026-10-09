import { screen, waitFor, within } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { skillsKey } from "#/api/queries/skills";
import type { Organization, SkillMetadata } from "#/api/typesGenerated";
import { MockOrganization, MockOrganization2 } from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import { MockSkill } from "#/testHelpers/skills";
import AgentSettingsSkillsPage from "./AgentSettingsSkillsPage";

const enabledSkill: SkillMetadata = {
	...MockSkill,
	id: "skill-reviewer",
	name: "reviewer",
};
const disabledSkill: SkillMetadata = {
	...MockSkill,
	id: "skill-legacy",
	name: "legacy",
	enabled: false,
};

const renderPage = (
	organizations: Organization[],
	skillsByOrganization: Record<string, SkillMetadata[]>,
) => {
	server.use(
		http.get("/api/v2/organizations", () => HttpResponse.json(organizations)),
		http.get("/api/experimental/users/me/skills", () => HttpResponse.json([])),
		http.get(
			"/api/experimental/organizations/:organizationId/skills",
			({ params }) =>
				HttpResponse.json(
					skillsByOrganization[String(params.organizationId)] ?? [],
				),
		),
	);
	const { queryClient } = renderWithAuth(<AgentSettingsSkillsPage />);
	return {
		waitForOrganizationSkills: () =>
			waitFor(() => {
				for (const organization of organizations) {
					const state = queryClient.getQueryState(
						skillsKey({
							type: "organization",
							organizationId: organization.id,
						}),
					);
					expect(state?.status).toBe("success");
				}
			}),
	};
};

describe("AgentSettingsSkillsPage", () => {
	it("lists enabled organization skills and omits organizations without any", async () => {
		const { waitForOrganizationSkills } = renderPage(
			[MockOrganization, MockOrganization2],
			{
				[MockOrganization.id]: [enabledSkill, disabledSkill],
				[MockOrganization2.id]: [disabledSkill],
			},
		);
		await waitForOrganizationSkills();

		const table = await screen.findByRole("table", {
			name: MockOrganization.display_name,
		});
		expect(within(table).getByText(enabledSkill.name)).toBeInTheDocument();
		expect(
			within(table).queryByText(disabledSkill.name),
		).not.toBeInTheDocument();
		expect(
			screen.queryByRole("table", { name: MockOrganization2.display_name }),
		).not.toBeInTheDocument();
	});

	it("hides the organization section when no organization shares a skill", async () => {
		const { waitForOrganizationSkills } = renderPage([MockOrganization], {
			[MockOrganization.id]: [disabledSkill],
		});

		await waitForOrganizationSkills();
		expect(
			screen.queryByRole("heading", { name: "From your organizations" }),
		).not.toBeInTheDocument();
	});
});
