import { Story } from "@storybook/react";
import { MockWorkspaceAgent, MockWorkspaceApp } from "testHelpers/entities";
import { AgentRowPreview, AgentRowPreviewProps } from "./AgentRowPreview";

export default {
  title: "components/AgentRowPreview",
  component: AgentRowPreview,
};

const Template: Story<AgentRowPreviewProps> = (args) => (
  <AgentRowPreview {...args} />
);

export const Example = Template.bind({});
Example.args = {
  agent: MockWorkspaceAgent,
};

export const BunchOfApps = Template.bind({});
BunchOfApps.args = {
  ...Example.args,
  agent: {
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
};

export const NoApps = Template.bind({});
NoApps.args = {
  ...Example.args,
  agent: {
    ...MockWorkspaceAgent,
    apps: [],
  },
};
