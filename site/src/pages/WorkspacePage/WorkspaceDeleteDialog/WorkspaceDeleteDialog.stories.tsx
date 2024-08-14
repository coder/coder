import type { Meta, StoryObj } from "@storybook/react";
import { MockWorkspace, MockFailedWorkspace } from "testHelpers/entities";
import { WorkspaceDeleteDialog } from "./WorkspaceDeleteDialog";

const meta: Meta<typeof WorkspaceDeleteDialog> = {
  title: "pages/WorkspacePage/WorkspaceDeleteDialog",
  component: WorkspaceDeleteDialog,
  args: {
    workspace: MockWorkspace,
    canUpdateTemplate: false,
    isOpen: true,
    onCancel: () => {},
    onConfirm: () => {},
    workspaceBuildDateStr: "2 days ago",
  },
};

export default meta;
type Story = StoryObj<typeof WorkspaceDeleteDialog>;

export const Example: Story = {};

// Should look the same as `Example`
export const Unhealthy: Story = {
  args: {
    workspace: MockFailedWorkspace,
  },
};

// Should look the same as `Example`
export const AdminView: Story = {
  args: {
    canUpdateTemplate: true,
  },
};

// Should show the `--orphan` option
export const UnhealthyAdminView: Story = {
  args: {
    workspace: MockFailedWorkspace,
    canUpdateTemplate: true,
  },
};
