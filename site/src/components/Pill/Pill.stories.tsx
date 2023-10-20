import { Pill } from "./Pill";
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof Pill> = {
  title: "components/Pill",
  component: Pill,
};

export default meta;
type Story = StoryObj<typeof Pill>;

export const Primary: Story = {
  args: {
    text: "Primary",
    type: "primary",
  },
};

export const Secondary: Story = {
  args: {
    text: "Secondary",
    type: "secondary",
  },
};

export const Success: Story = {
  args: {
    text: "Success",
    type: "success",
  },
};

export const Info: Story = {
  args: {
    text: "Information",
    type: "info",
  },
};

export const Warning: Story = {
  args: {
    text: "Warning",
    type: "warning",
  },
};

export const Error: Story = {
  args: {
    text: "Error",
    type: "error",
  },
};

export const Default: Story = {
  args: {
    text: "Default",
  },
};

export const WarningLight: Story = {
  args: {
    text: "Warning",
    type: "warning",
    lightBorder: true,
  },
};
