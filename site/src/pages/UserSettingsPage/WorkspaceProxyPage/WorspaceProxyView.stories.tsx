import { Story } from "@storybook/react";
import {
  mockApiError,
  MockWorkspaceProxies,
  MockPrimaryWorkspaceProxy,
  MockHealthyWildWorkspaceProxy,
  MockProxyLatencies,
} from "testHelpers/entities";
import {
  WorkspaceProxyView,
  WorkspaceProxyViewProps,
} from "./WorkspaceProxyView";

export default {
  title: "components/WorkspaceProxyView",
  component: WorkspaceProxyView,
  args: {
    onRegenerateClick: { action: "Submit" },
  },
};

const Template: Story<WorkspaceProxyViewProps> = (
  args: WorkspaceProxyViewProps,
) => <WorkspaceProxyView {...args} />;

export const PrimarySelected = Template.bind({});
PrimarySelected.args = {
  isLoading: false,
  hasLoaded: true,
  proxies: MockWorkspaceProxies,
  proxyLatencies: MockProxyLatencies,
  preferredProxy: MockPrimaryWorkspaceProxy,
};

export const Example = Template.bind({});
Example.args = {
  isLoading: false,
  hasLoaded: true,
  proxies: MockWorkspaceProxies,
  proxyLatencies: MockProxyLatencies,
  preferredProxy: MockHealthyWildWorkspaceProxy,
};

export const Loading = Template.bind({});
Loading.args = {
  ...Example.args,
  isLoading: true,
  hasLoaded: false,
};

export const Empty = Template.bind({});
Empty.args = {
  ...Example.args,
  proxies: [],
};

export const WithProxiesError = Template.bind({});
WithProxiesError.args = {
  ...Example.args,
  hasLoaded: false,
  getWorkspaceProxiesError: mockApiError({
    message: "Failed to get proxies.",
  }),
};

export const WithSelectProxyError = Template.bind({});
WithSelectProxyError.args = {
  ...Example.args,
  hasLoaded: false,
  selectProxyError: mockApiError({
    message: "Failed to select proxy.",
  }),
};
