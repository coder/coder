import type { Meta, StoryObj } from "@storybook/react";
import { Welcome } from "./Welcome";

const meta: Meta<typeof Welcome> = {
  title: "components/Welcome",
  component: Welcome,
};

export default meta;
type Story = StoryObj<typeof Welcome>;

const Example: Story = {};

export { Example as Welcome };
