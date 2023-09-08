import { Typography } from "./Typography";
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof Typography> = {
  title: "components/Typography",
  component: Typography,
  args: {
    children: "Colorless green ideas sleep furiously",
  },
};

export default meta;
type Story = StoryObj<typeof Typography>;

export const Short: Story = {
  args: {
    short: true,
  },
};

export const Tall: Story = {
  args: {
    short: false,
  },
};
