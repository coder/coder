import { Meta, StoryObj } from "@storybook/react";
import { MockWorkspace } from "testHelpers/entities";
import { WorkspaceSettingsPageView } from "./WorkspaceSettingsPageView";

const meta: Meta<typeof WorkspaceSettingsPageView> = {
  title: "pages/WorkspaceSettingsPage/WorkspaceSettingsPageView",
  component: WorkspaceSettingsPageView,
  args: {
    error: undefined,
    isSubmitting: false,
    workspace: MockWorkspace,
  },
};

export default meta;
type Story = StoryObj<typeof WorkspaceSettingsPageView>;

const Example: Story = {};

export { Example as WorkspaceSettingsPageView };
