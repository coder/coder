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
        enabled: true,
        message: "hello world",
        background_color: "white",
      },
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
