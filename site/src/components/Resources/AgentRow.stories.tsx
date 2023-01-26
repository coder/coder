import { Story } from "@storybook/react"
import {
  MockWorkspace,
  MockWorkspaceAgent,
  MockWorkspaceAgentConnecting,
  MockWorkspaceAgentStartError,
  MockWorkspaceAgentStarting,
  MockWorkspaceAgentStartTimeout,
  MockWorkspaceAgentTimeout,
  MockWorkspaceApp,
} from "testHelpers/entities"
import { AgentRow, AgentRowProps } from "./AgentRow"

export default {
  title: "components/AgentRow",
  component: AgentRow,
}

const Template: Story<AgentRowProps> = (args) => <AgentRow {...args} />

export const Example = Template.bind({})
Example.args = {
  agent: MockWorkspaceAgent,
  workspace: MockWorkspace,
  applicationsHost: "",
  showApps: true,
}

export const HideSSHButton = Template.bind({})
HideSSHButton.args = {
  agent: MockWorkspaceAgent,
  workspace: MockWorkspace,
  applicationsHost: "",
  showApps: true,
  hideSSHButton: true,
}

export const HideVSCodeDesktopButton = Template.bind({})
HideVSCodeDesktopButton.args = {
  agent: MockWorkspaceAgent,
  workspace: MockWorkspace,
  applicationsHost: "",
  showApps: true,
  hideVSCodeDesktopButton: true,
}

export const NotShowingApps = Template.bind({})
NotShowingApps.args = {
  agent: MockWorkspaceAgent,
  workspace: MockWorkspace,
  applicationsHost: "",
  showApps: false,
}

export const BunchOfApps = Template.bind({})
BunchOfApps.args = {
  ...Example.args,
  agent: {
    ...MockWorkspaceAgent,
    apps: [
      MockWorkspaceApp,
      MockWorkspaceApp,
      MockWorkspaceApp,
      MockWorkspaceApp,
      MockWorkspaceApp,
      MockWorkspaceApp,
      MockWorkspaceApp,
      MockWorkspaceApp,
    ],
  },
  workspace: MockWorkspace,
  applicationsHost: "",
  showApps: true,
}

export const Connecting = Template.bind({})
Connecting.args = {
  agent: MockWorkspaceAgentConnecting,
  workspace: MockWorkspace,
  applicationsHost: "",
  showApps: true,
}

export const Timeout = Template.bind({})
Timeout.args = {
  agent: MockWorkspaceAgentTimeout,
  workspace: MockWorkspace,
  applicationsHost: "",
  showApps: true,
}

export const Starting = Template.bind({})
Starting.args = {
  agent: MockWorkspaceAgentStarting,
  workspace: MockWorkspace,
  applicationsHost: "",
  showApps: true,
}

export const StartTimeout = Template.bind({})
StartTimeout.args = {
  agent: MockWorkspaceAgentStartTimeout,
  workspace: MockWorkspace,
  applicationsHost: "",
  showApps: true,
}

export const StartError = Template.bind({})
StartError.args = {
  agent: MockWorkspaceAgentStartError,
  workspace: MockWorkspace,
  applicationsHost: "",
  showApps: true,
}

export const ShowingPortForward = Template.bind({})
ShowingPortForward.args = {
  agent: MockWorkspaceAgent,
  workspace: MockWorkspace,
  applicationsHost: "https://coder.com",
  showApps: true,
}
