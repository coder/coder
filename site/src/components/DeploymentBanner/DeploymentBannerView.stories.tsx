import { Story } from "@storybook/react"
import { MockDeploymentStats } from "testHelpers/entities"
import {
  DeploymentBannerView,
  DeploymentBannerViewProps,
} from "./DeploymentBannerView"

export default {
  title: "components/DeploymentBannerView",
  component: DeploymentBannerView,
}

const Template: Story<DeploymentBannerViewProps> = (args) => (
  <DeploymentBannerView {...args} />
)

export const Preview = Template.bind({})
Preview.args = {
  stats: MockDeploymentStats,
}
