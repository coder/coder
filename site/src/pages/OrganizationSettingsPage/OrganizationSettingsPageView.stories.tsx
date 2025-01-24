import type { Meta, StoryObj } from "@storybook/react";
import { chromatic } from "testHelpers/chromatic";
import {
	MockDefaultOrganization,
	MockOrganization,
} from "testHelpers/entities";
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
