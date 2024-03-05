import type { Meta, StoryObj } from "@storybook/react";
import { MockOAuth2ProviderApps } from "testHelpers/entities";
import OAuth2ProviderPageView from "./OAuth2ProviderPageView";

const meta: Meta<typeof OAuth2ProviderPageView> = {
  title: "pages/UserSettingsPage/OAuth2ProviderPageView",
  component: OAuth2ProviderPageView,
};

export default meta;
type Story = StoryObj<typeof OAuth2ProviderPageView>;

export const Loading: Story = {
  args: {
    isLoading: true,
    error: undefined,
    revoke: () => undefined,
  },
};

export const Error: Story = {
  args: {
    isLoading: false,
    error: "some error",
    revoke: () => undefined,
  },
};

export const Apps: Story = {
  args: {
    isLoading: false,
    error: undefined,
    apps: MockOAuth2ProviderApps,
    revoke: () => undefined,
  },
};
