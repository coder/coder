import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import type { Mock } from "vitest";
import {
	MockGroupSyncSettings,
	MockOrganization,
	MockOrganizationSyncSettings,
	MockRoleSyncSettings,
} from "#/testHelpers/entities";
import { ExportPolicyButton } from "./ExportPolicyButton";

const meta: Meta<typeof ExportPolicyButton> = {
	title: "modules/idpSync/ExportPolicyButton",
	component: ExportPolicyButton,
	args: {
		syncSettings: MockGroupSyncSettings,
		filename: `${MockOrganization.name}_groups-policy.json`,
		size: "sm",
	},
};

export default meta;
type Story = StoryObj<typeof ExportPolicyButton>;

export const Default: Story = {};

const expectExportedPolicy = async (
	canvasElement: HTMLElement,
	download: Mock | undefined,
	filename: string,
	settings: unknown,
) => {
	const canvas = within(canvasElement);
	await expect(download).toBeDefined();
	if (!download) {
		return;
	}

	await userEvent.click(canvas.getByRole("button", { name: "Export policy" }));
	await waitFor(() =>
		expect(download).toHaveBeenCalledWith(expect.anything(), filename),
	);
	const [blob] = download.mock.lastCall ?? [];
	await expect(blob).toBeInstanceOf(Blob);
	if (!(blob instanceof Blob)) {
		return;
	}
	await expect(blob.type).toEqual("application/json");
	await expect(await blob.text()).toEqual(JSON.stringify(settings, null, 2));
};

export const ClickExportGroupPolicy: Story = {
	args: {
		syncSettings: MockGroupSyncSettings,
		filename: `${MockOrganization.name}_groups-policy.json`,
		size: "sm",
		download: fn(),
	},
	play: async ({ canvasElement, args }) => {
		await expectExportedPolicy(
			canvasElement,
			args.download as Mock | undefined,
			`${MockOrganization.name}_groups-policy.json`,
			MockGroupSyncSettings,
		);
	},
};

export const ClickExportRolePolicy: Story = {
	args: {
		syncSettings: MockRoleSyncSettings,
		filename: `${MockOrganization.name}_roles-policy.json`,
		size: "sm",
		download: fn(),
	},
	play: async ({ canvasElement, args }) => {
		await expectExportedPolicy(
			canvasElement,
			args.download as Mock | undefined,
			`${MockOrganization.name}_roles-policy.json`,
			MockRoleSyncSettings,
		);
	},
};

export const ClickExportOrganizationPolicy: Story = {
	args: {
		syncSettings: MockOrganizationSyncSettings,
		filename: "organizations_policy.json",
		download: fn(),
	},
	play: async ({ canvasElement, args }) => {
		await expectExportedPolicy(
			canvasElement,
			args.download as Mock | undefined,
			"organizations_policy.json",
			MockOrganizationSyncSettings,
		);
	},
};
