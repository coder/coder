import type { Meta, StoryObj } from "@storybook/react";
import { chromatic } from "testHelpers/chromatic";
import { mockApiError } from "testHelpers/entities";
import { SetupPageView } from "./SetupPageView";

const meta: Meta<typeof SetupPageView> = {
  title: "pages/SetupPage",
  parameters: { chromatic },
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
