import { TemplateVersionWarnings } from "./TemplateVersionWarnings";
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof TemplateVersionWarnings> = {
  title: "components/TemplateVersionWarnings",
  component: TemplateVersionWarnings,
};

export default meta;
type Story = StoryObj<typeof TemplateVersionWarnings>;

export const UnsupportedWorkspaces: Story = {
  args: {
    warnings: ["UNSUPPORTED_WORKSPACES"],
  },
};
