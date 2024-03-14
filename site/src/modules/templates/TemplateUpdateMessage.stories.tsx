import type { Meta, StoryObj } from "@storybook/react";
import { TemplateUpdateMessage } from "./TemplateUpdateMessage";

const meta: Meta<typeof TemplateUpdateMessage> = {
  title: "modules/templates/TemplateUpdateMessage",
  component: TemplateUpdateMessage,
  args: {
    children: `### Update message\nSome message here.`,
  },
};

export default meta;
type Story = StoryObj<typeof TemplateUpdateMessage>;

const Default: Story = {};

export { Default as TemplateUpdateMessage };
