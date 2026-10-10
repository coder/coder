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
import { MockSkills } from "#/testHelpers/skills";
import {
	withAuthProvider,
	withDashboardProvider,
	withToaster,
} from "#/testHelpers/storybook";
import SkillsPage from "./SkillsPage";

const mockOrganizationSkills = [
	...MockSkills,
	{
		...MockSkills[0],
		id: "skill-legacy-review",
		name: "legacy-review",
		description: "Older review checklist kept for reference.",
		enabled: false,
	},
];

const organizationSkillsQuery = (organizationId: string) => ({
	key: skillList({ type: "organization", organizationId }).queryKey,
	data: mockOrganizationSkills,
});

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
			organizationSkillsQuery(MockDefaultOrganization.id),
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
			organizationSkillsQuery(MockOrganization2.id),
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
