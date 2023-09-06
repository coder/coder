import { Story } from "@storybook/react";
import {
  MockPrimaryWorkspaceProxy,
  MockWorkspaceProxies,
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
  MockProxyLatencies,
} from "testHelpers/entities";
import { AgentRow, AgentRowProps } from "./AgentRow";
import { ProxyContext, getPreferredProxy } from "contexts/ProxyContext";
import { Region } from "api/typesGenerated";

export default {
  title: "components/AgentRow",
  component: AgentRow,
  args: {
    storybookStartupLogs: [
      "\x1b[91mCloning Git repository...",
      "\x1b[2;37;41mStarting Docker Daemon...",
      "\x1b[1;95mAdding some 🧙magic🧙...",
      "Starting VS Code...",
      "\r  0     0    0     0    0     0      0      0 --:--:-- --:--:-- --:--:--     0\r100  1475    0  1475    0     0   4231      0 --:--:-- --:--:-- --:--:--  4238",
    ].map((line, index) => ({
      id: index,
      level: "info",
      output: line,
      time: "",
    })),
  },
};

const Template: Story<AgentRowProps> = (args) => {
  return TemplateFC(args, [], undefined);
};

const TemplateWithPortForward: Story<AgentRowProps> = (args) => {
  return TemplateFC(args, MockWorkspaceProxies, MockPrimaryWorkspaceProxy);
};

const TemplateFC = (
  args: AgentRowProps,
  proxies: Region[],
  selectedProxy?: Region,
) => {
  return (
    <ProxyContext.Provider
      value={{
        proxyLatencies: MockProxyLatencies,
        proxy: getPreferredProxy(proxies, selectedProxy),
        proxies: proxies,
        isLoading: false,
        isFetched: true,
        setProxy: () => {
          return;
        },
        clearProxy: () => {
          return;
        },
        refetchProxyLatencies: (): Date => {
          return new Date();
        },
      }}
    >
      <AgentRow {...args} />
    </ProxyContext.Provider>
  );
};

const defaultAgentMetadata = [
  {
    result: {
      collected_at: "2021-05-05T00:00:00Z",
      error: "",
      value: "Master",
      age: 5,
    },
    description: {
      display_name: "Branch",
      key: "branch",
      interval: 10,
      timeout: 10,
      script: "git branch",
    },
  },
  {
    result: {
      collected_at: "2021-05-05T00:00:00Z",
      error: "",
      value: "No changes",
      age: 5,
    },
    description: {
      display_name: "Changes",
      key: "changes",
      interval: 10,
      timeout: 10,
      script: "git diff",
    },
  },
  {
    result: {
      collected_at: "2021-05-05T00:00:00Z",
      error: "",
      value: "2%",
      age: 5,
    },
    description: {
      display_name: "CPU Usage",
      key: "cpuUsage",
      interval: 10,
      timeout: 10,
      script: "cpu.sh",
    },
  },
  {
    result: {
      collected_at: "2021-05-05T00:00:00Z",
      error: "",
      value: "3%",
      age: 5,
    },
    description: {
      display_name: "Disk Usage",
      key: "diskUsage",
      interval: 10,
      timeout: 10,
      script: "disk.sh",
    },
  },
];

export const Example = Template.bind({});
Example.args = {
  agent: {
    ...MockWorkspaceAgent,
    startup_script:
      'set -eux -o pipefail\n\n# install and start code-server\ncurl -fsSL https://code-server.dev/install.sh | sh -s -- --method=standalone --prefix=/tmp/code-server --version 4.8.3\n/tmp/code-server/bin/code-server --auth none --port 13337 >/tmp/code-server.log 2>&1 &\n\n\nif [ ! -d ~/coder ]; then\n  mkdir -p ~/coder\n\n  git clone https://github.com/coder/coder ~/coder\nfi\n\nsudo service docker start\nDOTFILES_URI=" "\nrm -f ~/.personalize.log\nif [ -n "${DOTFILES_URI// }" ]; then\n  coder dotfiles "$DOTFILES_URI" -y 2>&1 | tee -a ~/.personalize.log\nfi\nif [ -x ~/personalize ]; then\n  ~/personalize 2>&1 | tee -a ~/.personalize.log\nelif [ -f ~/personalize ]; then\n  echo "~/personalize is not executable, skipping..." | tee -a ~/.personalize.log\nfi\n',
  },
  workspace: MockWorkspace,
  showApps: true,
  storybookAgentMetadata: defaultAgentMetadata,
};

export const HideSSHButton = Template.bind({});
HideSSHButton.args = {
  ...Example.args,
  hideSSHButton: true,
};

export const HideVSCodeDesktopButton = Template.bind({});
HideVSCodeDesktopButton.args = {
  ...Example.args,
  hideVSCodeDesktopButton: true,
};

export const NotShowingApps = Template.bind({});
NotShowingApps.args = {
  ...Example.args,
  showApps: false,
};

export const BunchOfApps = Template.bind({});
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
  showApps: true,
};

export const Connecting = Template.bind({});
Connecting.args = {
  ...Example.args,
  agent: MockWorkspaceAgentConnecting,
  storybookAgentMetadata: [],
};

export const Timeout = Template.bind({});
Timeout.args = {
  ...Example.args,
  agent: MockWorkspaceAgentTimeout,
};

export const Starting = Template.bind({});
Starting.args = {
  ...Example.args,
  agent: MockWorkspaceAgentStarting,
};

export const Started = Template.bind({});
Started.args = {
  ...Example.args,
  agent: {
    ...MockWorkspaceAgentReady,
    logs_length: 1,
  },
};

export const StartedNoMetadata = Template.bind({});
StartedNoMetadata.args = {
  ...Started.args,
  storybookAgentMetadata: [],
};

export const StartTimeout = Template.bind({});
StartTimeout.args = {
  ...Example.args,
  agent: MockWorkspaceAgentStartTimeout,
};

export const StartError = Template.bind({});
StartError.args = {
  ...Example.args,
  agent: MockWorkspaceAgentStartError,
};

export const ShuttingDown = Template.bind({});
ShuttingDown.args = {
  ...Example.args,
  agent: MockWorkspaceAgentShuttingDown,
};

export const ShutdownTimeout = Template.bind({});
ShutdownTimeout.args = {
  ...Example.args,
  agent: MockWorkspaceAgentShutdownTimeout,
};

export const ShutdownError = Template.bind({});
ShutdownError.args = {
  ...Example.args,
  agent: MockWorkspaceAgentShutdownError,
};

export const Off = Template.bind({});
Off.args = {
  ...Example.args,
  agent: MockWorkspaceAgentOff,
};

export const ShowingPortForward = TemplateWithPortForward.bind({});
ShowingPortForward.args = {
  ...Example.args,
};

export const Outdated = Template.bind({});
Outdated.args = {
  ...Example.args,
  agent: MockWorkspaceAgentOutdated,
  workspace: MockWorkspace,
  serverVersion: "v99.999.9999+c1cdf14",
};
