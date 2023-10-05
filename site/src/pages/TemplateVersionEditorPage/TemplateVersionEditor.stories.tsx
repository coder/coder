import {
  MockFailedProvisionerJob,
  MockRunningProvisionerJob,
  MockTemplate,
  MockTemplateVersion,
  MockTemplateVersionFileTree,
  MockWorkspaceBuildLogs,
  MockWorkspaceContainerResource,
  MockWorkspaceExtendedBuildLogs,
  MockWorkspaceImageResource,
  MockWorkspaceResource,
  MockWorkspaceResourceMultipleAgents,
  MockWorkspaceResourceSensitive,
  MockWorkspaceVolumeResource,
} from "testHelpers/entities";
import { TemplateVersionEditor } from "./TemplateVersionEditor";
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof TemplateVersionEditor> = {
  title: "pages/TemplateVersionEditor",
  component: TemplateVersionEditor,
  args: {
    template: MockTemplate,
    templateVersion: MockTemplateVersion,
    defaultFileTree: MockTemplateVersionFileTree,
  },
  parameters: {
    layout: "fullscreen",
  },
};

export default meta;
type Story = StoryObj<typeof TemplateVersionEditor>;

export const Example: Story = {};

export const Logs = {
  args: {
    isBuildingNewVersion: true,
    buildLogs: MockWorkspaceBuildLogs,
    templateVersion: {
      ...MockTemplateVersion,
      job: MockRunningProvisionerJob,
    },
  },
};

export const Resources: Story = {
  args: {
    isBuildingNewVersion: true,
    buildLogs: MockWorkspaceBuildLogs,
    resources: [
      MockWorkspaceResource,
      MockWorkspaceResourceSensitive,
      MockWorkspaceResourceMultipleAgents,
      MockWorkspaceVolumeResource,
      MockWorkspaceImageResource,
      MockWorkspaceContainerResource,
    ],
  },
};

export const ManyLogs = {
  args: {
    isBuildingNewVersion: true,
    templateVersion: {
      ...MockTemplateVersion,
      job: {
        ...MockFailedProvisionerJob,
        error:
          "template import provision for start: terraform plan: exit status 1",
      },
    },
    buildLogs: MockWorkspaceExtendedBuildLogs,
  },
};

export const Published = {
  args: {
    publishedVersion: MockTemplateVersion,
  },
};
