import { Story } from "@storybook/react"
import { LicenseBannerView, LicenseBannerViewProps } from "./LicenseBannerView"

export default {
  title: "components/LicenseBannerView",
  component: LicenseBannerView,
}

const Template: Story<LicenseBannerViewProps> = (args) => <LicenseBannerView {...args} />

export const OneWarning = Template.bind({})
OneWarning.args = {
  warnings: ["You have exceeded the number of seats in your license."],
}

export const TwoWarnings = Template.bind({})
TwoWarnings.args = {
  warnings: [
    "You have exceeded the number of seats in your license.",
    "You are flying too close to the sun.",
  ],
}
