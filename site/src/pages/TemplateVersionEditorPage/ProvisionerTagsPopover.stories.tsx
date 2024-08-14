import type { Meta, StoryObj } from "@storybook/react";
import { userEvent, within } from "@storybook/test";
import { chromatic } from "testHelpers/chromatic";
import { MockTemplateVersion } from "testHelpers/entities";
import { ProvisionerTagsPopover } from "./ProvisionerTagsPopover";

const meta: Meta<typeof ProvisionerTagsPopover> = {
  title: "pages/TemplateVersionEditorPage/ProvisionerTagsPopover",
  parameters: {
    chromatic,
    layout: "centered",
  },
  component: ProvisionerTagsPopover,
  args: {
    tags: MockTemplateVersion.job.tags,
  },
};

export default meta;
type Story = StoryObj<typeof ProvisionerTagsPopover>;

const Example: Story = {
  play: async ({ canvasElement, step }) => {
    const canvas = within(canvasElement);

    await step("Open popover", async () => {
      await userEvent.click(canvas.getByRole("button"));
    });
  },
};

export { Example as ProvisionerTagsPopover };
