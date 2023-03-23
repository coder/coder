import { Story } from "@storybook/react"
import {
  MockWorkspace,
  MockWorkspaceAgent,
  MockWorkspaceAgentConnecting,
  MockWorkspaceAgentOff,
  MockWorkspaceAgentOutdated,
  MockWorkspaceAgentReady,
  MockWorkspaceAgentShutdownError,
  MockWorkspaceAgentShutdownTimeout,
  MockWorkspaceAgentShuttingDown,
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

  storybookStartupLogs: [
    "Cloning Git repository...",
    "Starting Docker Daemon...",
    "Adding some 🧙magic🧙...",
    "Starting VS Code...",
  ].map((line, index) => ({
    id: index,
    level: "info",
    output: line,
    time: "",
  })),
}

export const Started = Template.bind({})
Started.args = {
  agent: {
    ...MockWorkspaceAgentReady,
    startup_logs_length: 1,
  },
  workspace: MockWorkspace,
  applicationsHost: "",
  showApps: true,

  storybookStartupLogs: [
    "Cloning Git repository...",
    "Starting Docker Daemon...",
    "Adding some 🧙magic🧙...",
    "Starting VS Code...",
  ].map((line, index) => ({
    id: index,
    level: "info",
    output: line,
    time: "",
  })),
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

export const ShuttingDown = Template.bind({})
ShuttingDown.args = {
  agent: MockWorkspaceAgentShuttingDown,
  workspace: MockWorkspace,
  applicationsHost: "",
  showApps: true,
}

export const ShutdownTimeout = Template.bind({})
ShutdownTimeout.args = {
  agent: MockWorkspaceAgentShutdownTimeout,
  workspace: MockWorkspace,
  applicationsHost: "",
  showApps: true,
}

export const ShutdownError = Template.bind({})
ShutdownError.args = {
  agent: MockWorkspaceAgentShutdownError,
  workspace: MockWorkspace,
  applicationsHost: "",
  showApps: true,
}

export const Off = Template.bind({})
Off.args = {
  agent: MockWorkspaceAgentOff,
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

export const Outdated = Template.bind({})
Outdated.args = {
  agent: MockWorkspaceAgentOutdated,
  workspace: MockWorkspace,
  applicationsHost: "",
  showApps: true,
  serverVersion: "v99.999.9999+c1cdf14",
}
