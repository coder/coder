import type { Meta, StoryObj } from "@storybook/react";
import { AppearanceForm } from "./AppearanceForm";

const meta: Meta<typeof AppearanceForm> = {
  title: "pages/UserSettingsPage/AppearanceForm",
  component: AppearanceForm,
  args: {
    isLoading: false,
  },
};

export default meta;
type Story = StoryObj<typeof AppearanceForm>;

export const Example: Story = {};
