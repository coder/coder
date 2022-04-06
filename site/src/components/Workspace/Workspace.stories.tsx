import { Story } from "@storybook/react"
import React from "react"
import { MockOrganization, MockTemplate, MockWorkspace } from "../../test_helpers"
import { Workspace, WorkspaceProps } from "./Workspace"

export default {
  title: "Workspaces/Workspace",
  component: Workspace,
  argTypes: {},
}

const Template: Story<WorkspaceProps> = (args) => <Workspace {...args} />

export const Example = Template.bind({})
Example.args = {
  organization: MockOrganization,
  template: MockTemplate,
  workspace: MockWorkspace,
}
