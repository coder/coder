import type { Meta, StoryObj } from "@storybook/react";
import { chromatic } from "testHelpers/chromatic";
import { MockTemplateVersion } from "testHelpers/entities";
import { ProvisionerTagsPopover } from "./ProvisionerTagsPopover";
import { useArgs } from "@storybook/preview-api";

const meta: Meta<typeof ProvisionerTagsPopover> = {
  title: "component/ProvisionerTagsPopover",
  parameters: {
    chromatic,
    layout: "centered",
  },
  component: ProvisionerTagsPopover,
  args: {
    tags: MockTemplateVersion.job.tags,
  },
  render: function Render(args) {
    const [{ tags }, updateArgs] = useArgs();

    return (
      <ProvisionerTagsPopover
        {...args}
        tags={tags}
        onSubmit={({ key, value }) => {
          updateArgs({ tags: { ...tags, [key]: value } });
        }}
        onDelete={(key) => {
          const newTags = { ...tags };
          delete newTags[key];
          updateArgs({ tags: newTags });
        }}
      />
    );
  },
};

export default meta;
type Story = StoryObj<typeof ProvisionerTagsPopover>;

export const Example: Story = {};
