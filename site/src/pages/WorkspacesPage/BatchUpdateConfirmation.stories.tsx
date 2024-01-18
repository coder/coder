import { action } from "@storybook/addon-actions";
import type { Meta, StoryObj } from "@storybook/react";
import { useQueryClient } from "react-query";
import { chromatic } from "testHelpers/chromatic";
import {
  MockWorkspace,
  MockRunningOutdatedWorkspace,
  MockDormantOutdatedWorkspace,
  MockOutdatedWorkspace,
  MockTemplateVersion,
  MockUser2,
} from "testHelpers/entities";
import {
  BatchUpdateConfirmation,
  type Update,
} from "./BatchUpdateConfirmation";

const workspaces = [
  { ...MockRunningOutdatedWorkspace, id: "1" },
  { ...MockDormantOutdatedWorkspace, id: "2" },
  { ...MockOutdatedWorkspace, id: "3" },
  { ...MockOutdatedWorkspace, id: "4" },
  { ...MockWorkspace, id: "5" },
  {
    ...MockRunningOutdatedWorkspace,
    id: "6",
    owner_id: MockUser2.id,
    owner_name: MockUser2.username,
  },
];

const updates = new Map<string, Update>();
for (const it of workspaces) {
  const versionId = it.template_active_version_id;
  const version = updates.get(versionId);

  if (version) {
    version.affected_workspaces.push(it);
    continue;
  }

  updates.set(versionId, {
    ...MockTemplateVersion,
    template_display_name: it.template_display_name,
    affected_workspaces: [it],
  });
}

const meta: Meta<typeof BatchUpdateConfirmation> = {
  title: "pages/WorkspacesPage/BatchUpdateConfirmation",
  parameters: { chromatic },
  component: BatchUpdateConfirmation,
  decorators: [
    (Story) => {
      const queryClient = useQueryClient();
      for (const [id, it] of updates) {
        queryClient.setQueryData(["batchUpdate", id], it);
      }
      return <Story />;
    },
  ],
  args: {
    onClose: action("onClose"),
    onConfirm: action("onConfirm"),
    open: true,
    checkedWorkspaces: workspaces,
  },
};

export default meta;
type Story = StoryObj<typeof BatchUpdateConfirmation>;

const Example: Story = {};

export { Example as BatchUpdateConfirmation };
