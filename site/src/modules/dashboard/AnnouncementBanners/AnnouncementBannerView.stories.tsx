import type { Meta, StoryObj } from "@storybook/react";
import { AnnouncementBannerView } from "./AnnouncementBannerView";

const meta: Meta<typeof AnnouncementBannerView> = {
  title: "modules/dashboard/AnnouncementBannerView",
  component: AnnouncementBannerView,
};

export default meta;
type Story = StoryObj<typeof AnnouncementBannerView>;

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
