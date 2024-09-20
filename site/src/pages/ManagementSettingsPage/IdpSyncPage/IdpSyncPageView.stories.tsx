import type { Meta, StoryObj } from "@storybook/react";
import {
	MockGroup,
	MockGroup2,
	MockGroupSyncSettings,
	MockRoleSyncSettings,
} from "testHelpers/entities";
import { IdpSyncPageView } from "./IdpSyncPageView";

const meta: Meta<typeof IdpSyncPageView> = {
	title: "pages/OrganizationIdpSyncPage",
	component: IdpSyncPageView,
};

export default meta;
type Story = StoryObj<typeof IdpSyncPageView>;

export const Empty: Story = {
	args: {
		groupSyncSettings: undefined,
		roleSyncSettings: undefined,
		groups: [MockGroup, MockGroup2],
	},
};

export const Default: Story = {
	args: {
		groupSyncSettings: MockGroupSyncSettings,
		roleSyncSettings: MockRoleSyncSettings,
		groups: [MockGroup, MockGroup2],
	},
};
