import type { Meta, StoryObj } from "@storybook/react";
import { AppearanceSettingsPageView } from "./AppearanceSettingsPageView";

const meta: Meta<typeof AppearanceSettingsPageView> = {
  title: "pages/DeploySettingsPage/AppearanceSettingsPageView",
  component: AppearanceSettingsPageView,
  args: {
    appearance: {
      application_name: "Foobar",
      logo_url: "https://github.com/coder.png",
      service_banner: {
        enabled: false,
        message: "",
        background_color: "#00ff00",
      },
      announcement_banners: [
        {
          enabled: true,
          message: "The beep-bop will be boop-beeped on Saturday at 12AM PST.",
          background_color: "#ffaff3",
        },
      ],
    },
    isEntitled: false,
  },
};

export default meta;
type Story = StoryObj<typeof AppearanceSettingsPageView>;

export const Entitled: Story = {
  args: {
    isEntitled: true,
  },
};

export const NotEntitled: Story = {};
