import { SetupPageView } from "./SetupPageView";
import { mockApiError } from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof SetupPageView> = {
  title: "pages/SetupPage",
  component: SetupPageView,
};

export default meta;
type Story = StoryObj<typeof SetupPageView>;

export const Ready: Story = {};

export const FormError: Story = {
  args: {
    error: mockApiError({
      validations: [{ field: "username", detail: "Username taken" }],
    }),
  },
};

export const Loading: Story = {
  args: {
    isLoading: true,
  },
};
