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
  MockAppearance,
} from "testHelpers/entities";
import { WorkspaceStatusBadge } from "./WorkspaceStatusBadge";
import { DashboardProviderContext } from "components/Dashboard/DashboardProvider";
import type { Meta, StoryObj } from "@storybook/react";

const MockedAppearance = {
  config: MockAppearance,
  preview: false,
  setPreview: () => null,
  save: () => null,
};

const meta: Meta<typeof WorkspaceStatusBadge> = {
  title: "components/WorkspaceStatusBadge",
  component: WorkspaceStatusBadge,
  decorators: [
    (Story) => (
      <DashboardProviderContext.Provider
        value={{
          buildInfo: MockBuildInfo,
          entitlements: MockEntitlementsWithScheduling,
          experiments: MockExperiments,
          appearance: MockedAppearance,
        }}
      >
        <Story />
      </DashboardProviderContext.Provider>
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
