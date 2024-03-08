import type { Meta, StoryObj } from "@storybook/react";
import {
  MockGithubAuthLink,
  MockGithubExternalProvider,
} from "testHelpers/entities";
import { ExternalAuthPageView } from "./ExternalAuthPageView";

const meta: Meta<typeof ExternalAuthPageView> = {
  title: "pages/UserSettingsPage/ExternalAuthPageView",
  component: ExternalAuthPageView,
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
type Story = StoryObj<typeof ExternalAuthPageView>;

export const NoProviders: Story = {};

export const NoIcon: Story = {
  args: {
    ...meta.args,
    auths: {
      providers: [{ ...MockGithubExternalProvider, display_icon: "" }],
      links: [MockGithubAuthLink],
    },
  },
};

export const Authenticated: Story = {
  args: {
    ...meta.args,
    auths: {
      providers: [MockGithubExternalProvider],
      links: [MockGithubAuthLink],
    },
  },
};

export const Unauthenticated: Story = {
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
