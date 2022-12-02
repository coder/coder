import { Story } from "@storybook/react"
import { ServiceBannerView, ServiceBannerViewProps } from "./ServiceBannerView"

export default {
  title: "components/LicenseBannerView",
  component: ServiceBannerView,
}

const Template: Story<ServiceBannerViewProps> = (args) => (
  <ServiceBannerView {...args} />
)

export const GoodColor = Template.bind({})
GoodColor.args = {
  message: "weeeee",
  backgroundColor: "#00FF00",
}
