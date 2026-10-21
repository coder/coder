import type { Meta, StoryObj } from "@storybook/react-vite";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import {
	organizationSkillSettings,
	organizationSkills,
} from "#/api/queries/organizationSkills";
import { organizationsPermissions } from "#/api/queries/organizations";
import type { OrganizationSkillSettings } from "#/api/typesGenerated";
import type { OrganizationPermissions } from "#/modules/permissions/organizations";
import {
	MockDefaultOrganization,
	MockNoOrganizationPermissions,
	MockOrganization2,
	MockOrganizationPermissions,
	MockUserOwner,
} from "#/testHelpers/entities";
import {
	MockDisabledSkill,
	MockOrganizationSkillSettings,
	MockSkills,
} from "#/testHelpers/skills";
import {
	withAuthProvider,
	withDashboardProvider,
	withToaster,
} from "#/testHelpers/storybook";
import SkillsPage from "./SkillsPage";

const mockOrganizationSkills = [...MockSkills, MockDisabledSkill];

const secondOrganizationRoute = reactRouterParameters({
	location: {
		path: "/ai/settings/skills",
		searchParams: { org: MockOrganization2.name },
	},
	routing: { path: "/ai/settings/skills" },
});

const meta = {
	title: "pages/AISettingsPage/SkillsPage",
	component: SkillsPage,
	decorators: [withToaster, withAuthProvider, withDashboardProvider],
	parameters: {
		layout: "fullscreen",
		user: MockUserOwner,
		permissions: { viewAnyOrganizationSkills: true },
		organizations: [MockDefaultOrganization],
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/skills" },
			routing: { path: "/ai/settings/skills" },
		}),
	},
} satisfies Meta<typeof SkillsPage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const OrganizationAdmin: Story = {
	parameters: {
		queries: [
			{
				key: organizationsPermissions([MockDefaultOrganization.id]).queryKey,
				data: { [MockDefaultOrganization.id]: MockOrganizationPermissions },
			},
			{
				key: organizationSkills(MockDefaultOrganization.id).queryKey,
				data: mockOrganizationSkills,
			},
			{
				key: organizationSkillSettings(MockDefaultOrganization.id).queryKey,
				data: MockOrganizationSkillSettings,
			},
		],
	},
};

const mockMembersCanDisableSkillsSettings: OrganizationSkillSettings = {
	...MockOrganizationSkillSettings,
	members_can_disable_skills: true,
};

export const MembersCanTurnOffSkills: Story = {
	parameters: {
		queries: [
			{
				key: organizationsPermissions([MockDefaultOrganization.id]).queryKey,
				data: { [MockDefaultOrganization.id]: MockOrganizationPermissions },
			},
			{
				key: organizationSkills(MockDefaultOrganization.id).queryKey,
				data: mockOrganizationSkills,
			},
			{
				key: organizationSkillSettings(MockDefaultOrganization.id).queryKey,
				data: mockMembersCanDisableSkillsSettings,
			},
		],
	},
};

// Reading skills without updating them leaves the setting read-only.
const mockReadOnlySkillsPermissions: OrganizationPermissions = {
	...MockNoOrganizationPermissions,
	viewOrganizationSkills: true,
};

export const MembersCanTurnOffSkillsReadOnly: Story = {
	parameters: {
		queries: [
			{
				key: organizationsPermissions([MockDefaultOrganization.id]).queryKey,
				data: { [MockDefaultOrganization.id]: mockReadOnlySkillsPermissions },
			},
			{
				key: organizationSkills(MockDefaultOrganization.id).queryKey,
				data: mockOrganizationSkills,
			},
			{
				key: organizationSkillSettings(MockDefaultOrganization.id).queryKey,
				data: mockMembersCanDisableSkillsSettings,
			},
		],
	},
};

export const MultipleOrganizations: Story = {
	parameters: {
		organizations: [MockDefaultOrganization, MockOrganization2],
		reactRouter: secondOrganizationRoute,
		queries: [
			{
				key: organizationsPermissions([
					MockDefaultOrganization.id,
					MockOrganization2.id,
				]).queryKey,
				data: {
					[MockDefaultOrganization.id]: MockOrganizationPermissions,
					[MockOrganization2.id]: MockOrganizationPermissions,
				},
			},
			{
				key: organizationSkills(MockOrganization2.id).queryKey,
				data: mockOrganizationSkills,
			},
			{
				key: organizationSkillSettings(MockOrganization2.id).queryKey,
				data: MockOrganizationSkillSettings,
			},
		],
	},
};

export const DeniedOrganization: Story = {
	parameters: {
		organizations: [MockDefaultOrganization, MockOrganization2],
		reactRouter: secondOrganizationRoute,
		queries: [
			{
				key: organizationsPermissions([
					MockDefaultOrganization.id,
					MockOrganization2.id,
				]).queryKey,
				data: {
					[MockDefaultOrganization.id]: MockOrganizationPermissions,
					[MockOrganization2.id]: MockNoOrganizationPermissions,
				},
			},
		],
	},
};
