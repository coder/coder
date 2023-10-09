import { Meta, StoryFn } from "@storybook/react";
import ExternalAuthPageView, {
  ExternalAuthPageViewProps,
} from "./ExternalAuthPageView";

export default {
  title: "pages/ExternalAuthPage",
  component: ExternalAuthPageView,
} as Meta<typeof ExternalAuthPageView>;

const Template: StoryFn<ExternalAuthPageViewProps> = (args) => (
  <ExternalAuthPageView {...args} />
);

export const WebAuthenticated = Template.bind({});
WebAuthenticated.args = {
  externalAuth: {
    authenticated: true,
    device: false,
    installations: [],
    app_install_url: "",
    app_installable: false,
    display_name: "BitBucket",
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
  externalAuth: {
    display_name: "GitHub",
    authenticated: false,
    device: true,
    installations: [],
    app_install_url: "",
    app_installable: false,
  },
  externalAuthDevice: {
    device_code: "1234-5678",
    expires_in: 900,
    interval: 5,
    user_code: "ABCD-EFGH",
    verification_uri: "",
  },
};

export const DeviceUnauthenticatedError = Template.bind({});
DeviceUnauthenticatedError.args = {
  externalAuth: {
    display_name: "GitHub",
    authenticated: false,
    device: true,
    installations: [],
    app_install_url: "",
    app_installable: false,
  },
  externalAuthDevice: {
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
  viewExternalAuthConfig: true,
  externalAuth: {
    display_name: "GitHub",
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
  externalAuth: {
    display_name: "GitHub",
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
