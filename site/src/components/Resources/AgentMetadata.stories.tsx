import { Story } from "@storybook/react";
import {
  WorkspaceAgentMetadataDescription,
  WorkspaceAgentMetadataResult,
} from "api/typesGenerated";
import { AgentMetadataView, AgentMetadataViewProps } from "./AgentMetadata";

export default {
  title: "components/AgentMetadata",
  component: AgentMetadataView,
};

const Template: Story<AgentMetadataViewProps> = (args) => (
  <AgentMetadataView {...args} />
);

const resultDefaults: WorkspaceAgentMetadataResult = {
  collected_at: "2021-05-05T00:00:00Z",
  error: "",
  value: "defvalue",
  age: 5,
};

const descriptionDefaults: WorkspaceAgentMetadataDescription = {
  display_name: "DisPlay",
  key: "defkey",
  interval: 10,
  timeout: 10,
  script: "some command",
};

export const Example = Template.bind({});
Example.args = {
  metadata: [
    {
      result: {
        ...resultDefaults,
        value: "110%",
      },
      description: {
        ...descriptionDefaults,
        display_name: "CPU",
        key: "CPU",
      },
    },
    {
      result: {
        ...resultDefaults,
        value: "50GB",
      },
      description: {
        ...descriptionDefaults,
        display_name: "Memory",
        key: "Memory",
      },
    },
    {
      result: {
        ...resultDefaults,
        value: "stale value",
        age: 300,
      },
      description: {
        ...descriptionDefaults,
        interval: 5,
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
        ...descriptionDefaults,
        display_name: "Error",
        key: "error",
      },
    },
    {
      result: {
        ...resultDefaults,
        value: "",
        collected_at: "0001-01-01T00:00:00Z",
        age: 1000000,
      },
      description: {
        ...descriptionDefaults,
        display_name: "Never loads",
        key: "nloads",
      },
    },
    {
      result: {
        ...resultDefaults,
        value: "r".repeat(1000),
      },
      description: {
        ...descriptionDefaults,
        display_name: "Really, really big",
        key: "big",
      },
    },
  ],
};
