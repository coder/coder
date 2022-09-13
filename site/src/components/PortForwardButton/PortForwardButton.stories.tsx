import { Story } from "@storybook/react"
import { MockWorkspace, MockWorkspaceAgent } from "../../testHelpers/renderHelpers"
import { PortForwardButton, PortForwardButtonProps } from "./PortForwardButton"

export default {
  title: "components/PortForwardButton",
  component: PortForwardButton,
}

const Template: Story<PortForwardButtonProps> = (args) => <PortForwardButton {...args} />

export const Closed = Template.bind({})
Closed.args = {
  username: MockWorkspace.owner_name,
  workspaceName: MockWorkspace.name,
  agentName: MockWorkspaceAgent.name,
}

export const Opened = Template.bind({})
Opened.args = {
  username: MockWorkspace.owner_name,
  workspaceName: MockWorkspace.name,
  agentName: MockWorkspaceAgent.name,
  defaultIsOpen: true,
}
