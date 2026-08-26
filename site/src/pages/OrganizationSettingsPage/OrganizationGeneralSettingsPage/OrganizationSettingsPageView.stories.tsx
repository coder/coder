import type { Meta, StoryObj } from "@storybook/react-vite";
import { action } from "storybook/actions";
import {
	MockDefaultOrganization,
	MockOrganization,
} from "#/testHelpers/entities";
import { OrganizationSettingsPageView } from "./OrganizationSettingsPageView";

const meta: Meta<typeof OrganizationSettingsPageView> = {
	title:
		"pages/OrganizationSettingsPage/OrganizationGeneralSettingsPage/OrganizationSettingsPageView",
	component: OrganizationSettingsPageView,
	args: {
		organization: MockOrganization,
		onSubmit: action("onSubmit"),
		onDeleteOrganization: action("onDeleteOrganization"),
		shareableWorkspaceOwners: "everyone",
		onChangeShareableOwners: action("onChangeShareableOwners"),
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
