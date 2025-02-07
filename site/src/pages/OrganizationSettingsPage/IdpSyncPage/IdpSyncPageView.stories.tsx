import type { Meta, StoryObj } from "@storybook/react";
import { expect, userEvent, within } from "@storybook/test";
import {
	MockGroup,
	MockGroup2,
	MockGroupSyncSettings,
	MockGroupSyncSettings2,
	MockLegacyMappingGroupSyncSettings,
	MockOrganization,
	MockRoleSyncSettings,
} from "testHelpers/entities";
import { IdpSyncPageView } from "./IdpSyncPageView";

const groupsMap = new Map<string, string>();
for (const group of [MockGroup, MockGroup2]) {
	groupsMap.set(group.id, group.display_name || group.name);
}

const meta: Meta<typeof IdpSyncPageView> = {
	title: "pages/IdpSyncPage",
	component: IdpSyncPageView,
	args: {
		tab: "groups",
		groupSyncSettings: MockGroupSyncSettings,
		roleSyncSettings: MockRoleSyncSettings,
		fieldValues: [
			...Object.keys(MockGroupSyncSettings.mapping),
			...Object.keys(MockRoleSyncSettings.mapping),
		],
		groups: [MockGroup, MockGroup2],
		groupsMap,
		organization: MockOrganization,
		error: undefined,
	},
};

export default meta;
type Story = StoryObj<typeof IdpSyncPageView>;

export const Empty: Story = {
	args: {
		groupSyncSettings: {
			field: "",
			mapping: {},
			regex_filter: "",
			auto_create_missing_groups: false,
		},
		roleSyncSettings: {
			field: "",
			mapping: {},
		},
		groups: [],
		groupsMap: undefined,
		organization: MockOrganization,
		error: undefined,
	},
};

export const Default: Story = {};

export const HasError: Story = {
	args: {
		error: "This is a test error",
	},
};

export const MissingGroups: Story = {
	args: {
		groupSyncSettings: MockGroupSyncSettings2,
	},
};

export const WithLegacyMapping: Story = {
	args: {
		groupSyncSettings: MockLegacyMappingGroupSyncSettings,
		fieldValues: Object.keys(
			MockLegacyMappingGroupSyncSettings.legacy_group_name_mapping,
		),
	},
};

export const GroupsTabMissingClaims: Story = {
	args: {
		fieldValues: [],
	},
};

export const RolesTab: Story = {
	args: {
		tab: "roles",
	},
};

export const RolesTabMissingClaims: Story = {
	args: {
		tab: "roles",
		fieldValues: [],
	},
};
