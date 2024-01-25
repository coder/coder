import type { Meta, StoryObj } from "@storybook/react";
import { CopyButton } from "./CopyButton";

const meta: Meta<typeof CopyButton> = {
  title: "components/CopyButton",
  component: CopyButton,
  args: {
    children: "Get secret",
    text: "cool secret",
  },
};

export default meta;
type Story = StoryObj<typeof CopyButton>;

const Example: Story = {};

export { Example as CopyButton };
