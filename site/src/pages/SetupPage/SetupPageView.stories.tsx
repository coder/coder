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

export const TrialError: Story = {
  args: {
    error: mockApiError({
      message: "Couldn't generate trial!",
      detail: "It looks like your team is already trying Coder.",
    }),
  },
};

export const Loading: Story = {
  args: {
    isLoading: true,
  },
};
