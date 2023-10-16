import { Meta, StoryObj } from "@storybook/react";
import { WorkspaceBuildLogs } from "./WorkspaceBuildLogs";
import { MockWorkspaceBuildLogs } from "testHelpers/entities";

const meta: Meta<typeof WorkspaceBuildLogs> = {
  title: "components/WorkspaceBuildLogs",
  component: WorkspaceBuildLogs,
};

export default meta;

type Story = StoryObj<typeof WorkspaceBuildLogs>;

export const InProgress: Story = {
  args: {
    logs: MockWorkspaceBuildLogs.slice(0, 20),
  },
};

export const Completed: Story = {
  args: {
    logs: MockWorkspaceBuildLogs,
  },
};
