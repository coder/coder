import type { Meta, StoryObj } from "@storybook/react";
import { ProvisionerKey } from "./ProvisionerKey";
import { userEvent } from "@storybook/test";
import {
	ProvisionerKeyNameBuiltIn,
	ProvisionerKeyNamePSK,
	ProvisionerKeyNameUserAuth,
} from "api/typesGenerated";

const meta: Meta<typeof ProvisionerKey> = {
	title: "pages/OrganizationProvisionersPage/ProvisionerKey",
	component: ProvisionerKey,
};

export default meta;
type Story = StoryObj<typeof ProvisionerKey>;

export const Key: Story = {
	args: {
		name: "gke-dogfood-v2-coder",
	},
};

export const BuiltIn: Story = {
	args: {
		name: ProvisionerKeyNameBuiltIn,
	},
	play: async () => {
		await userEvent.tab();
	},
};

export const UserAuth: Story = {
	args: {
		name: ProvisionerKeyNameUserAuth,
	},
	play: async () => {
		await userEvent.tab();
	},
};

export const PSK: Story = {
	args: {
		name: ProvisionerKeyNamePSK,
	},
	play: async () => {
		await userEvent.tab();
	},
};
