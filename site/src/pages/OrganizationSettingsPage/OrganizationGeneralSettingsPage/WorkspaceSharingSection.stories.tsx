import type { Meta, StoryObj } from "@storybook/react-vite";
import { action } from "storybook/actions";
import { userEvent, within } from "storybook/test";
import { MockOrganization } from "#/testHelpers/entities";
import { WorkspaceSharingSection } from "./WorkspaceSharingSection";

const meta: Meta<typeof WorkspaceSharingSection> = {
	title:
		"pages/OrganizationSettingsPage/OrganizationGeneralSettingsPage/WorkspaceSharingSection",
	component: WorkspaceSharingSection,
	args: {
		organizationId: MockOrganization.id,
		onChangeShareableOwners: action("onChangeShareableOwners"),
		isTogglingWorkspaceSharing: false,
	},
};

export default meta;
type Story = StoryObj<typeof WorkspaceSharingSection>;

export const SharingDisabled: Story = {
	args: {
		shareableWorkspaceOwners: "none",
	},
};

export const SharingServiceAccountsOnly: Story = {
	args: {
		shareableWorkspaceOwners: "service_accounts",
	},
};

export const SharingEveryone: Story = {
	args: {
		shareableWorkspaceOwners: "everyone",
	},
};

export const SharingGloballyDisabled: Story = {
	args: {
		shareableWorkspaceOwners: "none",
		workspaceSharingGloballyDisabled: true,
	},
};

export const DisableSharingDialog: Story = {
	args: {
		shareableWorkspaceOwners: "everyone",
	},
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const body = within(canvasElement.ownerDocument.body);
		const checkbox = await body.findByRole("checkbox", {
			name: /allow workspace sharing/i,
		});
		await user.click(checkbox);
	},
};

export const RestrictToServiceAccountsDialog: Story = {
	args: {
		shareableWorkspaceOwners: "everyone",
	},
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const body = within(canvasElement.ownerDocument.body);
		const radio = await body.findByRole("radio", {
			name: /only service accounts/i,
		});
		await user.click(radio);
	},
};
