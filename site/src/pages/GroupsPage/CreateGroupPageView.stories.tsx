import { CreateGroupPageView } from "./CreateGroupPageView";
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof CreateGroupPageView> = {
  title: "pages/CreateGroupPageView",
  component: CreateGroupPageView,
};

export default meta;
type Story = StoryObj<typeof CreateGroupPageView>;

export const Example: Story = {};
