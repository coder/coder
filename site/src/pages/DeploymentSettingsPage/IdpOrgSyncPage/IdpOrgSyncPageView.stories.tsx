import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent, within } from "storybook/test";
import {
	MockOrganization,
	MockOrganization2,
	MockOrganization3,
	MockOrganizationSyncSettings,
	MockOrganizationSyncSettings2,
	MockOrganizationSyncSettingsEmpty,
} from "#/testHelpers/entities";
import { IdpOrgSyncPageView } from "./IdpOrgSyncPageView";

const meta: Meta<typeof IdpOrgSyncPageView> = {
	title: "pages/IdpOrgSyncPageView",
	component: IdpOrgSyncPageView,
	args: {
		organizationSyncSettings: MockOrganizationSyncSettings2,
		claimFieldValues: Object.keys(MockOrganizationSyncSettings2.mapping),
		organizations: [MockOrganization, MockOrganization2, MockOrganization3],
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
		const canvas = within(canvasElement);
		const warnings = canvas.getAllByRole("button", {
			name: "Unknown claim value",
		});
		const warning = warnings[0];
		if (!warning) {
			throw new Error("Expected an unknown claim warning");
		}
		await userEvent.hover(warning);
		await screen.findByRole("tooltip", {
			name: /has not be seen in the specified claim field/i,
		});
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
