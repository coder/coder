import { Meta, StoryFn } from "@storybook/react";
import GitAuthPageView, { GitAuthPageViewProps } from "./GitAuthPageView";

export default {
  title: "pages/GitAuthPageView",
  component: GitAuthPageView,
} as Meta<typeof GitAuthPageView>;

const Template: StoryFn<GitAuthPageViewProps> = (args) => (
  <GitAuthPageView {...args} />
);

export const WebAuthenticated = Template.bind({});
WebAuthenticated.args = {
  gitAuth: {
    type: "BitBucket",
    authenticated: true,
    device: false,
    installations: [],
    app_install_url: "",
    app_installable: false,
    user: {
      avatar_url: "",
      login: "kylecarbs",
      name: "Kyle Carberry",
      profile_url: "",
    },
  },
};

export const DeviceUnauthenticated = Template.bind({});
DeviceUnauthenticated.args = {
  gitAuth: {
    type: "GitHub",
    authenticated: false,
    device: true,
    installations: [],
    app_install_url: "",
    app_installable: false,
  },
  gitAuthDevice: {
    device_code: "1234-5678",
    expires_in: 900,
    interval: 5,
    user_code: "ABCD-EFGH",
    verification_uri: "",
  },
};

export const DeviceUnauthenticatedError = Template.bind({});
DeviceUnauthenticatedError.args = {
  gitAuth: {
    type: "GitHub",
    authenticated: false,
    device: true,
    installations: [],
    app_install_url: "",
    app_installable: false,
  },
  gitAuthDevice: {
    device_code: "1234-5678",
    expires_in: 900,
    interval: 5,
    user_code: "ABCD-EFGH",
    verification_uri: "",
  },
  deviceExchangeError: {
    message: "Error exchanging device code.",
    detail: "expired_token",
  },
};

export const DeviceAuthenticatedNotInstalled = Template.bind({});
DeviceAuthenticatedNotInstalled.args = {
  viewGitAuthConfig: true,
  gitAuth: {
    type: "GitHub",
    authenticated: true,
    device: true,
    installations: [],
    app_install_url: "https://example.com",
    app_installable: true,
    user: {
      avatar_url: "",
      login: "kylecarbs",
      name: "Kyle Carberry",
      profile_url: "",
    },
  },
};

export const DeviceAuthenticatedInstalled = Template.bind({});
DeviceAuthenticatedInstalled.args = {
  gitAuth: {
    type: "GitHub",
    authenticated: true,
    device: true,
    installations: [
      {
        configure_url: "https://example.com",
        id: 1,
        account: {
          avatar_url: "https://github.com/coder.png",
          login: "coder",
          name: "Coder",
          profile_url: "https://github.com/coder",
        },
      },
    ],
    app_install_url: "https://example.com",
    app_installable: true,
    user: {
      avatar_url: "",
      login: "kylecarbs",
      name: "Kyle Carberry",
      profile_url: "",
    },
  },
};
