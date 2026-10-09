import type { Meta, StoryObj } from "@storybook/react-vite";
import { spyOn, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import { skillsKey } from "#/api/queries/skills";
import {
	MockDefaultOrganization,
	MockOrganization2,
	MockOrganization3,
	MockUserMember,
	mockApiError,
} from "#/testHelpers/entities";
import { MockDisabledSkill, MockSkills } from "#/testHelpers/skills";
import {
	withAuthProvider,
	withDashboardProvider,
	withToaster,
} from "#/testHelpers/storybook";
import AgentSettingsSkillsPage from "./AgentSettingsSkillsPage";

const meta = {
	title: "pages/AgentsPage/AgentSettingsSkillsPage",
	component: AgentSettingsSkillsPage,
	decorators: [withToaster, withAuthProvider, withDashboardProvider],
	parameters: {
		user: {
			...MockUserMember,
			organization_ids: [
				MockDefaultOrganization.id,
				MockOrganization2.id,
				MockOrganization3.id,
			],
		},
		organizations: [
			MockDefaultOrganization,
			MockOrganization2,
			MockOrganization3,
		],
		reactRouter: reactRouterParameters({
			location: { path: "/agents/settings/skills" },
			routing: { path: "/agents/settings/skills" },
		}),
	},
} satisfies Meta<typeof AgentSettingsSkillsPage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const OrganizationSkills: Story = {
	parameters: {
		queries: [
			{ key: skillsKey({ type: "user", user: "me" }), data: MockSkills },
			{
				key: skillsKey({
					type: "organization",
					organizationId: MockDefaultOrganization.id,
				}),
				data: [...MockSkills.slice(0, 2), MockDisabledSkill],
			},
			{
				key: skillsKey({
					type: "organization",
					organizationId: MockOrganization2.id,
				}),
				data: [MockSkills[2]],
			},
			{
				key: skillsKey({
					type: "organization",
					organizationId: MockOrganization3.id,
				}),
				data: [MockDisabledSkill],
			},
		],
	},
};

export const NoOrganizationSkills: Story = {
	parameters: {
		queries: [
			{ key: skillsKey({ type: "user", user: "me" }), data: MockSkills },
			{
				key: skillsKey({
					type: "organization",
					organizationId: MockDefaultOrganization.id,
				}),
				data: [],
			},
			{
				key: skillsKey({
					type: "organization",
					organizationId: MockOrganization2.id,
				}),
				data: [MockDisabledSkill],
			},
			{
				key: skillsKey({
					type: "organization",
					organizationId: MockOrganization3.id,
				}),
				data: [],
			},
		],
	},
};

export const OrganizationSkillsError: Story = {
	parameters: {
		organizations: [MockDefaultOrganization],
		queries: [
			{ key: skillsKey({ type: "user", user: "me" }), data: MockSkills },
		],
	},
	beforeEach: () => {
		spyOn(API.experimental, "getOrganizationSkills").mockRejectedValue(
			mockApiError({ message: "Failed to load organization skills." }),
		);
	},
	play: async ({ canvasElement }) => {
		await within(canvasElement).findByRole("button", { name: "Retry" });
	},
};
