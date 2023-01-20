import { ComponentMeta, Story } from "@storybook/react"
import { makeMockApiError, MockDeploymentDAUResponse } from "testHelpers/entities"
import { MetricsPageView, MetricsPageViewProps } from "./MetricsPageView"

export default {
  title: "pages/MetricsPageView",
  component: MetricsPageView,
} as ComponentMeta<typeof MetricsPageView>

const Template: Story<MetricsPageViewProps> = (args) => (
  <MetricsPageView {...args} />
)

export const MetricsPage = Template.bind({})
MetricsPage.args = {
  deploymentDAUs: MockDeploymentDAUResponse,
  getDeploymentDAUsError: undefined
}

export const MetricsPageError = Template.bind({})
MetricsPageError.args = {
  deploymentDAUs: undefined,
  getDeploymentDAUsError: makeMockApiError({})
}
