import type { Meta, StoryObj } from "@storybook/react";
import { MockOAuth2ProviderApps } from "testHelpers/entities";
import OAuth2AppsSettingsPageView from "./OAuth2AppsSettingsPageView";

const meta: Meta = {
  title: "pages/DeploySettingsPage/OAuth2AppsSettingsPageView",
  component: OAuth2AppsSettingsPageView,
};
export default meta;

type Story = StoryObj<typeof OAuth2AppsSettingsPageView>;

export const Loading: Story = {
  args: {
    isLoading: true,
  },
};

export const Error: Story = {
  args: {
    isLoading: false,
    error: "some error",
  },
};

export const Apps: Story = {
  args: {
    isLoading: false,
    apps: MockOAuth2ProviderApps,
  },
};

export const Empty: Story = {
  args: {
    isLoading: false,
  },
};
