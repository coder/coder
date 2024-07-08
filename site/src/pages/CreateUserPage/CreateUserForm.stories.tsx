import { action } from "@storybook/addon-actions";
import type { StoryObj, Meta } from "@storybook/react";
import { mockApiError } from "testHelpers/entities";
import { CreateUserForm } from "./CreateUserForm";

const meta: Meta<typeof CreateUserForm> = {
  title: "pages/CreateUserPage",
  component: CreateUserForm,
  args: {
    onCancel: action("cancel"),
    onSubmit: action("submit"),
    isLoading: false,
  },
};

export default meta;
type Story = StoryObj<typeof CreateUserForm>;

export const Ready: Story = {};

export const FormError: Story = {
  args: {
    error: mockApiError({
      validations: [{ field: "username", detail: "Username taken" }],
    }),
  },
};

export const GeneralError: Story = {
  args: {
    error: mockApiError({
      message: "User already exists",
    }),
  },
};

export const Loading: Story = {
  args: {
    isLoading: true,
  },
};
