import type { Meta, StoryObj } from "@storybook/react";
import {
	MockOrganization,
	MockOrganization2,
	MockOrganizationSyncSettings,
	MockOrganizationSyncSettings2,
} from "testHelpers/entities";
import { IdpOrgSyncPageView } from "./IdpOrgSyncPageView";

const meta: Meta<typeof IdpOrgSyncPageView> = {
	title: "pages/IdpOrgSyncPageView",
	component: IdpOrgSyncPageView,
};

export default meta;
type Story = StoryObj<typeof IdpOrgSyncPageView>;

export const Empty: Story = {
	args: {
		organizationSyncSettings: {
			field: "",
			mapping: {},
			organization_assign_default: true,
		},
		organizations: [MockOrganization, MockOrganization2],
		error: undefined,
	},
};

export const Default: Story = {
	args: {
		organizationSyncSettings: MockOrganizationSyncSettings2,
		organizations: [MockOrganization, MockOrganization2],
		error: undefined,
	},
};

export const HasError: Story = {
	args: {
		...Default.args,
		error: "This is a test error",
	},
};

export const MissingGroups: Story = {
	args: {
		...Default.args,
		organizationSyncSettings: MockOrganizationSyncSettings,
	},
};
