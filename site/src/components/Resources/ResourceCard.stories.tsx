import { Story } from "@storybook/react"
import {
  MockWorkspace,
  MockWorkspaceAgent,
  MockWorkspaceApp,
  MockWorkspaceResource,
} from "testHelpers/entities"
import { ResourceCard, ResourceCardProps } from "./ResourceCard"

export default {
  title: "components/ResourceCard",
  component: ResourceCard,
}

const Template: Story<ResourceCardProps> = (args) => <ResourceCard {...args} />

export const Example = Template.bind({})
Example.args = {
  resource: MockWorkspaceResource,
  workspace: MockWorkspace,
  applicationsHost: "https://dev.coder.com",
  hideSSHButton: false,
  showApps: true,
  serverVersion: MockWorkspaceAgent.version,
}

export const NotShowingApps = Template.bind({})
NotShowingApps.args = {
  ...Example.args,
  showApps: false,
}

export const HideSSHButton = Template.bind({})
HideSSHButton.args = {
  ...Example.args,
  hideSSHButton: true,
}

export const BunchOfApps = Template.bind({})
BunchOfApps.args = {
  ...Example.args,
  resource: {
    ...MockWorkspaceResource,
    agents: [
      {
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
    ],
  },
}

export const BunchOfMetadata = Template.bind({})
BunchOfMetadata.args = {
  ...Example.args,
  resource: {
    ...MockWorkspaceResource,
    metadata: [
      { key: "type", value: "kubernetes_pod", sensitive: false },
      {
        key: "CPU(limits, requests)",
        value: "2 cores, 500m",
        sensitive: false,
      },
      { key: "container image pull policy", value: "Always", sensitive: false },
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
}
