import type { Meta, StoryObj } from "@storybook/react";
import { mockApiError } from "testHelpers/entities";
import { AccountForm } from "./AccountForm";

const meta: Meta<typeof AccountForm> = {
  title: "pages/UserSettingsPage/AccountForm",
  component: AccountForm,
  args: {
    email: "test-user@org.com",
    isLoading: false,
    initialValues: {
      username: "test-user",
      name: "Test User",
    },
    updateProfileError: undefined,
  },
};

export default meta;
type Story = StoryObj<typeof AccountForm>;

export const Example: Story = {};

export const Loading: Story = {
  args: {
    isLoading: true,
  },
};
export const WithError: Story = {
  args: {
    updateProfileError: mockApiError({
      message: "Username is invalid",
      validations: [
        {
          field: "username",
          detail: "Username is too long.",
        },
      ],
    }),
    initialTouched: {
      username: true,
    },
  },
};
