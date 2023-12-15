import { Meta, StoryObj } from "@storybook/react";
import { MockWorkspace } from "testHelpers/entities";
import { WorkspaceSettingsPageView } from "./WorkspaceSettingsPageView";

const meta: Meta<typeof WorkspaceSettingsPageView> = {
  title: "pages/WorkspaceSettingsPage/WorkspaceSettingsPageView",
  component: WorkspaceSettingsPageView,
  args: {
    error: undefined,
    workspace: MockWorkspace,
  },
};

export default meta;
type Story = StoryObj<typeof WorkspaceSettingsPageView>;

export const Example: Story = {};

export const AutoUpdates: Story = {
  args: {
    templatePoliciesEnabled: true,
  },
};

export const RenamesDisabled: Story = {
  args: {
    workspace: { ...MockWorkspace, allow_renames: false },
  },
};
