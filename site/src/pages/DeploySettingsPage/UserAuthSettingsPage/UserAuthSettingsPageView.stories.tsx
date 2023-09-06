import { ComponentMeta, Story } from "@storybook/react";
import {
  UserAuthSettingsPageView,
  UserAuthSettingsPageViewProps,
} from "./UserAuthSettingsPageView";

export default {
  title: "pages/UserAuthSettingsPageView",
  component: UserAuthSettingsPageView,
  args: {
    options: [
      {
        name: "OIDC Client ID",
        description: "Client ID to use for Login with OIDC.",
        value: "1234",
        group: {
          name: "OIDC",
        },
      },
      {
        name: "OIDC Allow Signups",
        description: "Whether new users can sign up with OIDC.",
        value: true,
        group: {
          name: "OIDC",
        },
      },
      {
        name: "OIDC Email Domain",
        description:
          "Email domains that clients logging in with OIDC must match.",
        value: "@coder.com",
        group: {
          name: "OIDC",
        },
      },
      {
        name: "OIDC Issuer URL",
        description: "Issuer URL to use for Login with OIDC.",
        value: "https://coder.com",
        group: {
          name: "OIDC",
        },
      },
      {
        name: "OIDC Scopes",
        description: "Scopes to grant when authenticating with OIDC.",
        value: ["idk"],
        group: {
          name: "OIDC",
        },
      },
      {
        name: "OAuth2 GitHub Client ID",
        description: "Client ID for Login with GitHub.",
        value: "1224",
        group: {
          name: "GitHub",
        },
      },
      {
        name: "OAuth2 GitHub Allow Signups",
        description: "Whether new users can sign up with GitHub.",
        value: true,
        group: {
          name: "GitHub",
        },
      },
      {
        name: "OAuth2 GitHub Enterprise Base URL",
        description:
          "Base URL of a GitHub Enterprise deployment to use for Login with GitHub.",
        value: "https://google.com",
        group: {
          name: "GitHub",
        },
      },
      {
        name: "OAuth2 GitHub Allowed Orgs",
        description:
          "Organizations the user must be a member of to Login with GitHub.",
        value: true,
        group: {
          name: "GitHub",
        },
      },
      {
        name: "OAuth2 GitHub Allowed Teams",
        description:
          "Teams inside organizations the user must be a member of to Login with GitHub. Structured as: <organization-name>/<team-slug>.",
        value: true,
        group: {
          name: "GitHub",
        },
      },
    ],
  },
} as ComponentMeta<typeof UserAuthSettingsPageView>;

const Template: Story<UserAuthSettingsPageViewProps> = (args) => (
  <UserAuthSettingsPageView {...args} />
);
export const Page = Template.bind({});
