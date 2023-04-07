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

const defaultAgentMetadata = [
  {
    result: {
      collected_at: "2021-05-05T00:00:00Z",
      error: "",
      value: "defvalue",
      age: 5,
    },
    description: {
      display_name: "DisPlay",
      key: "defkey",
      interval: 10,
      timeout: 10,
      script: "some command",
    },
  },
]

export const Example = Template.bind({})
Example.args = {
  agent: MockWorkspaceAgent,
  workspace: MockWorkspace,
  applicationsHost: "",
  showApps: true,
  storybookAgentMetadata: defaultAgentMetadata,
}

export const HideSSHButton = Template.bind({})
HideSSHButton.args = {
  ...Example.args,
  hideSSHButton: true,
}

export const HideVSCodeDesktopButton = Template.bind({})
HideVSCodeDesktopButton.args = {
  ...Example.args,
  hideVSCodeDesktopButton: true,
}

export const NotShowingApps = Template.bind({})
NotShowingApps.args = {
  ...Example.args,
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
  ...Example.args,
  agent: MockWorkspaceAgentConnecting,
  storybookAgentMetadata: [],
}

export const Timeout = Template.bind({})
Timeout.args = {
  ...Example.args,
  agent: MockWorkspaceAgentTimeout,
}

export const Starting = Template.bind({})
Starting.args = {
  ...Example.args,
  agent: MockWorkspaceAgentStarting,

  storybookStartupLogs: [
    "\x1b[91mCloning Git repository...",
    "\x1b[2;37;41mStarting Docker Daemon...",
    "\x1b[1;95mAdding some 🧙magic🧙...",
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
  ...Example.args,
  agent: {
    ...MockWorkspaceAgentReady,
    startup_logs_length: 1,
  },

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

export const StartedNoMetadata = Template.bind({})
StartedNoMetadata.args = {
  ...Started.args,
  storybookAgentMetadata: [],
}

export const StartTimeout = Template.bind({})
StartTimeout.args = {
  ...Example.args,
  agent: MockWorkspaceAgentStartTimeout,
}

export const StartError = Template.bind({})
StartError.args = {
  ...Example.args,
  agent: MockWorkspaceAgentStartError,
}

export const ShuttingDown = Template.bind({})
ShuttingDown.args = {
  ...Example.args,
  agent: MockWorkspaceAgentShuttingDown,
}

export const ShutdownTimeout = Template.bind({})
ShutdownTimeout.args = {
  ...Example.args,
  agent: MockWorkspaceAgentShutdownTimeout,
}

export const ShutdownError = Template.bind({})
ShutdownError.args = {
  ...Example.args,
  agent: MockWorkspaceAgentShutdownError,
}

export const Off = Template.bind({})
Off.args = {
  ...Example.args,
  agent: MockWorkspaceAgentOff,
}

export const ShowingPortForward = Template.bind({})
ShowingPortForward.args = {
  ...Example.args,
  applicationsHost: "https://coder.com",
}

export const Outdated = Template.bind({})
Outdated.args = {
  ...Example.args,
  agent: MockWorkspaceAgentOutdated,
  workspace: MockWorkspace,
  serverVersion: "v99.999.9999+c1cdf14",
}
