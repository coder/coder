import { PortForwardButton } from "./PortForwardButton";
import type { Meta, StoryObj } from "@storybook/react";
import {
  MockListeningPortsResponse,
  MockSharedPortsResponse,
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
      listeningPortsQueryData: MockListeningPortsResponse,
      sharedPortsQueryData: MockSharedPortsResponse,
    },
  },
};

export const Loading: Story = {
  args: {
    storybook: {},
  },
};
