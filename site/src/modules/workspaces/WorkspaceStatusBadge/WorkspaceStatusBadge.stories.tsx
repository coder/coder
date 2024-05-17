import type { Meta, StoryObj } from "@storybook/react";
import {
  MockBuildInfo,
  MockCanceledWorkspace,
  MockCancelingWorkspace,
  MockDeletedWorkspace,
  MockDeletingWorkspace,
  MockFailedWorkspace,
  MockPendingWorkspace,
  MockStartingWorkspace,
  MockStoppedWorkspace,
  MockStoppingWorkspace,
  MockWorkspace,
} from "testHelpers/entities";
import { withDashboardProvider } from "testHelpers/storybook";
import { WorkspaceStatusBadge } from "./WorkspaceStatusBadge";

const meta: Meta<typeof WorkspaceStatusBadge> = {
  title: "modules/workspaces/WorkspaceStatusBadge",
  component: WorkspaceStatusBadge,
  parameters: {
    queries: [
      {
        key: ["buildInfo"],
        data: MockBuildInfo,
      },
    ],
  },
  decorators: [withDashboardProvider],
};

export default meta;
type Story = StoryObj<typeof WorkspaceStatusBadge>;

export const Running: Story = {
  args: {
    workspace: MockWorkspace,
  },
};

export const Starting: Story = {
  args: {
    workspace: MockStartingWorkspace,
  },
};

export const Stopped: Story = {
  args: {
    workspace: MockStoppedWorkspace,
  },
};

export const Stopping: Story = {
  args: {
    workspace: MockStoppingWorkspace,
  },
};

export const Deleting: Story = {
  args: {
    workspace: MockDeletingWorkspace,
  },
};

export const Deleted: Story = {
  args: {
    workspace: MockDeletedWorkspace,
  },
};

export const Canceling: Story = {
  args: {
    workspace: MockCancelingWorkspace,
  },
};

export const Canceled: Story = {
  args: {
    workspace: MockCanceledWorkspace,
  },
};

export const Failed: Story = {
  args: {
    workspace: MockFailedWorkspace,
  },
};

export const Pending: Story = {
  args: {
    workspace: MockPendingWorkspace,
  },
};
