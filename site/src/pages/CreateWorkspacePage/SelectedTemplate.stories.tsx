import type { Meta, StoryObj } from "@storybook/react";
import { MockTemplate } from "testHelpers/entities";
import { SelectedTemplate } from "./SelectedTemplate";

const meta: Meta<typeof SelectedTemplate> = {
  title: "pages/CreateWorkspacePage/SelectedTemplate",
  component: SelectedTemplate,
};

export default meta;
type Story = StoryObj<typeof SelectedTemplate>;

export const WithIcon: Story = {
  args: {
    template: {
      ...MockTemplate,
      icon: "/icon/docker.png",
    },
  },
};

export const WithoutIcon: Story = {
  args: {
    template: {
      ...MockTemplate,
      icon: "",
    },
  },
};
