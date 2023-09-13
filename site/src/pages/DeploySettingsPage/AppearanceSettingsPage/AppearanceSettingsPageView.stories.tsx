import { AppearanceSettingsPageView } from "./AppearanceSettingsPageView";
import type { Meta, StoryObj } from "@storybook/react";

const meta: Meta<typeof AppearanceSettingsPageView> = {
  title: "pages/AppearanceSettingsPageView",
  component: AppearanceSettingsPageView,
  args: {
    appearance: {
      logo_url: "https://github.com/coder.png",
      service_banner: {
        enabled: true,
        message: "hello world",
        background_color: "white",
      },
    },
    isEntitled: false,
    updateAppearance: () => {
      return undefined;
    },
  },
};

export default meta;
type Story = StoryObj<typeof AppearanceSettingsPageView>;

export const Page: Story = {};
