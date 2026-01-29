import type { Meta, StoryObj } from "@storybook/react-vite";
import { PaywallPremium } from "./PaywallPremium";

const meta: Meta<typeof PaywallPremium> = {
	title: "components/Paywall/Premium",
	component: PaywallPremium,
};

export default meta;
type Story = StoryObj<typeof PaywallPremium>;

export const Default: Story = {
	args: {
		message: "Black Lotus",
		description:
			"Adds 3 mana of any single color of your choice to your mana pool, then is discarded. Tapping this artifact can be played as an interrupt.",
		documentationLink: "https://coder.com/docs",
	},
};
