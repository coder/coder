import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, spyOn, userEvent, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import { organizationsPermissions } from "#/api/queries/organizations";
import { skillsKey } from "#/api/queries/skills";
import {
	MockDefaultOrganization,
	MockGroup,
	MockNoOrganizationPermissions,
	MockOrganization2,
	MockOrganizationPermissions,
	MockOrganizationSkillACL,
	MockUserOwner,
	mockApiError,
} from "#/testHelpers/entities";
import { MockSkills } from "#/testHelpers/skills";
import {
	withAuthProvider,
	withDashboardProvider,
	withToaster,
} from "#/testHelpers/storybook";
import SkillsPage from "./SkillsPage";

const mockDisabledSkill = {
	...MockSkills[0],
	id: "skill-legacy-review",
	name: "legacy-review",
	description: "Older review checklist kept for reference.",
	enabled: false,
};

const mockOrganizationSkills = [...MockSkills, mockDisabledSkill];

const openFirstRowMenu = async (canvasElement: HTMLElement) => {
	const canvas = within(canvasElement);
	const [menuButton] = await canvas.findAllByRole("button", {
		name: "Open menu",
	});
	await userEvent.click(menuButton);
};

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
				key: skillsKey({
					type: "organization",
					organizationId: MockDefaultOrganization.id,
				}),
				data: mockOrganizationSkills,
			},
		],
	},
};

export const OrganizationAuditor: Story = {
	parameters: {
		queries: [
			{
				key: organizationsPermissions([MockDefaultOrganization.id]).queryKey,
				data: {
					[MockDefaultOrganization.id]: {
						...MockNoOrganizationPermissions,
						viewOrganizationSkills: true,
					},
				},
			},
			{
				key: skillsKey({
					type: "organization",
					organizationId: MockDefaultOrganization.id,
				}),
				data: mockOrganizationSkills,
			},
		],
	},
};

export const OrganizationAdminRowMenu: Story = {
	parameters: OrganizationAdmin.parameters,
	play: async ({ canvasElement }) => {
		await openFirstRowMenu(canvasElement);
	},
};

export const OrganizationAuditorRowMenu: Story = {
	parameters: OrganizationAuditor.parameters,
	play: async ({ canvasElement }) => {
		await openFirstRowMenu(canvasElement);
	},
};

export const ManagePermissions: Story = {
	parameters: OrganizationAdmin.parameters,
	beforeEach: () => {
		spyOn(API.experimental, "getOrganizationSkillACL").mockResolvedValue(
			MockOrganizationSkillACL,
		);
	},
	play: async ({ canvasElement }) => {
		await openFirstRowMenu(canvasElement);
		await userEvent.click(
			await screen.findByRole("menuitem", { name: "Manage permissions" }),
		);
		await screen.findByRole("button", {
			name: `Remove ${MockGroup.display_name}`,
		});
	},
};

export const Empty: Story = {
	parameters: {
		queries: [
			{
				key: organizationsPermissions([MockDefaultOrganization.id]).queryKey,
				data: { [MockDefaultOrganization.id]: MockOrganizationPermissions },
			},
			{
				key: skillsKey({
					type: "organization",
					organizationId: MockDefaultOrganization.id,
				}),
				data: [],
			},
		],
	},
};

export const MultipleOrganizations: Story = {
	parameters: {
		organizations: [MockDefaultOrganization, MockOrganization2],
		reactRouter: reactRouterParameters({
			location: {
				path: "/ai/settings/skills",
				searchParams: { org: MockOrganization2.name },
			},
			routing: { path: "/ai/settings/skills" },
		}),
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
				key: skillsKey({
					type: "organization",
					organizationId: MockOrganization2.id,
				}),
				data: mockOrganizationSkills,
			},
		],
	},
};

export const DeniedOrganization: Story = {
	parameters: {
		organizations: [MockDefaultOrganization, MockOrganization2],
		reactRouter: reactRouterParameters({
			location: {
				path: "/ai/settings/skills",
				searchParams: { org: MockOrganization2.name },
			},
			routing: { path: "/ai/settings/skills" },
		}),
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

export const PermissionsLoadError: Story = {
	beforeEach: () => {
		spyOn(API, "checkAuthorization").mockRejectedValue(
			mockApiError({ message: "Failed to load organization permissions." }),
		);
	},
	play: async ({ canvasElement }) => {
		await within(canvasElement).findByText(
			"Failed to load organization permissions.",
		);
	},
};
