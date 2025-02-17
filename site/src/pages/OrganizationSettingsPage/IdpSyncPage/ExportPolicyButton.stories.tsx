import type { Meta, StoryObj } from "@storybook/react";
import { expect, fn, userEvent, waitFor, within } from "@storybook/test";
import {
	MockGroupSyncSettings,
	MockOrganization,
	MockRoleSyncSettings,
} from "testHelpers/entities";
import { ExportPolicyButton } from "./ExportPolicyButton";

const meta: Meta<typeof ExportPolicyButton> = {
	title: "pages/IdpSyncPage/ExportPolicyButton",
	component: ExportPolicyButton,
	args: {
		syncSettings: MockGroupSyncSettings,
		type: "groups",
		organization: MockOrganization,
	},
};

export default meta;
type Story = StoryObj<typeof ExportPolicyButton>;

export const Default: Story = {};

export const ClickExportGroupPolicy: Story = {
	args: {
		syncSettings: MockGroupSyncSettings,
		type: "groups",
		organization: MockOrganization,
		download: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Export policy" }),
		);
		await waitFor(() =>
			expect(args.download).toHaveBeenCalledWith(
				expect.anything(),
				`${MockOrganization.name}_groups-policy.json`,
			),
		);
		const blob: Blob = (args.download as jest.Mock).mock.lastCall[0];
		await expect(blob.type).toEqual("application/json");
		await expect(await blob.text()).toEqual(
			JSON.stringify(MockGroupSyncSettings, null, 2),
		);
	},
};

export const ClickExportRolePolicy: Story = {
	args: {
		syncSettings: MockRoleSyncSettings,
		type: "roles",
		organization: MockOrganization,
		download: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Export policy" }),
		);
		await waitFor(() =>
			expect(args.download).toHaveBeenCalledWith(
				expect.anything(),
				`${MockOrganization.name}_roles-policy.json`,
			),
		);
		const blob: Blob = (args.download as jest.Mock).mock.lastCall[0];
		await expect(blob.type).toEqual("application/json");
		await expect(await blob.text()).toEqual(
			JSON.stringify(MockRoleSyncSettings, null, 2),
		);
	},
};
