import type { Meta, StoryObj } from "@storybook/react-vite";
import { AIGovernanceAddOnCard } from "./AIGovernanceAddOnCard";

const meta: Meta<typeof AIGovernanceAddOnCard> = {
	title:
		"pages/DeploymentSettingsPage/LicensesSettingsPage/AIGovernanceAddOnCard",
	component: AIGovernanceAddOnCard,
	args: {
		title: "AI governance",
		unit: "Seats",
		actual: 750,
		limit: 1000,
		isExceeded: false,
	},
};

export default meta;
type Story = StoryObj<typeof AIGovernanceAddOnCard>;

export const Default: Story = {};

export const Exceeded: Story = {
	args: {
		actual: 1200,
		limit: 1000,
		isExceeded: true,
	},
};
