import type { StoryObj } from "@storybook/react";
import { MockOAuth2Apps } from "testHelpers/entities";
import OAuth2AppsSettingsPageView from "./OAuth2AppsSettingsPageView";

export default {
  title: "pages/DeploySettingsPage/OAuth2AppsSettingsPageView",
  component: OAuth2AppsSettingsPageView,
};

type Story = StoryObj<typeof OAuth2AppsSettingsPageView>;

export const Loading: Story = {
  args: {
    isLoading: true,
  },
};

export const Unentitled: Story = {
  args: {
    isLoading: false,
    apps: MockOAuth2Apps,
  },
};

export const Entitled: Story = {
  args: {
    isLoading: false,
    apps: MockOAuth2Apps,
    isEntitled: true,
  },
};

export const Empty: Story = {
  args: {
    isLoading: false,
  },
};
