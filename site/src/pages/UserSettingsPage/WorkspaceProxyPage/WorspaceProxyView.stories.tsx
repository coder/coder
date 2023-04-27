import { Story } from "@storybook/react"
import {
  makeMockApiError,
  MockRegions,
  MockPrimaryRegion,
  MockHealthyWildRegion,
} from "testHelpers/entities"
import {
  WorkspaceProxyPageView,
  WorkspaceProxyPageViewProps,
} from "./WorkspaceProxyView"

export default {
  title: "components/WorkspaceProxyPageView",
  component: WorkspaceProxyPageView,
  args: {
    onRegenerateClick: { action: "Submit" },
  },
}

const Template: Story<WorkspaceProxyPageViewProps> = (
  args: WorkspaceProxyPageViewProps,
) => <WorkspaceProxyPageView {...args} />

export const PrimarySelected = Template.bind({})
PrimarySelected.args = {
  isLoading: false,
  hasLoaded: true,
  proxies: MockRegions,
  preferredProxy: MockPrimaryRegion,
  onSelect: () => {
    return Promise.resolve()
  },
}

export const Example = Template.bind({})
Example.args = {
  isLoading: false,
  hasLoaded: true,
  proxies: MockRegions,
  preferredProxy: MockHealthyWildRegion,
  onSelect: () => {
    return Promise.resolve()
  },
}

export const Loading = Template.bind({})
Loading.args = {
  ...Example.args,
  isLoading: true,
  hasLoaded: false,
}

export const Empty = Template.bind({})
Empty.args = {
  ...Example.args,
  proxies: [],
}

export const WithProxiesError = Template.bind({})
WithProxiesError.args = {
  ...Example.args,
  hasLoaded: false,
  getWorkspaceProxiesError: makeMockApiError({
    message: "Failed to get proxies.",
  }),
}

export const WithSelectProxyError = Template.bind({})
WithSelectProxyError.args = {
  ...Example.args,
  hasLoaded: false,
  selectProxyError: makeMockApiError({
    message: "Failed to select proxy.",
  }),
}
