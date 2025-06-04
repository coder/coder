import type { Meta, StoryObj } from "@storybook/react";
import { userEvent } from "@storybook/test";
import { LastConnectionHead } from "./LastConnectionHead";

const meta: Meta<typeof LastConnectionHead> = {
	title: "pages/OrganizationProvisionersPage/LastConnectionHead",
	component: LastConnectionHead,
};

export default meta;
type Story = StoryObj<typeof LastConnectionHead>;

export const Default: Story = {};

export const OnFocus: Story = {
	play: async () => {
		await userEvent.tab();
	},
};
