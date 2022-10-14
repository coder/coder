import { Story } from "@storybook/react"
import {
  ResourceAgentLatency,
  ResourceAgentLatencyProps,
} from "./ResourceAgentLatency"

export default {
  title: "components/ResourceAgentLatency",
  component: ResourceAgentLatency,
}

const Template: Story<ResourceAgentLatencyProps> = (args) => (
  <ResourceAgentLatency {...args} />
)

export const Single = Template.bind({})
Single.args = {
  latency: {
    "Coder DERP": {
      latency_ms: 100.52,
      preferred: true,
    },
  },
}

export const Multiple = Template.bind({})
Multiple.args = {
  latency: {
    Chicago: {
      latency_ms: 22.25551,
      preferred: false,
    },
    "New York": {
      latency_ms: 56.1111,
      preferred: true,
    },
    "San Francisco": {
      latency_ms: 125.11,
      preferred: false,
    },
    Tokyo: {
      latency_ms: 255,
      preferred: false,
    },
  },
}
