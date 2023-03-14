import { ComponentMeta, Story } from "@storybook/react"
import { DeploymentOption } from "api/types"
import {
  SecuritySettingsPageView,
  SecuritySettingsPageViewProps,
} from "./SecuritySettingsPageView"

export default {
  title: "pages/SecuritySettingsPageView",
  component: SecuritySettingsPageView,
  argTypes: {
    options: {
      defaultValue: [
        {
          name: "SSH Keygen Algorithm",
          usage: "something",
          value: "1234",
        },
        {
          name: "Secure Auth Cookie",
          usage: "something",
          value: "1234",
        },
        {
          name: "TLS Version",
          usage: "something",
          value: ["something"],
          group: {
            name: "TLS",
          },
        },
      ],
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

export const NoTLS = Template.bind({})
NoTLS.args = {
  options: [
    {
      name: "SSH Keygen Algorithm",
      value: "1234",
    } as DeploymentOption,
    {
      name: "Secure Auth Cookie",
      value: "1234",
    } as DeploymentOption,
  ],
}
