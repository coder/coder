import { action } from "@storybook/addon-actions";
import type { Meta, StoryObj } from "@storybook/react";
import { expect, userEvent, waitFor, within } from "@storybook/test";
import { MockTemplateVersion, MockTemplate } from "testHelpers/entities";
import { WorkspaceOutdatedTooltip } from "./WorkspaceOutdatedTooltip";

const meta: Meta<typeof WorkspaceOutdatedTooltip> = {
  title: "modules/workspaces/WorkspaceOutdatedTooltip",
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

const Example: Story = {
  play: async ({ canvasElement, step }) => {
    const screen = within(canvasElement);

    await step("activate hover trigger", async () => {
      await userEvent.hover(screen.getByRole("button"));
      await waitFor(() =>
        expect(
          screen.getByText(MockTemplateVersion.message),
        ).toBeInTheDocument(),
      );
    });
  },
};

export { Example as WorkspaceOutdatedTooltip };
