import { type ComponentProps } from "react";
import { Meta, StoryObj } from "@storybook/react";
import { WorkspaceDeleteDialog } from "./WorkspaceDeleteDialog";
import { MockWorkspace } from "testHelpers/entities";

const meta: Meta<typeof WorkspaceDeleteDialog> = {
  title: "pages/WorkspacePage/WorkspaceDeleteDialog",
  component: WorkspaceDeleteDialog,
};

export default meta;
type Story = StoryObj<typeof WorkspaceDeleteDialog>;

const args: ComponentProps<typeof WorkspaceDeleteDialog> = {
  workspace: MockWorkspace,
  canUpdateTemplate: false,
  isOpen: true,
  onCancel: () => {},
  onConfirm: () => {},
  workspaceBuildDateStr: "2 days ago",
};

export const NotTemplateAdmin: Story = {
  args,
};

export const TemplateAdmin: Story = {
  args: {
    ...args,
    canUpdateTemplate: true,
  },
};
