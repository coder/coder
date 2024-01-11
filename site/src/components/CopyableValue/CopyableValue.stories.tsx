import type { Meta, StoryObj } from "@storybook/react";
import { CopyableValue } from "./CopyableValue";

const meta: Meta<typeof CopyableValue> = {
  title: "components/CopyableValue",
  component: CopyableValue,
  args: {
    children: <button>Get secret</button>,
    value: "cool secret",
  },
};

export default meta;
type Story = StoryObj<typeof CopyableValue>;

const Example: Story = {};

export { Example as CopyableValue };
