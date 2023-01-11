import { ComponentMeta, Story } from "@storybook/react"
import {
  UserAuthSettingsPageView,
  UserAuthSettingsPageViewProps,
} from "./UserAuthSettingsPageView"

export default {
  title: "pages/UserAuthSettingsPageView",
  component: UserAuthSettingsPageView,
  argTypes: {
    deploymentConfig: {
      defaultValue: {
        oidc: {
          client_id: {
            name: "OIDC Client ID",
            usage: "Client ID to use for Login with OIDC.",
            value: "1234",
          },
          allow_signups: {
            name: "OIDC Allow Signups",
            usage: "Whether new users can sign up with OIDC.",
            value: true,
          },
          email_domain: {
            name: "OIDC Email Domain",
            usage:
              "Email domains that clients logging in with OIDC must match.",
            value: "@coder.com",
          },
          issuer_url: {
            name: "OIDC Issuer URL",
            usage: "Issuer URL to use for Login with OIDC.",
            value: "https://coder.com",
          },
          scopes: {
            name: "OIDC Scopes",
            usage: "Scopes to grant when authenticating with OIDC.",
            value: ["idk"],
          },
        },
        oauth2: {
          github: {
            client_id: {
              name: "OAuth2 GitHub Client ID",
              usage: "Client ID for Login with GitHub.",
              value: "1224",
            },
            allow_signups: {
              name: "OAuth2 GitHub Allow Signups",
              usage: "Whether new users can sign up with GitHub.",
              value: true,
            },
            enterprise_base_url: {
              name: "OAuth2 GitHub Enterprise Base URL",
              usage:
                "Base URL of a GitHub Enterprise deployment to use for Login with GitHub.",
              value: "https://google.com",
            },
            allowed_orgs: {
              name: "OAuth2 GitHub Allowed Orgs",
              usage:
                "Organizations the user must be a member of to Login with GitHub.",
              value: true,
            },
            allowed_teams: {
              name: "OAuth2 GitHub Allowed Teams",
              usage:
                "Teams inside organizations the user must be a member of to Login with GitHub. Structured as: <organization-name>/<team-slug>.",
              value: true,
            },
          },
        },
      },
    },
  },
} as ComponentMeta<typeof UserAuthSettingsPageView>

const Template: Story<UserAuthSettingsPageViewProps> = (args) => (
  <UserAuthSettingsPageView {...args} />
)
export const Page = Template.bind({})
