import { InfoTooltip } from "./InfoTooltip";
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof InfoTooltip> = {
  title: "components/InfoTooltip",
  component: InfoTooltip,
  args: {
    type: "info",
    title: "Hello, friend!",
    message: "Today is a lovely day :^)",
  },
};

export default meta;
type Story = StoryObj<typeof InfoTooltip>;

export const Example: Story = {};

export const Notice: Story = {
  args: {
    type: "notice",
    message: "Unfortunately, there's a radio connected to my brain",
  },
};

export const Warning: Story = {
  args: {
    type: "warning",
    message: "Unfortunately, there's a radio connected to my brain",
  },
};
