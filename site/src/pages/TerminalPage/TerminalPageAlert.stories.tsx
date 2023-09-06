import type { Meta, StoryObj } from "@storybook/react";

import TerminalPageAlert from "./TerminalPageAlert";

const meta: Meta<typeof TerminalPageAlert> = {
  component: TerminalPageAlert,
  title: "components/TerminalPageAlert",
  argTypes: {
    alertType: {
      control: {
        type: "radio",
      },
      options: ["error", "starting", "success"],
    },
  },
};
type Story = StoryObj<typeof TerminalPageAlert>;

export const Error: Story = {
  args: {
    alertType: "error",
  },
};

export const Starting: Story = {
  args: {
    alertType: "starting",
  },
};

export const Success: Story = {
  args: {
    alertType: "success",
  },
};

export default meta;
