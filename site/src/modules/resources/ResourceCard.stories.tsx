import {
  MockProxyLatencies,
  MockWorkspaceResource,
} from "testHelpers/entities";
import { ResourceCard } from "./ResourceCard";
import { ProxyContext, getPreferredProxy } from "contexts/ProxyContext";
import type { Meta, StoryObj } from "@storybook/react";
import { type WorkspaceAgent } from "api/typesGenerated";
import { AgentRowPreview } from "./AgentRowPreview";

const meta: Meta<typeof ResourceCard> = {
  title: "modules/resources/ResourceCard",
  component: ResourceCard,
  args: {
    resource: MockWorkspaceResource,
    agentRow: getAgentRow,
  },
};

export default meta;
type Story = StoryObj<typeof ResourceCard>;

export const Example: Story = {};

export const BunchOfMetadata: Story = {
  args: {
    resource: {
      ...MockWorkspaceResource,
      metadata: [
        {
          key: "CPU(limits, requests)",
          value: "2 cores, 500m",
          sensitive: false,
        },
        {
          key: "container image pull policy",
          value: "Always",
          sensitive: false,
        },
        { key: "Disk", value: "10GiB", sensitive: false },
        {
          key: "image",
          value: "docker.io/markmilligan/pycharm-community:latest",
          sensitive: false,
        },
        { key: "kubernetes namespace", value: "oss", sensitive: false },
        {
          key: "memory(limits, requests)",
          value: "4GB, 500mi",
          sensitive: false,
        },
        {
          key: "security context - container",
          value: "run_as_user 1000",
          sensitive: false,
        },
        {
          key: "security context - pod",
          value: "run_as_user 1000 fs_group 1000",
          sensitive: false,
        },
        { key: "volume", value: "/home/coder", sensitive: false },
        {
          key: "secret",
          value: "3XqfNW0b1bvsGsqud8O6OW6VabH3fwzI",
          sensitive: true,
        },
      ],
    },
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
      <AgentRowPreview agent={agent} key={agent.id} />
    </ProxyContext.Provider>
  );
}
