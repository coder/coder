import { action } from "@storybook/addon-actions"
import { Story } from "@storybook/react"
import { MockWorkspace, MockWorkspaceResource } from "testHelpers/entities"
import { AgentRow } from "./AgentRow"
import { ResourceCard, ResourceCardProps } from "./ResourceCard"

export default {
  title: "components/ResourceCard",
  component: ResourceCard,
}

const Template: Story<ResourceCardProps> = (args) => <ResourceCard {...args} />

export const Example = Template.bind({})
Example.args = {
  resource: MockWorkspaceResource,
  agentRow: (agent) => (
    <AgentRow
      showApps
      key={agent.id}
      agent={agent}
      workspace={MockWorkspace}
      applicationsHost=""
      serverVersion=""
      onUpdateAgent={action("updateAgent")}
    />
  ),
}

export const BunchOfMetadata = Template.bind({})
BunchOfMetadata.args = {
  ...Example.args,
  resource: {
    ...MockWorkspaceResource,
    metadata: [
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
  agentRow: (agent) => (
    <AgentRow
      showApps
      key={agent.id}
      agent={agent}
      workspace={MockWorkspace}
      applicationsHost=""
      serverVersion=""
      onUpdateAgent={action("updateAgent")}
    />
  ),
}
