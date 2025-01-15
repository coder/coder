import type { Meta, StoryObj } from "@storybook/react";
import { expect, userEvent, waitFor, within } from "@storybook/test";
import {
	MockNoPermissions,
	MockOrganization,
	MockOrganization2,
	MockPermissions,
} from "testHelpers/entities";
import { withDashboardProvider } from "testHelpers/storybook";
import { OrganizationSidebarView } from "./OrganizationSidebarView";

const meta: Meta<typeof OrganizationSidebarView> = {
	title: "modules/management/OrganizationSidebarView",
	component: OrganizationSidebarView,
	decorators: [withDashboardProvider],
	parameters: { showOrganizations: true },
	args: {
		activeOrganization: undefined,
		organizations: [
			{
				...MockOrganization,
				permissions: {
					editOrganization: true,
					editMembers: true,
					editGroups: true,
					auditOrganization: true,
				},
			},
			{
				...MockOrganization2,
				permissions: {
					editOrganization: true,
					editMembers: true,
					editGroups: true,
					auditOrganization: true,
				},
			},
		],
		permissions: MockPermissions,
	},
};

export default meta;
type Story = StoryObj<typeof OrganizationSidebarView>;

export const LoadingOrganizations: Story = {
	args: {
		organizations: undefined,
	},
};

export const NoCreateOrg: Story = {
	args: {
		activeOrganization: {
			...MockOrganization,
			permissions: { createOrganization: false },
		},
		permissions: {
			...MockPermissions,
			createOrganization: false,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: /My Organization/i }),
		);
		await waitFor(() =>
			expect(canvas.queryByText("Create Organization")).not.toBeInTheDocument(),
		);
	},
};

export const OverflowDropdown: Story = {
	args: {
		activeOrganization: {
			...MockOrganization,
			permissions: { createOrganization: true },
		},
		permissions: {
			...MockPermissions,
			createOrganization: true,
		},
		organizations: [
			{
				...MockOrganization,
				permissions: {},
			},
			{
				...MockOrganization2,
				permissions: {},
			},
			{
				id: "my-organization-3-id",
				name: "my-organization-3",
				display_name: "My Organization 3",
				description: "Another organization that gets used for stuff.",
				icon: "/emojis/1f957.png",
				created_at: "",
				updated_at: "",
				is_default: false,
				permissions: {},
			},
			{
				id: "my-organization-4-id",
				name: "my-organization-4",
				display_name: "My Organization 4",
				description: "Another organization that gets used for stuff.",
				icon: "/emojis/1f957.png",
				created_at: "",
				updated_at: "",
				is_default: false,
				permissions: {},
			},
			{
				id: "my-organization-5-id",
				name: "my-organization-5",
				display_name: "My Organization 5",
				description: "Another organization that gets used for stuff.",
				icon: "/emojis/1f957.png",
				created_at: "",
				updated_at: "",
				is_default: false,
				permissions: {},
			},
			{
				id: "my-organization-6-id",
				name: "my-organization-6",
				display_name: "My Organization 6",
				description: "Another organization that gets used for stuff.",
				icon: "/emojis/1f957.png",
				created_at: "",
				updated_at: "",
				is_default: false,
				permissions: {},
			},
			{
				id: "my-organization-7-id",
				name: "my-organization-7",
				display_name: "My Organization 7",
				description: "Another organization that gets used for stuff.",
				icon: "/emojis/1f957.png",
				created_at: "",
				updated_at: "",
				is_default: false,
				permissions: {},
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: /My Organization/i }),
		);
	},
};

export const NoPermissions: Story = {
	args: {
		activeOrganization: {
			...MockOrganization,
			permissions: MockNoPermissions,
		},
		permissions: MockNoPermissions,
	},
};

export const AllPermissions: Story = {
	args: {
		activeOrganization: {
			...MockOrganization,
			permissions: {
				editOrganization: true,
				editMembers: true,
				editGroups: true,
				auditOrganization: true,
				assignOrgRole: true,
				viewProvisioners: true,
				viewIdpSyncSettings: true,
			},
		},
		organizations: [
			{
				...MockOrganization,
				permissions: {
					editOrganization: true,
					editMembers: true,
					editGroups: true,
					auditOrganization: true,
					assignOrgRole: true,
					viewProvisioners: true,
					viewIdpSyncSettings: true,
				},
			},
		],
	},
};

export const SelectedOrgAdmin: Story = {
	args: {
		activeOrganization: {
			...MockOrganization,
			permissions: {
				editOrganization: true,
				editMembers: true,
				editGroups: true,
				auditOrganization: true,
				assignOrgRole: true,
			},
		},
		organizations: [
			{
				...MockOrganization,
				permissions: {
					editOrganization: true,
					editMembers: true,
					editGroups: true,
					auditOrganization: true,
					assignOrgRole: true,
				},
			},
		],
	},
};

export const SelectedOrgAuditor: Story = {
	args: {
		activeOrganization: {
			...MockOrganization,
			permissions: {
				editOrganization: false,
				editMembers: false,
				editGroups: false,
				auditOrganization: true,
			},
		},
		permissions: {
			...MockPermissions,
			createOrganization: false,
		},
		organizations: [
			{
				...MockOrganization,
				permissions: {
					editOrganization: false,
					editMembers: false,
					editGroups: false,
					auditOrganization: true,
				},
			},
		],
	},
};

export const SelectedOrgUserAdmin: Story = {
	args: {
		activeOrganization: {
			...MockOrganization,
			permissions: {
				editOrganization: false,
				editMembers: true,
				editGroups: true,
				auditOrganization: false,
			},
		},
		permissions: {
			...MockPermissions,
			createOrganization: false,
		},
		organizations: [
			{
				...MockOrganization,
				permissions: {
					editOrganization: false,
					editMembers: true,
					editGroups: true,
					auditOrganization: false,
				},
			},
		],
	},
};

export const OrgsDisabled: Story = {
	parameters: {
		showOrganizations: false,
	},
};
