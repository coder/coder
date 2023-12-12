import { action } from "@storybook/addon-actions";
import type { Meta, StoryObj } from "@storybook/react";
import { MockWorkspace, MockUser2 } from "testHelpers/entities";
import { BatchDeleteConfirmation } from "./BatchActions";

const meta: Meta<typeof BatchDeleteConfirmation> = {
  title: "pages/WorkspacesPage/BatchDelete",
  component: BatchDeleteConfirmation,
  args: {
    onClose: action("onClose"),
    onConfirm: action("onConfirm"),
    open: true,
    checkedWorkspaces: [
      MockWorkspace,
      {
        ...MockWorkspace,
        name: "Test-Workspace-2",
        last_used_at: "2023-08-16T15:29:10.302441433Z",
        owner_id: MockUser2.id,
        owner_name: MockUser2.username,
      },
      {
        ...MockWorkspace,
        name: "Test-Workspace-3",
        last_used_at: "2023-11-16T15:29:10.302441433Z",
        owner_id: MockUser2.id,
        owner_name: MockUser2.username,
      },
    ],
  },
};

export default meta;
type Story = StoryObj<typeof BatchDeleteConfirmation>;

const Example: Story = {};

export { Example as BatchDelete };
