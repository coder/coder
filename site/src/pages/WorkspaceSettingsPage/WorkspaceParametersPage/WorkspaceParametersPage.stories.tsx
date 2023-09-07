import { ComponentMeta, Story } from "@storybook/react";
import {
  WorkspaceParametersPageView,
  WorkspaceParametersPageViewProps,
} from "./WorkspaceParametersPage";
import { action } from "@storybook/addon-actions";
import {
  MockWorkspaceBuildParameter1,
  MockWorkspaceBuildParameter2,
  MockTemplateVersionParameter1,
  MockTemplateVersionParameter2,
  MockTemplateVersionParameter3,
  MockWorkspaceBuildParameter3,
} from "testHelpers/entities";

export default {
  title: "pages/WorkspaceParametersPageView",
  component: WorkspaceParametersPageView,
  args: {
    submitError: undefined,
    isSubmitting: false,
    onCancel: action("cancel"),
    data: {
      buildParameters: [
        MockWorkspaceBuildParameter1,
        MockWorkspaceBuildParameter2,
        MockWorkspaceBuildParameter3,
      ],
      templateVersionRichParameters: [
        MockTemplateVersionParameter1,
        MockTemplateVersionParameter2,
        {
          ...MockTemplateVersionParameter3,
          mutable: false,
        },
      ],
    },
  },
} as ComponentMeta<typeof WorkspaceParametersPageView>;

const Template: Story<WorkspaceParametersPageViewProps> = (args) => (
  <WorkspaceParametersPageView {...args} />
);

export const Example = Template.bind({});
Example.args = {};
