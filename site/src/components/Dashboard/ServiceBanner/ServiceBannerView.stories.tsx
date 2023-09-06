import { Story } from "@storybook/react";
import { ServiceBannerView, ServiceBannerViewProps } from "./ServiceBannerView";

export default {
  title: "components/ServiceBannerView",
  component: ServiceBannerView,
};

const Template: Story<ServiceBannerViewProps> = (args) => (
  <ServiceBannerView {...args} />
);

export const Production = Template.bind({});
Production.args = {
  message: "weeeee",
  backgroundColor: "#FFFFFF",
};

export const Preview = Template.bind({});
Preview.args = {
  message: "weeeee",
  backgroundColor: "#000000",
  preview: true,
};
