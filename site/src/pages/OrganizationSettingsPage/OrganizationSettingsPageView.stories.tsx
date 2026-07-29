import type { Meta, StoryObj } from "@storybook/react-vite";
import { action } from "storybook/actions";
import { expect, userEvent, within } from "storybook/test";
import { chromatic } from "#/testHelpers/chromatic";
import {
	MockDefaultOrganization,
	MockOrganization,
} from "#/testHelpers/entities";
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

export const EditInfoFields: Story = {
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);

		for (const name of ["Slug", "Display name", "Description", "Icon"]) {
			const field = canvas.getByRole("textbox", { name });
			await user.clear(field);
			await user.type(field, "edited");
			await expect(field).toHaveValue("edited");
		}

		const slug = canvas.getByRole("textbox", { name: "Slug" });
		await user.type(slug, " invalid!");
		await user.tab();
		await expect(slug).toHaveAttribute("aria-invalid", "true");
	},
};

export const DefaultOrg: Story = {
	args: {
		organization: MockDefaultOrganization,
	},
};

export const SharingDisabled: Story = {
	args: {
		shareableWorkspaceOwners: "none",
		onChangeShareableOwners: action("onChangeShareableOwners"),
	},
};

export const SharingServiceAccountsOnly: Story = {
	args: {
		shareableWorkspaceOwners: "service_accounts",
		onChangeShareableOwners: action("onChangeShareableOwners"),
	},
};

export const SharingEveryone: Story = {
	args: {
		shareableWorkspaceOwners: "everyone",
		onChangeShareableOwners: action("onChangeShareableOwners"),
	},
};

export const SharingGloballyDisabled: Story = {
	args: {
		shareableWorkspaceOwners: "none",
		workspaceSharingGloballyDisabled: true,
		onChangeShareableOwners: action("onChangeShareableOwners"),
	},
};

export const DisableSharingDialog: Story = {
	args: {
		shareableWorkspaceOwners: "everyone",
		onChangeShareableOwners: action("onChangeShareableOwners"),
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
		onChangeShareableOwners: action("onChangeShareableOwners"),
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
