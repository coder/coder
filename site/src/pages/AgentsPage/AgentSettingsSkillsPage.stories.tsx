import type { Meta, StoryObj } from "@storybook/react-vite";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { skillsKey } from "#/api/queries/skills";
import type { Organization, SkillMetadata } from "#/api/typesGenerated";
import {
	MockDefaultOrganization,
	MockOrganization2,
	MockUserMember,
} from "#/testHelpers/entities";
import { MockSkill, MockSkills } from "#/testHelpers/skills";
import {
	withAuthProvider,
	withDashboardProvider,
	withToaster,
} from "#/testHelpers/storybook";
import AgentSettingsSkillsPage from "./AgentSettingsSkillsPage";

const MockOrganization3: Organization = {
	...MockOrganization2,
	id: "my-organization-3-id",
	name: "my-organization-3",
	display_name: "My Organization 3",
};

const disabledSkill: SkillMetadata = {
	...MockSkill,
	id: "skill-legacy-review",
	name: "legacy-review",
	description: "Older review checklist kept for reference.",
	enabled: false,
};

const organizationSkillsQuery = (
	organization: Organization,
	data: SkillMetadata[],
) => ({
	key: skillsKey({ type: "organization", organizationId: organization.id }),
	data,
});

const meta = {
	title: "pages/AgentsPage/AgentSettingsSkillsPage",
	component: AgentSettingsSkillsPage,
	decorators: [withToaster, withAuthProvider, withDashboardProvider],
	parameters: {
		user: MockUserMember,
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
			organizationSkillsQuery(MockDefaultOrganization, [
				...MockSkills.slice(0, 2),
				disabledSkill,
			]),
			organizationSkillsQuery(MockOrganization2, [MockSkills[2]]),
			organizationSkillsQuery(MockOrganization3, [disabledSkill]),
		],
	},
};

export const NoOrganizationSkills: Story = {
	parameters: {
		queries: [
			{ key: skillsKey({ type: "user", user: "me" }), data: MockSkills },
			organizationSkillsQuery(MockDefaultOrganization, []),
			organizationSkillsQuery(MockOrganization2, [disabledSkill]),
			organizationSkillsQuery(MockOrganization3, []),
		],
	},
};
