import type { Meta, StoryObj } from "@storybook/react";
import type { WorkspaceAgent } from "api/typesGenerated";
import { ProxyContext, getPreferredProxy } from "contexts/ProxyContext";
import {
  MockProxyLatencies,
  MockWorkspaceResource,
  MockWorkspaceResourceMultipleAgents,
} from "testHelpers/entities";
import { AgentRowPreview } from "./AgentRowPreview";
import { Resources } from "./Resources";

const meta: Meta<typeof Resources> = {
  title: "modules/resources/Resources",
  component: Resources,
  args: {
    resources: [MockWorkspaceResource],
    agentRow: getAgentRow,
  },
};

export default meta;
type Story = StoryObj<typeof Resources>;

export const Example: Story = {};

export const MultipleAgents: Story = {
  args: {
    resources: [MockWorkspaceResourceMultipleAgents],
  },
};

const nullDevice = {
  created_at: "",
  job_id: "",
  workspace_transition: "start",
  type: "null_resource",
  hide: false,
  icon: "",
  daily_cost: 0,
} as const;

const short = {
  key: "Short",
  value: "Hi!",
  sensitive: false,
};
const long = {
  key: "Long",
  value: "The quick brown fox jumped over the lazy dog",
  sensitive: false,
};
const reallyLong = {
  key: "Really long",
  value:
    "Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.",
  sensitive: false,
};

export const Markdown: Story = {
  args: {
    resources: [
      {
        ...nullDevice,
        type: "workspace",
        id: "1",
        name: "Workspace",
        metadata: [
          { key: "text", value: "hello", sensitive: false },
          { key: "link", value: "[hello](#)", sensitive: false },
          { key: "b/i", value: "_hello_, **friend**!", sensitive: false },
          { key: "coder", value: "`beep boop`", sensitive: false },
        ],
      },

      // bits of Markdown that are intentionally not supported here
      {
        ...nullDevice,
        type: "unsupported",
        id: "2",
        name: "Unsupported",
        metadata: [
          {
            key: "multiple paragraphs",
            value: `home,

home on the range`,
            sensitive: false,
          },
          { key: "heading", value: "# HI", sensitive: false },
          { key: "image", value: "![go](/icon/go.svg)", sensitive: false },
        ],
      },
    ],
  },
};

export const BunchOfDevicesWithMetadata: Story = {
  args: {
    resources: [
      MockWorkspaceResource,
      {
        ...nullDevice,
        id: "e8c846da",
        name: "Short",
        metadata: [short],
      },
      {
        ...nullDevice,
        id: "a1b11343",
        name: "Long",
        metadata: [long],
      },
      {
        ...nullDevice,
        id: "09ab7e8c",
        name: "Really long",
        metadata: [reallyLong],
      },
      {
        ...nullDevice,
        id: "0a09fa91",
        name: "Many short",
        metadata: Array.from({ length: 8 }, (_, i) => ({
          ...short,
          key: `Short ${i}`,
        })),
      },
      {
        ...nullDevice,
        id: "d0b9eb9d",
        name: "Many long",
        metadata: Array.from({ length: 4 }, (_, i) => ({
          ...long,
          key: `Long ${i}`,
        })),
      },
      {
        ...nullDevice,
        id: "3af84e31",
        name: "Many really long",
        metadata: Array.from({ length: 8 }, (_, i) => ({
          ...reallyLong,
          key: `Really long ${i}`,
        })),
      },
      {
        ...nullDevice,
        id: "d0b9eb9d",
        name: "Couple long",
        metadata: Array.from({ length: 2 }, (_, i) => ({
          ...long,
          key: `Long ${i}`,
        })),
      },
      {
        ...nullDevice,
        id: "a6c69587",
        name: "Short and long",
        metadata: Array.from({ length: 8 }, (_, i) =>
          i % 2 === 0
            ? { ...short, key: `Short ${i}` }
            : { ...long, key: `Long ${i}` },
        ),
      },
    ],
    agentRow: getAgentRow,
  },
};

function getAgentRow(agent: WorkspaceAgent): JSX.Element {
  return (
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
      <AgentRowPreview key={agent.id} agent={agent} />
    </ProxyContext.Provider>
  );
}
