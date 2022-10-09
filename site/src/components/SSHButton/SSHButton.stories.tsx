import { Story } from "@storybook/react"
import {
  MockWorkspace,
  MockWorkspaceAgent,
} from "../../testHelpers/renderHelpers"
import { SSHButton, SSHButtonProps } from "./SSHButton"

export default {
  title: "components/SSHButton",
  component: SSHButton,
}

const Template: Story<SSHButtonProps> = (args) => <SSHButton {...args} />

export const Closed = Template.bind({})
Closed.args = {
  workspaceName: MockWorkspace.name,
  agentName: MockWorkspaceAgent.name,
}

export const Opened = Template.bind({})
Opened.args = {
  workspaceName: MockWorkspace.name,
  agentName: MockWorkspaceAgent.name,
  defaultIsOpen: true,
}
