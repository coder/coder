import { Meta, StoryObj } from "@storybook/react";
import { WorkspaceParametersPageView } from "./WorkspaceParametersPage";
import {
  MockWorkspaceBuildParameter1,
  MockWorkspaceBuildParameter2,
  MockTemplateVersionParameter1,
  MockTemplateVersionParameter2,
  MockTemplateVersionParameter3,
  MockWorkspaceBuildParameter3,
} from "testHelpers/entities";

const meta: Meta<typeof WorkspaceParametersPageView> = {
  title: "pages/WorkspaceSettingsPage/WorkspaceParametersPageView",
  component: WorkspaceParametersPageView,
  args: {
    submitError: undefined,
    isSubmitting: false,

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
};

export default meta;
type Story = StoryObj<typeof WorkspaceParametersPageView>;

const Example: Story = {};

export const Empty: Story = {
  args: {
    data: {
      buildParameters: [],
      templateVersionRichParameters: [],
    },
  },
};

export { Example as WorkspaceParametersPage };
