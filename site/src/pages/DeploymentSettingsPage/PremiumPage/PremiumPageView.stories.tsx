import type { Meta, StoryObj } from "@storybook/react-vite";
import { PremiumPageView } from "./PremiumPageView";

const meta: Meta<typeof PremiumPageView> = {
	title: "pages/DeploymentSettingsPage/PremiumPageView",
	component: PremiumPageView,
};

export default meta;

type Story = StoryObj<typeof PremiumPageView>;

export const Enterprise: Story = {
	args: {
		isEnterprise: true,
	},
};

export const OSS: Story = {
	args: {
		isEnterprise: false,
	},
};
