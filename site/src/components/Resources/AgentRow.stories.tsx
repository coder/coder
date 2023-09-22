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
import { AgentRow, LineWithID } from "./AgentRow";
import { ProxyContext, getPreferredProxy } from "contexts/ProxyContext";
import type { Meta, StoryObj } from "@storybook/react";

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

const storybookLogs: LineWithID[] = [
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
}));

const meta: Meta<typeof AgentRow> = {
  title: "components/AgentRow",
  component: AgentRow,
  args: {
    storybookLogs,
    agent: {
      ...MockWorkspaceAgent,
      logs_length: storybookLogs.length,
      startup_script:
        'set -eux -o pipefail\n\n# install and start code-server\ncurl -fsSL https://code-server.dev/install.sh | sh -s -- --method=standalone --prefix=/tmp/code-server --version 4.8.3\n/tmp/code-server/bin/code-server --auth none --port 13337 >/tmp/code-server.log 2>&1 &\n\n\nif [ ! -d ~/coder ]; then\n  mkdir -p ~/coder\n\n  git clone https://github.com/coder/coder ~/coder\nfi\n\nsudo service docker start\nDOTFILES_URI=" "\nrm -f ~/.personalize.log\nif [ -n "${DOTFILES_URI// }" ]; then\n  coder dotfiles "$DOTFILES_URI" -y 2>&1 | tee -a ~/.personalize.log\nfi\nif [ -x ~/personalize ]; then\n  ~/personalize 2>&1 | tee -a ~/.personalize.log\nelif [ -f ~/personalize ]; then\n  echo "~/personalize is not executable, skipping..." | tee -a ~/.personalize.log\nfi\n',
    },
    workspace: MockWorkspace,
    showApps: true,
    storybookAgentMetadata: defaultAgentMetadata,
  },
  decorators: [
    (Story) => (
      <ProxyContext.Provider
        value={{
          proxyLatencies: MockProxyLatencies,
          proxy: getPreferredProxy([], undefined),
          proxies: [],
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
        <Story />
      </ProxyContext.Provider>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof AgentRow>;

export const Example: Story = {};

export const HideSSHButton: Story = {
  args: {
    hideSSHButton: true,
  },
};

export const HideVSCodeDesktopButton: Story = {
  args: {
    hideVSCodeDesktopButton: true,
  },
};

export const NotShowingApps: Story = {
  args: {
    showApps: false,
  },
};

export const BunchOfApps: Story = {
  args: {
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
  },
};

export const Connecting: Story = {
  args: {
    agent: MockWorkspaceAgentConnecting,
    storybookAgentMetadata: [],
  },
};

export const Timeout: Story = {
  args: {
    agent: MockWorkspaceAgentTimeout,
  },
};

export const Starting: Story = {
  args: {
    agent: MockWorkspaceAgentStarting,
  },
};

export const Started: Story = {
  args: {
    agent: {
      ...MockWorkspaceAgentReady,
      logs_length: 1,
    },
  },
};

export const StartedNoMetadata: Story = {
  args: {
    ...Started.args,
    storybookAgentMetadata: [],
  },
};

export const StartTimeout: Story = {
  args: {
    agent: MockWorkspaceAgentStartTimeout,
  },
};

export const StartError: Story = {
  args: {
    agent: MockWorkspaceAgentStartError,
  },
};

export const ShuttingDown: Story = {
  args: {
    agent: MockWorkspaceAgentShuttingDown,
  },
};

export const ShutdownTimeout: Story = {
  args: {
    agent: MockWorkspaceAgentShutdownTimeout,
  },
};

export const ShutdownError: Story = {
  args: {
    agent: MockWorkspaceAgentShutdownError,
  },
};

export const Off: Story = {
  args: {
    agent: MockWorkspaceAgentOff,
  },
};

export const ShowingPortForward: Story = {
  decorators: [
    (Story) => (
      <ProxyContext.Provider
        value={{
          proxyLatencies: MockProxyLatencies,
          proxy: getPreferredProxy(
            MockWorkspaceProxies,
            MockPrimaryWorkspaceProxy,
          ),
          proxies: MockWorkspaceProxies,
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
        <Story />
      </ProxyContext.Provider>
    ),
  ],
};

export const Outdated: Story = {
  args: {
    agent: MockWorkspaceAgentOutdated,
    workspace: MockWorkspace,
    serverVersion: "v99.999.9999+c1cdf14",
  },
};
