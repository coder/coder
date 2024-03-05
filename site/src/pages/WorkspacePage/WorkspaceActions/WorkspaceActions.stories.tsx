import type { Meta, StoryObj } from "@storybook/react";
import * as Mocks from "testHelpers/entities";
import { WorkspaceActions } from "./WorkspaceActions";

const meta: Meta<typeof WorkspaceActions> = {
  title: "pages/WorkspacePage/WorkspaceActions",
  component: WorkspaceActions,
  args: {
    isUpdating: false,
  },
};

export default meta;
type Story = StoryObj<typeof WorkspaceActions>;

export const Starting: Story = {
  args: {
    workspace: Mocks.MockStartingWorkspace,
  },
};

export const Running: Story = {
  args: {
    workspace: Mocks.MockWorkspace,
  },
};

export const Stopping: Story = {
  args: {
    workspace: Mocks.MockStoppingWorkspace,
  },
};

export const Stopped: Story = {
  args: {
    workspace: Mocks.MockStoppedWorkspace,
  },
};

export const Canceling: Story = {
  args: {
    workspace: Mocks.MockCancelingWorkspace,
  },
};

export const Canceled: Story = {
  args: {
    workspace: Mocks.MockCanceledWorkspace,
  },
};

export const Deleting: Story = {
  args: {
    workspace: Mocks.MockDeletingWorkspace,
  },
};

export const Deleted: Story = {
  args: {
    workspace: Mocks.MockDeletedWorkspace,
  },
};

export const Outdated: Story = {
  args: {
    workspace: Mocks.MockOutdatedWorkspace,
  },
};

export const Failed: Story = {
  args: {
    workspace: Mocks.MockFailedWorkspace,
  },
};

export const FailedWithDebug: Story = {
  args: {
    workspace: Mocks.MockFailedWorkspace,
    canDebug: true,
  },
};

export const Updating: Story = {
  args: {
    isUpdating: true,
    workspace: Mocks.MockOutdatedWorkspace,
  },
};

export const RequireActiveVersionStarted: Story = {
  args: {
    workspace: Mocks.MockOutdatedRunningWorkspaceRequireActiveVersion,
    canChangeVersions: false,
  },
};

export const RequireActiveVersionStopped: Story = {
  args: {
    workspace: Mocks.MockOutdatedStoppedWorkspaceRequireActiveVersion,
    canChangeVersions: false,
  },
};

export const AlwaysUpdateStarted: Story = {
  args: {
    workspace: Mocks.MockOutdatedRunningWorkspaceAlwaysUpdate,
    canChangeVersions: true,
  },
};

export const AlwaysUpdateStopped: Story = {
  args: {
    workspace: Mocks.MockOutdatedStoppedWorkspaceAlwaysUpdate,
    canChangeVersions: true,
  },
};

export const CancelShownForOwner: Story = {
  args: {
    workspace: {
      ...Mocks.MockStartingWorkspace,
      template_allow_user_cancel_workspace_jobs: false,
    },
    isOwner: true,
  },
};
export const CancelShownForUser: Story = {
  args: {
    workspace: Mocks.MockStartingWorkspace,
    isOwner: false,
  },
};

export const CancelHiddenForUser: Story = {
  args: {
    workspace: {
      ...Mocks.MockStartingWorkspace,
      template_allow_user_cancel_workspace_jobs: false,
    },
    isOwner: false,
  },
};
