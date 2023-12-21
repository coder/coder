import { Callout } from "./Callout";
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof Callout> = {
  title: "components/Callout",
  component: Callout,
};

export default meta;
type Story = StoryObj<typeof Callout>;

export const Danger: Story = {
  args: {
    children: "Danger",
    type: "danger",
  },
};

export const Error: Story = {
  args: {
    children: "Error",
    type: "error",
  },
};

export const Warning: Story = {
  args: {
    children: "Warning",
    type: "warning",
  },
};

export const Notice: Story = {
  args: {
    children: "Notice",
    type: "notice",
  },
};

export const Info: Story = {
  args: {
    children: "Information",
    type: "info",
  },
};

export const Success: Story = {
  args: {
    children: "Success",
    type: "success",
  },
};

export const Active: Story = {
  args: {
    children: "Active",
    type: "active",
  },
};

export const Default: Story = {
  args: {
    children: "Neutral/default",
  },
};

export const DefaultLight: Story = {
  args: {
    children: "Neutral/default",
  },
};
