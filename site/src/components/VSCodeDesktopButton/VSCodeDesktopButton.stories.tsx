import { Story } from "@storybook/react"
import { MockWorkspace, MockWorkspaceAgent } from "testHelpers/renderHelpers"
import {
  VSCodeDesktopButton,
  VSCodeDesktopButtonProps,
} from "./VSCodeDesktopButton"

export default {
  title: "components/VSCodeDesktopButton",
  component: VSCodeDesktopButton,
}

const Template: Story<VSCodeDesktopButtonProps> = (args) => (
  <VSCodeDesktopButton {...args} />
)

export const Default = Template.bind({})
Default.args = {
  userName: MockWorkspace.owner_name,
  workspaceName: MockWorkspace.name,
  agentName: MockWorkspaceAgent.name,
}
