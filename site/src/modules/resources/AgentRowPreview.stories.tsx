import type { Meta, StoryObj } from "@storybook/react";
import { MockWorkspaceAgent, MockWorkspaceApp } from "testHelpers/entities";
import { AgentRowPreview } from "./AgentRowPreview";

const meta: Meta<typeof AgentRowPreview> = {
  title: "modules/resources/AgentRowPreview",
  component: AgentRowPreview,
  args: {
    agent: MockWorkspaceAgent,
  },
};

export default meta;
type Story = StoryObj<typeof AgentRowPreview>;

export const Example: Story = {};

export const BunchOfApps: Story = {
  args: {
    agent: {
      ...MockWorkspaceAgent,
      apps: [
        MockWorkspaceApp,
        MockWorkspaceApp,
        MockWorkspaceApp,
        MockWorkspaceApp,
        MockWorkspaceApp,
        MockWorkspaceApp,
        MockWorkspaceApp,
        MockWorkspaceApp,
      ],
    },
  },
};

export const NoApps: Story = {
  args: {
    agent: {
      ...MockWorkspaceAgent,
      apps: [],
    },
  },
};
