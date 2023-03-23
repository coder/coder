import { Story } from "@storybook/react"
import { WorkspaceAgentMetadataResult } from "api/typesGenerated"
import { AgentMetadataView, AgentMetadataViewProps } from "./AgentMetadata"

export default {
  title: "components/AgentMetadata",
  component: AgentMetadataView,
}

const Template: Story<AgentMetadataViewProps> = (args) => (
  <AgentMetadataView {...args} />
)

const resultDefaults: WorkspaceAgentMetadataResult = {
  collected_at: "2021-05-05T00:00:00Z",
  error: "",
  value: "defvalue",
  age: 5,
}

const descriptionDefaults: WorkspaceAgentMetadataDescription = {
  interval: 10,
  timeout: 10,
  script: "some command",
}

export const Example = Template.bind({})
Example.args = {
  metadata: [
    {
      result: {
        value: "110%",
        ...resultDefaults,
      },
      description: {
        display_name: "CPU",
        key: "CPU",
        ...descriptionDefaults,
      },
    },
    {
      result: {
        value: "50GB",
        ...resultDefaults,
      },
      description: {
        display_name: "Memory",
        key: "Memory",
        ...descriptionDefaults,
      },
    },
    {
      result: {
        value: "cant see it",
        ...resultDefaults,
        age: 50,
      },
      description: {
        ...descriptionDefaults,
        display_name: "Stale",
        key: "stale",
      },
    },
    {
      result: {
        ...resultDefaults,
        value: "oops",
        error: "fatal error",
      },
      description: {
        display_name: "Error",
        key: "stale",
        ...descriptionDefaults,
      },
    },
    {
      result: {
        ...resultDefaults,
        value: "oops",
        error: "fatal error",
      },
      description: {
        display_name: "Error",
        key: "stale",
        ...descriptionDefaults,
      },
    },
  ],
}
