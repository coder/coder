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
  args: {
    deploymentOptions: [
      {
        name: "Access URL",
        description:
          "External URL to access your deployment. This must be accessible by all provisioned workspaces.",
        value: "https://dev.coder.com",
      },
      {
        name: "Wildcard Access URL",
        description:
          'Specifies the wildcard hostname to use for workspace applications in the form "*.example.com".',
        value: "*--apps.dev.coder.com",
      },
    ],
    deploymentDAUs: MockDeploymentDAUResponse,
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
