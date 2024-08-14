import type { Meta, StoryObj } from "@storybook/react";
import { Latency } from "./Latency";

const meta: Meta<typeof Latency> = {
  title: "components/Latency",
  component: Latency,
};

export default meta;
type Story = StoryObj<typeof Latency>;

export const Low: Story = {
  args: {
    latency: 10,
  },
};

export const Medium: Story = {
  args: {
    latency: 150,
  },
};

export const High: Story = {
  args: {
    latency: 300,
  },
};

export const Loading: Story = {
  args: {
    isLoading: true,
  },
};
