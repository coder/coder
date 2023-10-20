import { mockApiError, MockTokens } from "testHelpers/entities";
import { TokensPageView } from "./TokensPageView";
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof TokensPageView> = {
  title: "pages/UserSettingsPage/TokensPageView",
  component: TokensPageView,
  args: {
    isLoading: false,
    hasLoaded: true,
    tokens: MockTokens,
    onDelete: () => {
      return Promise.resolve();
    },
  },
};

export default meta;
type Story = StoryObj<typeof TokensPageView>;

export const Example: Story = {};

export const Loading: Story = {
  args: {
    isLoading: true,
    hasLoaded: false,
  },
};

export const Empty: Story = {
  args: {
    tokens: [],
  },
};

export const WithGetTokensError: Story = {
  args: {
    hasLoaded: false,
    getTokensError: mockApiError({
      message: "Failed to get tokens.",
    }),
  },
};

export const WithDeleteTokenError: Story = {
  args: {
    hasLoaded: false,
    deleteTokenError: mockApiError({
      message: "Failed to delete token.",
    }),
  },
};
