import type { Meta, StoryObj } from "@storybook/react";
import type { SerpentGroup } from "api/typesGenerated";
import { UserAuthSettingsPageView } from "./UserAuthSettingsPageView";

const oidcGroup: SerpentGroup = {
  name: "OIDC",
  description: "",
};

const ghGroup: SerpentGroup = {
  name: "GitHub",
  description: "",
};

const meta: Meta<typeof UserAuthSettingsPageView> = {
  title: "pages/DeploySettingsPage/UserAuthSettingsPageView",
  component: UserAuthSettingsPageView,
  args: {
    options: [
      {
        name: "OIDC Client ID",
        description: "Client ID to use for Login with OIDC.",
        value: "1234",
        group: oidcGroup,
        flag: "oidc",
        flag_shorthand: "o",
        hidden: false,
      },
      {
        name: "OIDC Allow Signups",
        description: "Whether new users can sign up with OIDC.",
        value: true,
        group: oidcGroup,
        flag: "oidc",
        flag_shorthand: "o",
        hidden: false,
      },
      {
        name: "OIDC Email Domain",
        description:
          "Email domains that clients logging in with OIDC must match.",
        value: "@coder.com",
        group: oidcGroup,
        flag: "oidc",
        flag_shorthand: "o",
        hidden: false,
      },
      {
        name: "OIDC Issuer URL",
        description: "Issuer URL to use for Login with OIDC.",
        value: "https://coder.com",
        group: oidcGroup,
        flag: "oidc",
        flag_shorthand: "o",
        hidden: false,
      },
      {
        name: "OIDC Scopes",
        description: "Scopes to grant when authenticating with OIDC.",
        value: ["idk"],
        group: oidcGroup,
        flag: "oidc",
        flag_shorthand: "o",
        hidden: false,
      },
      {
        name: "OAuth2 GitHub Client ID",
        description: "Client ID for Login with GitHub.",
        value: "1224",
        group: ghGroup,
        flag: "oidc",
        flag_shorthand: "o",
        hidden: false,
      },
      {
        name: "OAuth2 GitHub Allow Signups",
        description: "Whether new users can sign up with GitHub.",
        value: true,
        group: ghGroup,
        flag: "oidc",
        flag_shorthand: "o",
        hidden: false,
      },
      {
        name: "OAuth2 GitHub Enterprise Base URL",
        description:
          "Base URL of a GitHub Enterprise deployment to use for Login with GitHub.",
        value: "https://google.com",
        group: ghGroup,
        flag: "oidc",
        flag_shorthand: "o",
        hidden: false,
      },
      {
        name: "OAuth2 GitHub Allowed Orgs",
        description:
          "Organizations the user must be a member of to Login with GitHub.",
        value: true,
        group: ghGroup,
        flag: "oidc",
        flag_shorthand: "o",
        hidden: false,
      },
      {
        name: "OAuth2 GitHub Allowed Teams",
        description:
          "Teams inside organizations the user must be a member of to Login with GitHub. Structured as: <organization-name>/<team-slug>.",
        value: true,
        group: ghGroup,
        flag: "oidc",
        flag_shorthand: "o",
        hidden: false,
      },
    ],
  },
};

export default meta;
type Story = StoryObj<typeof UserAuthSettingsPageView>;

export const Page: Story = {};
