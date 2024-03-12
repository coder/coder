import type { Meta, StoryObj } from "@storybook/react";
import {
  MockWorkspaceAgent,
  MockWorkspaceAgentConnecting,
  MockWorkspaceImageResource,
  MockWorkspaceResource,
  MockWorkspaceVolumeResource,
} from "testHelpers/entities";
import { TemplateResourcesTable } from "./TemplateResourcesTable";

const meta: Meta<typeof TemplateResourcesTable> = {
  title: "modules/templates/TemplateResourcesTable",
  component: TemplateResourcesTable,
};

export default meta;
type Story = StoryObj<typeof TemplateResourcesTable>;

const Default: Story = {
  args: {
    resources: [
      MockWorkspaceResource,
      MockWorkspaceVolumeResource,
      MockWorkspaceImageResource,
    ],
  },
};

export const MultipleAgents: Story = {
  args: {
    resources: [
      {
        ...MockWorkspaceResource,
        agents: [MockWorkspaceAgent, MockWorkspaceAgentConnecting],
      },
    ],
  },
};

export { Default as TemplateResourcesTable };
