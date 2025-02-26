import type { Meta, StoryObj } from "@storybook/react";
import { expect, userEvent, waitFor, within } from "@storybook/test";
import type { Organization } from "api/typesGenerated";
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

export const NoOrganizations: Story = {
	args: {
		organizations: [],
		activeOrganization: undefined,
		orgPermissions: MockNoOrganizationPermissions,
		permissions: MockNoPermissions,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: /No organization selected/i }),
		);
	},
};

export const NoOtherOrganizations: Story = {
	args: {
		organizations: [MockOrganization],
		activeOrganization: MockOrganization,
		orgPermissions: MockNoOrganizationPermissions,
		permissions: MockNoPermissions,
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

const activeOrganization: Organization = {
	...MockOrganization,
	display_name: "Omega org",
	name: "omega",
	id: "1",
};

export const OrgsSortedAlphabetically: Story = {
	args: {
		activeOrganization,
		orgPermissions: MockOrganizationPermissions,
		permissions: {
			...MockPermissions,
			createOrganization: true,
		},
		organizations: [
			{
				...MockOrganization,
				display_name: "Zeta Org",
				id: "2",
				name: "zeta",
			},
			{
				...MockOrganization,
				display_name: "alpha Org",
				id: "3",
				name: "alpha",
			},
			activeOrganization,
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: /Omega org/i }));

		// dropdown is not in #storybook-root so must query full document
		const globalScreen = within(document.body);

		await waitFor(() => {
			expect(globalScreen.queryByText("alpha Org")).toBeInTheDocument();
			expect(globalScreen.queryByText("Zeta Org")).toBeInTheDocument();
		});

		const orgElements = globalScreen.getAllByRole("option");
		// filter out Create btn
		const filteredElems = orgElements.slice(0, 3);

		const orgNames = filteredElems.map(
			// handling fuzzy matching
			(el) => el.textContent?.replace(/^[A-Z]/, "").trim() || "",
		);

		// active name first
		expect(orgNames).toEqual(["Omega org", "alpha Org", "Zeta Org"]);
	},
};

export const SearchForOrg: Story = {
	args: {
		activeOrganization,
		permissions: MockPermissions,
		organizations: [
			{
				...MockOrganization,
				display_name: "Zeta Org",
				id: "2",
				name: "zeta",
			},
			{
				...MockOrganization,
				display_name: "alpha Org",
				id: "3",
				name: "fish",
			},
			activeOrganization,
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: /Omega org/i }));

		// dropdown is not in #storybook-root so must query full document
		const globalScreen = within(document.body);
		const searchInput =
			await globalScreen.findByPlaceholderText("Find organization");

		await userEvent.type(searchInput, "ALPHA");

		const filteredResult = await globalScreen.findByText("alpha Org");
		expect(filteredResult).toBeInTheDocument();

		// Omega org remains visible as the default org
		await waitFor(() => {
			expect(globalScreen.queryByText("Zeta Org")).not.toBeInTheDocument();
		});
	},
};
