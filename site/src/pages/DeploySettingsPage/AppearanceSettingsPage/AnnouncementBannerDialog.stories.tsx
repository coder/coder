import { action } from "@storybook/addon-actions";
import type { Meta, StoryObj } from "@storybook/react";
import { AnnouncementBannerDialog } from "./AnnouncementBannerDialog";

const meta: Meta<typeof AnnouncementBannerDialog> = {
  title: "pages/DeploySettingsPage/AnnouncementBannerDialog",
  component: AnnouncementBannerDialog,
  args: {
    banner: {
      enabled: true,
      message: "The beep-bop will be boop-beeped on Saturday at 12AM PST.",
      background_color: "#ffaff3",
    },
    onCancel: action("onCancel"),
    onUpdate: () => Promise.resolve(void action("onUpdate")),
  },
};

export default meta;
type Story = StoryObj<typeof AnnouncementBannerDialog>;

const Example: Story = {};

export { Example as AnnouncementBannerDialog };
