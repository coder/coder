import type { Meta, StoryObj } from "@storybook/react";
import { CodeExample } from "./CodeExample";

const sampleCode = `echo "Hello, world"`;

const meta: Meta<typeof CodeExample> = {
  title: "components/CodeExample",
  component: CodeExample,
  argTypes: {
    code: { control: "string", defaultValue: sampleCode },
  },
};

export default meta;
type Story = StoryObj<typeof CodeExample>;

export const Example: Story = {
  args: {
    code: sampleCode,
  },
};

export const LongCode: Story = {
  args: {
    code: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAICnKzATuWwmmt5+CKTPuRGN0R1PBemA+6/SStpLiyX+L",
  },
};
