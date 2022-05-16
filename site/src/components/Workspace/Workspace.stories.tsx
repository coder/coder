import { action } from "@storybook/addon-actions"
import { Story } from "@storybook/react"
import React from "react"
import { MockOutdatedWorkspace, MockWorkspace } from "../../testHelpers/renderHelpers"
import { Workspace, WorkspaceProps } from "./Workspace"

export default {
  title: "components/Workspace",
  component: Workspace,
  argTypes: {},
}

const Template: Story<WorkspaceProps> = (args) => <Workspace {...args} />

export const Started = Template.bind({})
Started.args = {
  workspace: MockWorkspace,
  handleStart: action("start"),
  handleStop: action("stop"),
  handleRetry: action("retry"),
  workspaceStatus: "started",
}

export const Starting = Template.bind({})
Starting.args = { ...Started.args, workspaceStatus: "starting" }

export const Stopped = Template.bind({})
Stopped.args = { ...Started.args, workspaceStatus: "stopped" }

export const Stopping = Template.bind({})
Stopping.args = { ...Started.args, workspaceStatus: "stopping" }

export const Error = Template.bind({})
Error.args = { ...Started.args, workspaceStatus: "error" }

export const BuildLoading = Template.bind({})
BuildLoading.args = { ...Started.args, workspaceStatus: "loading" }

export const Deleting = Template.bind({})
Deleting.args = { ...Started.args, workspaceStatus: "deleting" }

export const Deleted = Template.bind({})
Deleted.args = { ...Started.args, workspaceStatus: "deleted" }

export const Canceling = Template.bind({})
Canceling.args = { ...Started.args, workspaceStatus: "canceling" }

export const NoBreadcrumb = Template.bind({})
NoBreadcrumb.args = { ...Started.args, template: undefined }

export const Outdated = Template.bind({})
Outdated.args = { ...Started.args, workspace: MockOutdatedWorkspace }
