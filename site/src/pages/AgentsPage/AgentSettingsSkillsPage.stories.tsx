import type { Meta, StoryObj } from "@storybook/react-vite";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { skillList } from "#/api/queries/skills";
import {
	MockDefaultOrganization,
	MockOrganization2,
	MockOrganization3,
	MockUserMember,
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
			{
				key: skillList({ type: "user", user: "me" }).queryKey,
				data: MockSkills,
			},
			{
				key: skillList({
					type: "organization",
					organizationId: MockDefaultOrganization.id,
				}).queryKey,
				data: [...MockSkills.slice(0, 2), MockDisabledSkill],
			},
			{
				key: skillList({
					type: "organization",
					organizationId: MockOrganization2.id,
				}).queryKey,
				data: [MockSkills[2]],
			},
			{
				key: skillList({
					type: "organization",
					organizationId: MockOrganization3.id,
				}).queryKey,
				data: [MockDisabledSkill],
			},
		],
	},
};
