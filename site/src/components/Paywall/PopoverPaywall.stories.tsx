import type { Meta, StoryObj } from "@storybook/react";
import { PopoverPaywall } from "./PopoverPaywall";

const meta: Meta<typeof PopoverPaywall> = {
  title: "components/Paywall/PopoverPaywall",
  component: PopoverPaywall,
};

export default meta;
type Story = StoryObj<typeof PopoverPaywall>;

const Example: Story = {
  args: {
    message: "Black Lotus",
    description:
      "Adds 3 mana of any single color of your choice to your mana pool, then is discarded. Tapping this artifact can be played as an interrupt.",
  },
};

export { Example as PopoverPaywall };
