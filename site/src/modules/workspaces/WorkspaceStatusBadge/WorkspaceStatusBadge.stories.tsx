import {
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
  MockBuildInfo,
  MockEntitlementsWithScheduling,
  MockExperiments,
  MockAppearanceConfig,
} from "testHelpers/entities";
import { WorkspaceStatusBadge } from "./WorkspaceStatusBadge";
import { DashboardContext } from "modules/dashboard/DashboardProvider";
import type { Meta, StoryObj } from "@storybook/react";

const MockedAppearance = {
  config: MockAppearanceConfig,
  isPreview: false,
  setPreview: () => {},
};

const meta: Meta<typeof WorkspaceStatusBadge> = {
  title: "modules/workspaces/WorkspaceStatusBadge",
  component: WorkspaceStatusBadge,
  decorators: [
    (Story) => (
      <DashboardContext.Provider
        value={{
          buildInfo: MockBuildInfo,
          entitlements: MockEntitlementsWithScheduling,
          experiments: MockExperiments,
          appearance: MockedAppearance,
        }}
      >
        <Story />
      </DashboardContext.Provider>
    ),
  ],
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
