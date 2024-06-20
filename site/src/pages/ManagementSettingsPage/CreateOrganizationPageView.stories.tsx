import type { Meta, StoryObj } from "@storybook/react";
import { CreateOrganizationPageView } from "./CreateOrganizationPageView";

const meta: Meta<typeof CreateOrganizationPageView> = {
  title: "pages/CreateOrganizationPageView",
  component: CreateOrganizationPageView,
};

export default meta;
type Story = StoryObj<typeof CreateOrganizationPageView>;

export const Example: Story = {};
