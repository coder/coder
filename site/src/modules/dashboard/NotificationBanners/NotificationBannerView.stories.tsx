import type { Meta, StoryObj } from "@storybook/react";
import { NotificationBannerView } from "./NotificationBannerView";

const meta: Meta<typeof NotificationBannerView> = {
  title: "modules/dashboard/NotificationBannerView",
  component: NotificationBannerView,
};

export default meta;
type Story = StoryObj<typeof NotificationBannerView>;

export const Production: Story = {
  args: {
    message: "Unfortunately, there's a radio connected to my brain.",
    backgroundColor: "#ffaff3",
  },
};

export const Preview: Story = {
  args: {
    message: "バアン バン バン バン バアン ブレイバアン！",
    backgroundColor: "#4cd473",
  },
};
