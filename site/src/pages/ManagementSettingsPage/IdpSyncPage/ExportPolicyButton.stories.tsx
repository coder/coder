import type { Meta, StoryObj } from "@storybook/react";
import { expect, fn, userEvent, waitFor, within } from "@storybook/test";
import {
	MockGroupSyncSettings,
	MockOrganization,
	MockRoleSyncSettings,
} from "testHelpers/entities";
import { ExportPolicyButton } from "./ExportPolicyButton";

const meta: Meta<typeof ExportPolicyButton> = {
	title: "modules/resources/ExportPolicyButton",
	component: ExportPolicyButton,
	args: {
		policy: JSON.stringify(MockGroupSyncSettings, null, 2),
		type: "groups",
		organization: MockOrganization,
	},
};

export default meta;
type Story = StoryObj<typeof ExportPolicyButton>;

export const Default: Story = {};

export const ClickExportGroupPolicy: Story = {
	args: {
		policy: JSON.stringify(MockGroupSyncSettings, null, 2),
		type: "groups",
		organization: MockOrganization,
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
				`${MockOrganization.name}_groups-policy.json`,
			),
		);
		const blob: Blob = (args.download as jest.Mock).mock.calls[0][0];
		await expect(blob.type).toEqual("application/json");
	},
};

export const ClickExportRolePolicy: Story = {
	args: {
		policy: JSON.stringify(MockRoleSyncSettings, null, 2),
		type: "roles",
		organization: MockOrganization,
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
				`${MockOrganization.name}_roles-policy.json`,
			),
		);
		const blob: Blob = (args.download as jest.Mock).mock.calls[0][0];
		await expect(blob.type).toEqual("application/json");
	},
};
