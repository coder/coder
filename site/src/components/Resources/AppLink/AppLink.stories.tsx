import {
  MockPrimaryWorkspaceProxy,
  MockWorkspaceProxies,
  MockWorkspace,
  MockWorkspaceAgent,
  MockWorkspaceApp,
  MockProxyLatencies,
} from "testHelpers/entities";
import { AppLink } from "./AppLink";
import { ProxyContext, getPreferredProxy } from "contexts/ProxyContext";
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof AppLink> = {
  title: "components/AppLink",
  component: AppLink,
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

export default meta;
type Story = StoryObj<typeof AppLink>;

export const WithIcon: Story = {
  args: {
    workspace: MockWorkspace,
    app: {
      ...MockWorkspaceApp,
      icon: "/icon/code.svg",
      sharing_level: "owner",
      health: "healthy",
    },
    agent: MockWorkspaceAgent,
  },
};

export const ExternalApp: Story = {
  args: {
    workspace: MockWorkspace,
    app: {
      ...MockWorkspaceApp,
      external: true,
    },
    agent: MockWorkspaceAgent,
  },
};

export const SharingLevelOwner: Story = {
  args: {
    workspace: MockWorkspace,
    app: {
      ...MockWorkspaceApp,
      sharing_level: "owner",
    },
    agent: MockWorkspaceAgent,
  },
};

export const SharingLevelAuthenticated: Story = {
  args: {
    workspace: MockWorkspace,
    app: {
      ...MockWorkspaceApp,
      sharing_level: "authenticated",
    },
    agent: MockWorkspaceAgent,
  },
};

export const SharingLevelPublic: Story = {
  args: {
    workspace: MockWorkspace,
    app: {
      ...MockWorkspaceApp,
      sharing_level: "public",
    },
    agent: MockWorkspaceAgent,
  },
};

export const HealthDisabled: Story = {
  args: {
    workspace: MockWorkspace,
    app: {
      ...MockWorkspaceApp,
      sharing_level: "owner",
      health: "disabled",
    },
    agent: MockWorkspaceAgent,
  },
};

export const HealthInitializing: Story = {
  args: {
    workspace: MockWorkspace,
    app: {
      ...MockWorkspaceApp,
      health: "initializing",
    },
    agent: MockWorkspaceAgent,
  },
};

export const HealthUnhealthy: Story = {
  args: {
    workspace: MockWorkspace,
    app: {
      ...MockWorkspaceApp,
      health: "unhealthy",
    },
    agent: MockWorkspaceAgent,
  },
};
