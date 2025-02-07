import type { Meta, StoryObj } from "@storybook/react";
import { expect, userEvent, within } from "@storybook/test";
import {
	MockOrganization,
	MockOrganization2,
	MockOrganizationSyncSettings,
	MockOrganizationSyncSettings2,
	MockOrganizationSyncSettingsEmpty,
} from "testHelpers/entities";
import { IdpOrgSyncPageView } from "./IdpOrgSyncPageView";

const meta: Meta<typeof IdpOrgSyncPageView> = {
	title: "pages/IdpOrgSyncPageView",
	component: IdpOrgSyncPageView,
	args: {
		organizationSyncSettings: MockOrganizationSyncSettings2,
		claimFieldValues: Object.keys(MockOrganizationSyncSettings2.mapping),
		organizations: [MockOrganization, MockOrganization2],
		error: undefined,
	},
};

export default meta;
type Story = StoryObj<typeof IdpOrgSyncPageView>;

export const Empty: Story = {
	args: {
		organizationSyncSettings: MockOrganizationSyncSettingsEmpty,
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
		organizationSyncSettings: MockOrganizationSyncSettings,
		claimFieldValues: Object.keys(MockOrganizationSyncSettings.mapping),
		organizations: [],
	},
};

export const MissingClaims: Story = {
	args: {
		claimFieldValues: [],
	},
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const warning = canvasElement.querySelector(".lucide-triangle-alert")!;
		expect(warning).not.toBe(null);
		await user.hover(warning);
	},
};

export const AssignDefaultOrgWarningDialog: Story = {
	args: {
		organizationSyncSettings: MockOrganizationSyncSettings,
		organizations: [MockOrganization, MockOrganization2],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("switch", {
				name: "Assign Default Organization",
			}),
		);
	},
};
