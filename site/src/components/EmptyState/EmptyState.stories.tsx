import Button from "@mui/material/Button";
import type { Meta, StoryObj } from "@storybook/react";
import { EmptyState } from "./EmptyState";

const meta: Meta<typeof EmptyState> = {
  title: "components/EmptyState",
  component: EmptyState,
  args: {
    message: "Create your first workspace",
  },
};

export default meta;
type Story = StoryObj<typeof EmptyState>;

const Example: Story = {
  args: {
    description: "It is easy, just click the button below",
    cta: <Button>Create workspace</Button>,
  },
};

export { Example as EmptyState };
