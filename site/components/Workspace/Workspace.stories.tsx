import { Story } from "@storybook/react"
import React from "react"
import { Workspace, WorkspaceProps } from "./Workspace"
import { MockWorkspace } from "../../test_helpers"

export default {
  title: "Workspace",
  component: Workspace,
  argTypes: {},
}

const Template: Story<WorkspaceProps> = (args) => <Workspace {...args} />

export const Example = Template.bind({})
Example.args = {
  workspace: MockWorkspace,
}
