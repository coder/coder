import type { Meta, StoryObj } from "@storybook/react";
import { MockBuildInfo, MockProvisioner } from "testHelpers/entities";
import { ProvisionerVersion } from "./ProvisionerVersion";
import { userEvent, within, expect } from "@storybook/test";

const meta: Meta<typeof ProvisionerVersion> = {
	title: "pages/OrganizationProvisionersPage/ProvisionerVersion",
	component: ProvisionerVersion,
	args: {
		provisionerVersion: MockProvisioner.version,
		buildVersion: MockBuildInfo.version,
	},
};

export default meta;
type Story = StoryObj<typeof ProvisionerVersion>;

export const UpToDate: Story = {};

export const Outdated: Story = {
	args: {
		provisionerVersion: "0.0.0",
		buildVersion: MockBuildInfo.version,
	},
};

export const OnFocus: Story = {
	args: {
		provisionerVersion: "0.0.0",
		buildVersion: MockBuildInfo.version,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const version = canvas.getByText(/outdated/i);
		await userEvent.tab();
		expect(version).toHaveFocus();
	},
};
