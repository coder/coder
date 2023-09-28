import { AppearanceSettingsPageView } from "./AppearanceSettingsPageView";
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof AppearanceSettingsPageView> = {
  title: "pages/AppearanceSettingsPageView",
  component: AppearanceSettingsPageView,
  args: {
    appearance: {
      application_name: "",
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
