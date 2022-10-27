import { Story } from "@storybook/react"
import { LicenseBannerView, LicenseBannerViewProps } from "./LicenseBannerView"

export default {
  title: "components/LicenseBannerView",
  component: LicenseBannerView,
}

const Template: Story<LicenseBannerViewProps> = (args) => (
  <LicenseBannerView {...args} />
)

export const OneWarning = Template.bind({})
OneWarning.args = {
  errors: [],
  warnings: ["You have exceeded the number of seats in your license."],
}

export const TwoWarnings = Template.bind({})
TwoWarnings.args = {
  errors: [],
  warnings: [
    "You have exceeded the number of seats in your license.",
    "You are flying too close to the sun.",
  ],
}

export const OneError = Template.bind({})
OneError.args = {
  errors: [
    "You have multiple replicas but high availability is an Enterprise feature. You will be unable to connect to workspaces.",
  ],
  warnings: [],
}
