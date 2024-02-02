import { PortForwardButton } from "./PortForwardButton";
import type { Meta, StoryObj } from "@storybook/react";
import {
  MockListeningPortsResponse,
  MockWorkspaceAgent,
} from "testHelpers/entities";

const meta: Meta<typeof PortForwardButton> = {
  title: "modules/resources/PortForwardButton",
  component: PortForwardButton,
  args: {
    agent: MockWorkspaceAgent,
  },
};

export default meta;
type Story = StoryObj<typeof PortForwardButton>;

export const Example: Story = {
  args: {
    storybook: {
      portsQueryData: MockListeningPortsResponse,
    },
  },
};

export const Loading: Story = {
  args: {
    storybook: {},
  },
};
