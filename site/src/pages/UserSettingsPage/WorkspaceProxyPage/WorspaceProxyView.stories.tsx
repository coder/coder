import {
  mockApiError,
  MockWorkspaceProxies,
  MockPrimaryWorkspaceProxy,
  MockHealthyWildWorkspaceProxy,
  MockProxyLatencies,
} from "testHelpers/entities";
import { WorkspaceProxyView } from "./WorkspaceProxyView";
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof WorkspaceProxyView> = {
  title: "pages/UserSettingsPage/WorkspaceProxyView",
  component: WorkspaceProxyView,
};

export default meta;
type Story = StoryObj<typeof WorkspaceProxyView>;

export const PrimarySelected: Story = {
  args: {
    isLoading: false,
    hasLoaded: true,
    proxies: MockWorkspaceProxies,
    proxyLatencies: MockProxyLatencies,
    preferredProxy: MockPrimaryWorkspaceProxy,
  },
};

export const Example: Story = {
  args: {
    isLoading: false,
    hasLoaded: true,
    proxies: MockWorkspaceProxies,
    proxyLatencies: MockProxyLatencies,
    preferredProxy: MockHealthyWildWorkspaceProxy,
  },
};

export const Loading: Story = {
  args: {
    ...Example.args,
    isLoading: true,
    hasLoaded: false,
  },
};

export const Empty: Story = {
  args: {
    ...Example.args,
    proxies: [],
  },
};

export const WithProxiesError: Story = {
  args: {
    ...Example.args,
    hasLoaded: false,
    getWorkspaceProxiesError: mockApiError({
      message: "Failed to get proxies.",
    }),
  },
};

export const WithSelectProxyError: Story = {
  args: {
    ...Example.args,
    hasLoaded: false,
    selectProxyError: mockApiError({
      message: "Failed to select proxy.",
    }),
  },
};
