import type { Meta, StoryObj } from "@storybook/react";
import { ServiceBannerView } from "./ServiceBannerView";

const meta: Meta<typeof ServiceBannerView> = {
  title: "components/ServiceBannerView",
  component: ServiceBannerView,
};

export default meta;
type Story = StoryObj<typeof ServiceBannerView>;

export const Production: Story = {
  args: {
    message: "weeeee",
    backgroundColor: "#FFFFFF",
  },
};

export const Preview: Story = {
  args: {
    message: "weeeee",
    backgroundColor: "#000000",
    isPreview: true,
  },
};
