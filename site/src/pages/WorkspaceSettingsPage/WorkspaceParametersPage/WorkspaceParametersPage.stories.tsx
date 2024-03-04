import type { Meta, StoryObj } from "@storybook/react";
import {
  MockWorkspaceBuildParameter1,
  MockWorkspaceBuildParameter2,
  MockTemplateVersionParameter1,
  MockTemplateVersionParameter2,
  MockTemplateVersionParameter3,
  MockWorkspaceBuildParameter3,
  MockWorkspace,
  MockOutdatedStoppedWorkspaceRequireActiveVersion,
} from "testHelpers/entities";
import { WorkspaceParametersPageView } from "./WorkspaceParametersPage";

const meta: Meta<typeof WorkspaceParametersPageView> = {
  title: "pages/WorkspaceSettingsPage/WorkspaceParametersPageView",
  component: WorkspaceParametersPageView,
  args: {
    submitError: undefined,
    isSubmitting: false,
    workspace: MockWorkspace,
    canChangeVersions: true,

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

export const RequireActiveVersionNoChangeVersion: Story = {
  args: {
    workspace: MockOutdatedStoppedWorkspaceRequireActiveVersion,
    canChangeVersions: false,
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

export const RequireActiveVersionCanChangeVersion: Story = {
  args: {
    workspace: MockOutdatedStoppedWorkspaceRequireActiveVersion,
    canChangeVersions: true,
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

export { Example as WorkspaceParametersPage };
