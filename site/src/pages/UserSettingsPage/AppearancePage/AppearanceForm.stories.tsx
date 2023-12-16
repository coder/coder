import { action } from "@storybook/addon-actions";
import type { Meta, StoryObj } from "@storybook/react";
import { AppearanceForm } from "./AppearanceForm";

const onUpdateTheme = action("update");

const meta: Meta<typeof AppearanceForm> = {
  title: "pages/UserSettingsPage/AppearanceForm",
  component: AppearanceForm,
  args: {
    onSubmit: (update) =>
      Promise.resolve(onUpdateTheme(update.theme_preference)),
  },
};

export default meta;
type Story = StoryObj<typeof AppearanceForm>;

export const Example: Story = {
  args: {
    enableAuto: true,
    initialValues: { theme_preference: "" },
  },
};
