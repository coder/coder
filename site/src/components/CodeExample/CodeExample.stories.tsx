import type { Meta, StoryObj } from "@storybook/react";
import { CodeExample } from "./CodeExample";

const meta: Meta<typeof CodeExample> = {
  title: "components/CodeExample",
  component: CodeExample,
  args: {
    code: `echo "hello, friend!"`,
  },
};

export default meta;
type Story = StoryObj<typeof CodeExample>;

export const Example: Story = {};

export const Secret: Story = {
  args: {
    secret: true,
  },
};

export const LongCode: Story = {
  args: {
    code: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAICnKzATuWwmmt5+CKTPuRGN0R1PBemA+6/SStpLiyX+L",
  },
};
