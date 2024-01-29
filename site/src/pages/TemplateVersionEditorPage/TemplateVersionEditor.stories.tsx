import { action } from "@storybook/addon-actions";
import type { Meta, StoryObj } from "@storybook/react";
import { chromatic } from "testHelpers/chromatic";
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

const meta: Meta<typeof TemplateVersionEditor> = {
  title: "pages/TemplateVersionEditor",
  parameters: {
    chromatic,
    layout: "fullscreen",
  },
  component: TemplateVersionEditor,
  args: {
    template: MockTemplate,
    templateVersion: MockTemplateVersion,
    defaultFileTree: MockTemplateVersionFileTree,
    onPreview: action("onPreview"),
    onPublish: action("onPublish"),
    onConfirmPublish: action("onConfirmPublish"),
    onCancelPublish: action("onCancelPublish"),
    onCreateWorkspace: action("onCreateWorkspace"),
    onSubmitMissingVariableValues: action("onSubmitMissingVariableValues"),
    onCancelSubmitMissingVariableValues: action(
      "onCancelSubmitMissingVariableValues",
    ),
    provisionerTags: { wibble: "wobble", wiggle: "woggle" },
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
