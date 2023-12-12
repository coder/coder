import {
  MockGithubAuthLink,
  MockGithubExternalProvider,
} from "testHelpers/entities";
import { UserExternalAuthSettingsPageView } from "./UserExternalAuthSettingsPageView";
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof UserExternalAuthSettingsPageView> = {
  title: "pages/UserExternalAuthSettingsPage/UserExternalAuthSettingsPageView",
  component: UserExternalAuthSettingsPageView,
  args: {
    isLoading: false,
    getAuthsError: undefined,
    unlinked: 0,
    auths: {
      providers: [],
      links: [],
    },
    onUnlinkExternalAuth: () => {},
    onValidateExternalAuth: () => {},
  },
};

export default meta;
type Story = StoryObj<typeof UserExternalAuthSettingsPageView>;

export const NoProviders: Story = {};

export const Authenticated: Story = {
  args: {
    ...meta.args,
    auths: {
      providers: [MockGithubExternalProvider],
      links: [MockGithubAuthLink],
    },
  },
};

export const UnAuthenticated: Story = {
  args: {
    ...meta.args,
    auths: {
      providers: [MockGithubExternalProvider],
      links: [
        {
          ...MockGithubAuthLink,
          authenticated: false,
        },
      ],
    },
  },
};
