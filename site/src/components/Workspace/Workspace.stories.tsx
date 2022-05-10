import { action } from "@storybook/addon-actions"
import { Story } from "@storybook/react"
import React from "react"
import { MockOrganization, MockTemplate, MockWorkspace } from "../../testHelpers"
import { Workspace, WorkspaceProps } from "./Workspace"

export default {
  title: "components/Workspace",
  component: Workspace,
  argTypes: {},
}

const Template: Story<WorkspaceProps> = (args) => <Workspace {...args} />

export const Started = Template.bind({})
Started.args = {
  organization: MockOrganization,
  template: MockTemplate,
  workspace: MockWorkspace,
  handleStart: action("start"),
  handleStop: action("stop"),
  handleRetry: action("retry"),
  workspaceStatus: "started"
}

export const Starting = Template.bind({})
Starting.args = { ...Started.args, workspaceStatus: "starting" }

export const Stopped = Template.bind({})
Stopped.args = { ...Started.args, workspaceStatus: "stopped" }

export const Stopping = Template.bind({})
Stopping.args = { ...Started.args, workspaceStatus: "stopping" }

export const Error = Template.bind({})
Error.args = { ...Started.args, workspaceStatus: "error" }

export const NoBreadcrumb = Template.bind({})
NoBreadcrumb.args = { ...Started.args, template: undefined }
