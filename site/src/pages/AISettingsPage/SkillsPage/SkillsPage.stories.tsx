import type { Meta, StoryObj } from "@storybook/react-vite";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { organizationsPermissions } from "#/api/queries/organizations";
import { skillList } from "#/api/queries/skills";
import {
	MockDefaultOrganization,
	MockNoOrganizationPermissions,
	MockOrganization2,
	MockOrganizationPermissions,
	MockUserOwner,
} from "#/testHelpers/entities";
import { MockDisabledSkill, MockSkills } from "#/testHelpers/skills";
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
				key: skillList({
					type: "organization",
					organizationId: MockDefaultOrganization.id,
				}).queryKey,
				data: mockOrganizationSkills,
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
				key: skillList({
					type: "organization",
					organizationId: MockOrganization2.id,
				}).queryKey,
				data: mockOrganizationSkills,
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
