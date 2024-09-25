import type { Meta, StoryObj } from "@storybook/react";
import {
	MockGroup,
	MockGroup2,
	MockGroupSyncSettings,
	MockGroupSyncSettings2,
	MockOrganization,
	MockRoleSyncSettings,
} from "testHelpers/entities";
import { IdpSyncPageView } from "./IdpSyncPageView";

const meta: Meta<typeof IdpSyncPageView> = {
	title: "pages/OrganizationIdpSyncPage",
	component: IdpSyncPageView,
};

export default meta;
type Story = StoryObj<typeof IdpSyncPageView>;

const groupsMap = new Map<string, string>();

for (const group of [MockGroup, MockGroup2]) {
	groupsMap.set(group.id, group.display_name || group.name);
}

export const NotEnabled: Story = {
	args: {
		groupSyncSettings: undefined,
		roleSyncSettings: undefined,
		groupsMap: undefined,
		organization: MockOrganization,
		isIdpSyncEnabled: false,
	},
};

export const Empty: Story = {
	args: {
		groupSyncSettings: undefined,
		roleSyncSettings: undefined,
		groupsMap: undefined,
		organization: MockOrganization,
		isIdpSyncEnabled: true,
	},
};

export const Default: Story = {
	args: {
		groupSyncSettings: MockGroupSyncSettings,
		roleSyncSettings: MockRoleSyncSettings,
		groupsMap,
		organization: MockOrganization,
		isIdpSyncEnabled: true,
	},
};

export const MissingGroups: Story = {
	args: {
		groupSyncSettings: MockGroupSyncSettings2,
		roleSyncSettings: MockRoleSyncSettings,
		groupsMap,
		organization: MockOrganization,
		isIdpSyncEnabled: true,
	},
};
