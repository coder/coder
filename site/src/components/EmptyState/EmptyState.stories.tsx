import { EmptyState } from "./EmptyState";
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof EmptyState> = {
  title: "components/EmptyState",
  component: EmptyState,
  args: {
    message: "Hello world",
  },
};

export default meta;
type Story = StoryObj<typeof EmptyState>;

export const Example: Story = {};

export const WithDescription: Story = {
  args: {
    description: "Friendly greeting",
  },
};

export const WithCTA: Story = {
  args: {
    cta: <button title="Click me" />,
  },
};
