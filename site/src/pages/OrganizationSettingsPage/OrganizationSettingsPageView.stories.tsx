import { chromatic } from "testHelpers/chromatic";
import {
	MockDefaultOrganization,
	MockOrganization,
} from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { within } from "@testing-library/react";
import { action } from "storybook/actions";
import { userEvent } from "storybook/test";
import { OrganizationSettingsPageView } from "./OrganizationSettingsPageView";

const meta: Meta<typeof OrganizationSettingsPageView> = {
	title: "pages/OrganizationSettingsPageView",
	component: OrganizationSettingsPageView,
	parameters: { chromatic },
	args: {
		organization: MockOrganization,
	},
};

export default meta;
type Story = StoryObj<typeof OrganizationSettingsPageView>;

export const Example: Story = {};

export const DefaultOrg: Story = {
	args: {
		organization: MockDefaultOrganization,
	},
};

export const WithWorkspaceSharingEnabled: Story = {
	args: {
		workspaceSharingEnabled: true,
		onToggleWorkspaceSharing: action("onToggleWorkspaceSharing"),
	},
};

export const WithWorkspaceSharingDisabled: Story = {
	args: {
		workspaceSharingEnabled: false,
		onToggleWorkspaceSharing: action("onToggleWorkspaceSharing"),
	},
};

export const DisableSharingDialog: Story = {
	args: {
		workspaceSharingEnabled: true,
		onToggleWorkspaceSharing: action("onToggleWorkspaceSharing"),
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
