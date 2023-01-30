import { ComponentMeta, Story } from "@storybook/react"
import {
  makeMockApiError,
  MockDeploymentDAUResponse,
} from "testHelpers/entities"
import {
  GeneralSettingsPageView,
  GeneralSettingsPageViewProps,
} from "./GeneralSettingsPageView"

export default {
  title: "pages/GeneralSettingsPageView",
  component: GeneralSettingsPageView,
  argTypes: {
    deploymentConfig: {
      defaultValue: {
        access_url: {
          name: "Access URL",
          usage:
            "External URL to access your deployment. This must be accessible by all provisioned workspaces.",
          value: "https://dev.coder.com",
        },
        wildcard_access_url: {
          name: "Wildcard Access URL",
          usage:
            'Specifies the wildcard hostname to use for workspace applications in the form "*.example.com".',
          value: "*--apps.dev.coder.com",
        },
      },
    },
    deploymentDAUs: {
      defaultValue: MockDeploymentDAUResponse,
    },
  },
} as ComponentMeta<typeof GeneralSettingsPageView>

const Template: Story<GeneralSettingsPageViewProps> = (args) => (
  <GeneralSettingsPageView {...args} />
)
export const Page = Template.bind({})

export const NoDAUs = Template.bind({})
NoDAUs.args = {
  deploymentDAUs: undefined,
}

export const DAUError = Template.bind({})
DAUError.args = {
  deploymentDAUs: undefined,
  getDeploymentDAUsError: makeMockApiError({ message: "Error fetching DAUs." }),
}
