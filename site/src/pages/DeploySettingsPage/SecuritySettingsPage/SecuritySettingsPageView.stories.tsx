import { ComponentMeta, Story } from "@storybook/react"
import {
  SecuritySettingsPageView,
  SecuritySettingsPageViewProps,
} from "./SecuritySettingsPageView"

export default {
  title: "pages/SecuritySettingsPageView",
  component: SecuritySettingsPageView,
  argTypes: {
    deploymentConfig: {
      defaultValue: {
        ssh_keygen_algorithm: {
          name: "key",
          usage: "something",
          value: "1234",
        },
        secure_auth_cookie: {
          name: "key",
          usage: "something",
          value: "1234",
        },
        tls: {
          enable: {
            name: "yes or no",
            usage: "something",
            value: true,
          },
          cert_file: {
            name: "yes or no",
            usage: "something",
            value: ["something"],
          },
          key_file: {
            name: "yes or no",
            usage: "something",
            value: ["something"],
          },
          min_version: {
            name: "yes or no",
            usage: "something",
            value: "something",
          },
        },
      },
    },
    featureAuditLogEnabled: {
      defaultValue: true,
    },
    featureBrowserOnlyEnabled: {
      defaultValue: true,
    },
  },
} as ComponentMeta<typeof SecuritySettingsPageView>

const Template: Story<SecuritySettingsPageViewProps> = (args) => (
  <SecuritySettingsPageView {...args} />
)
export const Page = Template.bind({})
