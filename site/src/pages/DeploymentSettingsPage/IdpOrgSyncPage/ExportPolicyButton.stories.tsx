import type { Meta, StoryObj } from "@storybook/react";
import { expect, fn, userEvent, waitFor, within } from "@storybook/test";
import { MockOrganizationSyncSettings } from "testHelpers/entities";
import { ExportPolicyButton } from "./ExportPolicyButton";

const meta: Meta<typeof ExportPolicyButton> = {
	title: "pages/DeploymentSettingsPage/IdpOrgSyncPage/ExportPolicyButton",
	component: ExportPolicyButton,
	args: {
		syncSettings: MockOrganizationSyncSettings,
	},
};

export default meta;
type Story = StoryObj<typeof ExportPolicyButton>;

export const Default: Story = {};

export const ClickExportPolicy: Story = {
	args: {
		syncSettings: MockOrganizationSyncSettings,
		download: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Export Policy" }),
		);
		await waitFor(() =>
			expect(args.download).toHaveBeenCalledWith(
				expect.anything(),
				"organizations_policy.json",
			),
		);
		const blob: Blob = (args.download as jest.Mock).mock.lastCall[0];
		await expect(blob.type).toEqual("application/json");
		await expect(await blob.text()).toEqual(
			JSON.stringify(MockOrganizationSyncSettings, null, 2),
		);
	},
};
