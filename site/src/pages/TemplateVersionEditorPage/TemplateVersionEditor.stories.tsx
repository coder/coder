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
  title: "pages/TemplateVersionEditorPage",
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

export const Logs: Story = {
  args: {
    defaultTab: "logs",
    buildLogs: MockWorkspaceBuildLogs,
    templateVersion: {
      ...MockTemplateVersion,
      job: MockRunningProvisionerJob,
    },
  },
};

export const Resources: Story = {
  args: {
    defaultTab: "resources",
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

export const WithError = {
  args: {
    defaultTab: "logs",
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
