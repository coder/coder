import { MockTemplate } from "testHelpers/entities";
import { SelectedTemplate } from "./SelectedTemplate";
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof SelectedTemplate> = {
  title: "components/SelectedTemplate",
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
