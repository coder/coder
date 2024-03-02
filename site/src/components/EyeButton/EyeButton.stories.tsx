import type { Meta, StoryObj } from "@storybook/react";
import { EyeButton } from "./EyeButton";

const meta: Meta<typeof EyeButton> = {
  title: "components/EyeButton",
  component: EyeButton,
  args: {
    children: "View/Hide secret",
  },
};

export default meta;
type Story = StoryObj<typeof EyeButton>;

const Example: Story = {};

export { Example as EyeButton };
