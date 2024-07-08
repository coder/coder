import type { Meta, StoryObj } from "@storybook/react";
import { mockApiError } from "testHelpers/entities";
import { CreateGroupPageView } from "./CreateGroupPageView";

const meta: Meta<typeof CreateGroupPageView> = {
  title: "pages/GroupsPage/CreateGroupPageView",
  component: CreateGroupPageView,
};

export default meta;
type Story = StoryObj<typeof CreateGroupPageView>;

export const Example: Story = {};

export const WithError: Story = {
  args: {
    error: mockApiError({
      message: "A group named new-group already exists.",
      validations: [{ field: "name", detail: "Group names must be unique" }],
    }),
    initialTouched: { name: true },
  },
};
