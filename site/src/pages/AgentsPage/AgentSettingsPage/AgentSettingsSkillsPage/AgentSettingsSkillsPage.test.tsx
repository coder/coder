import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type { OrganizationSkillPreferences } from "#/api/typesGenerated";
import {
	MockDefaultOrganization,
	MockOrganization,
	MockOrganization2,
	MockOrganization3,
	MockUserMember,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import {
	MockOrganizationSkillPreferences,
	MockSkill,
} from "#/testHelpers/skills";
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
		vi.spyOn(
			API.experimental,
			"getOrganizationSkillPreferences",
		).mockResolvedValue(MockOrganizationSkillPreferences);
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
		vi.spyOn(
			API.experimental,
			"getOrganizationSkillPreferences",
		).mockResolvedValue(MockOrganizationSkillPreferences);
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

	describe("member controls", () => {
		const mockReviewSkill = { ...MockSkill, name: "review-sql" };
		const mockMemberControlsPreferences: OrganizationSkillPreferences = {
			...MockOrganizationSkillPreferences,
			members_can_disable_skills: true,
		};

		const renderMemberControls = () => {
			vi.spyOn(API, "getAuthenticatedUser").mockResolvedValue({
				...MockUserMember,
				organization_ids: [MockOrganization.id],
			});
			vi.spyOn(API, "getOrganizations").mockResolvedValue([MockOrganization]);
			vi.spyOn(API.experimental, "getUserSkills").mockResolvedValue([]);
			vi.spyOn(API.experimental, "getOrganizationSkills").mockResolvedValue([
				mockReviewSkill,
			]);
			const getPreferences = vi
				.spyOn(API.experimental, "getOrganizationSkillPreferences")
				.mockResolvedValue(mockMemberControlsPreferences);
			const updatePreferences = vi
				.spyOn(API.experimental, "updateOrganizationSkillPreferences")
				.mockResolvedValue(mockMemberControlsPreferences);
			renderWithAuth(<AgentSettingsSkillsPage />);
			return { getPreferences, updatePreferences };
		};

		it("turns one organization skill off for the member's chats", async () => {
			const user = userEvent.setup();
			const { updatePreferences } = renderMemberControls();

			await user.click(
				await screen.findByRole("switch", {
					name: "Use review-sql in my chats",
				}),
			);
			await waitFor(() =>
				expect(updatePreferences).toHaveBeenCalledWith(MockOrganization.id, {
					skills: { [mockReviewSkill.id]: false },
				}),
			);
		});

		it("turns every organization skill off with the master switch", async () => {
			const user = userEvent.setup();
			const { updatePreferences } = renderMemberControls();

			await user.click(
				await screen.findByRole("switch", {
					name: "Use organization skills in my chats",
				}),
			);
			await waitFor(() =>
				expect(updatePreferences).toHaveBeenCalledWith(MockOrganization.id, {
					all_skills_disabled: true,
				}),
			);
		});

		it("reloads the preferences when a change fails", async () => {
			const user = userEvent.setup();
			const { getPreferences, updatePreferences } = renderMemberControls();
			updatePreferences.mockRejectedValue(new Error("Setting is off."));

			await user.click(
				await screen.findByRole("switch", {
					name: "Use review-sql in my chats",
				}),
			);
			await waitFor(() => expect(getPreferences).toHaveBeenCalledTimes(2));
		});
	});
});
