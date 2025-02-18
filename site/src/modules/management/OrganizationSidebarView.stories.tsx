import type { Meta, StoryObj } from "@storybook/react";
import { expect, userEvent, waitFor, within } from "@storybook/test";
import {
	MockNoOrganizationPermissions,
	MockNoPermissions,
	MockOrganization,
	MockOrganization2,
	MockOrganizationPermissions,
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
		organizations: [MockOrganization, MockOrganization2],
		permissions: MockPermissions,
	},
};

export default meta;
type Story = StoryObj<typeof OrganizationSidebarView>;

export const NoCreateOrg: Story = {
	args: {
		activeOrganization: MockOrganization,
		orgPermissions: MockNoOrganizationPermissions,
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
		activeOrganization: MockOrganization,
		orgPermissions: MockOrganizationPermissions,
		permissions: {
			...MockPermissions,
			createOrganization: true,
		},
		organizations: [
			MockOrganization,
			MockOrganization2,
			{
				id: "my-organization-3-id",
				name: "my-organization-3",
				display_name: "My Organization 3",
				description: "Another organization that gets used for stuff.",
				icon: "/emojis/1f957.png",
				created_at: "",
				updated_at: "",
				is_default: false,
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
		activeOrganization: MockOrganization,
		orgPermissions: MockNoOrganizationPermissions,
		permissions: MockNoPermissions,
	},
};

export const AllPermissions: Story = {
	args: {
		activeOrganization: MockOrganization,
		orgPermissions: MockOrganizationPermissions,
		organizations: [MockOrganization],
	},
};

export const SelectedOrgAdmin: Story = {
	args: {
		activeOrganization: MockOrganization,
		orgPermissions: MockOrganizationPermissions,
		organizations: [MockOrganization],
	},
};

export const SelectedOrgAuditor: Story = {
	args: {
		activeOrganization: MockOrganization,
		orgPermissions: MockNoOrganizationPermissions,
		permissions: {
			...MockPermissions,
			createOrganization: false,
		},
		organizations: [MockOrganization],
	},
};

export const SelectedOrgUserAdmin: Story = {
	args: {
		activeOrganization: MockOrganization,
		orgPermissions: {
			...MockNoOrganizationPermissions,
			viewMembers: true,
			viewGroups: true,
			viewOrgRoles: true,
			viewProvisioners: true,
			viewIdpSyncSettings: true,
		},
		permissions: {
			...MockPermissions,
			createOrganization: false,
		},
		organizations: [MockOrganization],
	},
};

export const OrgsDisabled: Story = {
	parameters: {
		showOrganizations: false,
	},
};
