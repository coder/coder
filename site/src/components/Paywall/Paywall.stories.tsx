import type { Meta, StoryObj } from "@storybook/react-vite";
import { Paywall } from "./Paywall";

const meta: Meta<typeof Paywall> = {
	title: "components/Paywall",
	component: Paywall,
};

export default meta;
type Story = StoryObj<typeof Paywall>;

export const Premium: Story = {
	args: {
		message: "Black Lotus",
		description:
			"Adds 3 mana of any single color of your choice to your mana pool, then is discarded. Tapping this artifact can be played as an interrupt.",
	},
};
