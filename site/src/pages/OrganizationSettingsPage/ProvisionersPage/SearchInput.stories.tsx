import type { Meta, StoryObj } from "@storybook/react";
import { SearchInput } from "./SearchInput";

const meta: Meta<typeof SearchInput> = {
	title: "pages/OrganizationSettingsPage/ProvisionersPage/SearchInput",
	component: SearchInput,
	args: {
		placeholder: "Search provisioner jobs...",
	},
};

export default meta;
type Story = StoryObj<typeof SearchInput>;

export const Default: Story = {};
