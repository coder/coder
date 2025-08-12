import type { Meta, StoryObj } from "@storybook/react-vite";
import { action } from "storybook/actions";
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
		initialValues: { theme_preference: "", terminal_font: "" },
	},
};
