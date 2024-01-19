import { action } from "@storybook/addon-actions";
import type { Meta, StoryObj } from "@storybook/react";
import { MockTemplateVersion, MockTemplate } from "testHelpers/entities";
import { WorkspaceOutdatedTooltip } from "./WorkspaceOutdatedTooltip";

const meta: Meta<typeof WorkspaceOutdatedTooltip> = {
  title: "components/WorkspaceOutdatedTooltip",
  component: WorkspaceOutdatedTooltip,
  parameters: {
    queries: [
      {
        key: ["templateVersion", MockTemplateVersion.id],
        data: MockTemplateVersion,
      },
    ],
  },
  args: {
    onUpdateVersion: action("onUpdateVersion"),
    templateName: MockTemplate.display_name,
    latestVersionId: MockTemplateVersion.id,
  },
};

export default meta;
type Story = StoryObj<typeof WorkspaceOutdatedTooltip>;

const Example: Story = {};

export { Example as WorkspaceOutdatedTooltip };
