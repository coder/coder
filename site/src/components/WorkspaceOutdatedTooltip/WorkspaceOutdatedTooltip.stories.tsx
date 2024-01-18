import { action } from "@storybook/addon-actions";
import type { Meta, StoryObj } from "@storybook/react";
import { useQueryClient } from "react-query";
import { MockTemplateVersion, MockTemplate } from "testHelpers/entities";
import { WorkspaceOutdatedTooltip } from "./WorkspaceOutdatedTooltip";

const meta: Meta<typeof WorkspaceOutdatedTooltip> = {
  title: "components/WorkspaceOutdatedTooltip",
  component: WorkspaceOutdatedTooltip,
  decorators: [
    (Story) => {
      const queryClient = useQueryClient();
      queryClient.setQueryData(
        ["templateVersion", MockTemplateVersion.id],
        MockTemplateVersion,
      );
      return <Story />;
    },
  ],
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
