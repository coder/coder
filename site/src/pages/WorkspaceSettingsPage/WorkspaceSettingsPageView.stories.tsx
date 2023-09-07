import { ComponentMeta, Story } from "@storybook/react";
import { MockWorkspace } from "testHelpers/entities";
import {
  WorkspaceSettingsPageView,
  WorkspaceSettingsPageViewProps,
} from "./WorkspaceSettingsPageView";
import { action } from "@storybook/addon-actions";

export default {
  title: "pages/WorkspaceSettingsPageView",
  component: WorkspaceSettingsPageView,
  args: {
    error: undefined,
    isSubmitting: false,
    workspace: MockWorkspace,
    onCancel: action("cancel"),
  },
} as ComponentMeta<typeof WorkspaceSettingsPageView>;

const Template: Story<WorkspaceSettingsPageViewProps> = (args) => (
  <WorkspaceSettingsPageView {...args} />
);

export const Example = Template.bind({});
Example.args = {};
