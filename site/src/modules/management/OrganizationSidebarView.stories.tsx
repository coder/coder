import type { Meta, StoryObj } from "@storybook/react";
import { expect, userEvent, waitFor, within } from "@storybook/test";
import type { AuthorizationResponse } from "api/typesGenerated";
import {
	MockNoPermissions,
	MockOrganization,
	MockOrganization2,
	MockPermissions,
} from "testHelpers/entities";
import { withDashboardProvider } from "testHelpers/storybook";
import {
	OrganizationSidebarView,
	type OrganizationWithPermissions,
} from "./OrganizationSidebarView";

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

const commonPerms: AuthorizationResponse = {
	editOrganization: true,
	editMembers: true,
	editGroups: true,
	auditOrganization: true,
};

const activeOrganization: OrganizationWithPermissions = {
	...MockOrganization,
	display_name: "Omega org",
	name: "omega",
	id: "1",
	permissions: {
		...commonPerms,
	},
};

export const OrgsSortedAlphabetically: Story = {
	args: {
		activeOrganization,
		permissions: MockPermissions,
		organizations: [
			{
				...MockOrganization,
				display_name: "Zeta Org",
				id: "2",
				name: "zeta",
				permissions: commonPerms,
			},
			{
				...MockOrganization,
				display_name: "alpha Org",
				id: "3",
				name: "alpha",
				permissions: commonPerms,
			},
			activeOrganization,
		],
	},
};

export const SearchAndSelectOrg: Story = {
	args: {
		activeOrganization,
		permissions: MockPermissions,
		organizations: [
			{
				...MockOrganization,
				display_name: "Zeta Org",
				id: "2",
				name: "zeta",
				permissions: commonPerms,
			},
			{
				...MockOrganization,
				display_name: "alpha Org",
				id: "3",
				name: "fish",
				permissions: commonPerms,
			},
			activeOrganization,
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: /Omega org/i }));

		// searchInput is not in #storybook-root so must query full document
		const globalScreen = within(document.body);
		const searchInput =
			await globalScreen.getByPlaceholderText("Find organization");

		await userEvent.type(searchInput, "ALPHA");

		const filteredResult = await globalScreen.findByText("alpha Org");
		expect(filteredResult).toBeInTheDocument();

		// Omega org remains visible as the default org
		await waitFor(() => {
			expect(globalScreen.queryByText("Zeta Org")).not.toBeInTheDocument();
		});
	},
};
