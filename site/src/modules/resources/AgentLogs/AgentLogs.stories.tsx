import type { Meta, StoryObj } from "@storybook/react";
import { AGENT_LOG_LINE_HEIGHT } from "./AgentLogLine";
import { AgentLogs } from "./AgentLogs";
import { MockLogs, MockSources } from "./mocks";

const meta: Meta<typeof AgentLogs> = {
  title: "modules/resources/AgentLogs",
  component: AgentLogs,
  args: {
    sources: MockSources,
    logs: MockLogs,
    height: MockLogs.length * AGENT_LOG_LINE_HEIGHT,
  },
  parameters: {
    layout: "fullscreen",
  },
};

export default meta;
type Story = StoryObj<typeof AgentLogs>;

const Default: Story = {};

export { Default as AgentLogs };
