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

export const Default: Story = {
	args: {
		groupSyncSettings: MockGroupSyncSettings,
		roleSyncSettings: MockRoleSyncSettings,
		groups: [MockGroup, MockGroup2],
		groupsMap,
		organization: MockOrganization,
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
		groupSyncSettings: MockGroupSyncSettings2,
	},
};

export const WithLegacyMapping: Story = {
	args: {
		...Default.args,
		groupSyncSettings: MockLegacyMappingGroupSyncSettings,
	},
};

export const RolesTab: Story = {
	args: {
		...Default.args,
	},
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const rolesTab = await canvas.findByText("Role Sync Settings");
		await user.click(rolesTab);
		await expect(canvas.findByText("IdP Role")).resolves.toBeVisible();
	},
};
