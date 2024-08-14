import type { Meta, StoryObj } from "@storybook/react";
import { BuildIcon } from "./BuildIcon";

const meta: Meta<typeof BuildIcon> = {
  title: "components/BuildIcon",
  component: BuildIcon,
};

export default meta;
type Story = StoryObj<typeof BuildIcon>;

export const Start: Story = {
  args: {
    transition: "start",
  },
};

export const Stop: Story = {
  args: {
    transition: "stop",
  },
};

export const Delete: Story = {
  args: {
    transition: "delete",
  },
};
