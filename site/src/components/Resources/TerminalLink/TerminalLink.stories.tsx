import { MockWorkspace } from "testHelpers/entities";
import { TerminalLink } from "./TerminalLink";
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof TerminalLink> = {
  title: "components/TerminalLink",
  component: TerminalLink,
};

export default meta;
type Story = StoryObj<typeof TerminalLink>;

export const Example: Story = {
  args: {
    workspaceName: MockWorkspace.name,
  },
};
