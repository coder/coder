import type { Meta, StoryObj } from "@storybook/react-vite";
import { action } from "storybook/actions";
import { userEvent, within } from "storybook/test";
import type { UserAppearanceSettings } from "#/api/typesGenerated";
import { AppearanceForm } from "./AppearanceForm";

const onUpdateTheme = action("update");

const baseSettings: UserAppearanceSettings = {
	theme_preference: "dark",
	theme_mode: "single",
	theme_light: "light",
	theme_dark: "dark",
	terminal_font: "",
};

const meta: Meta<typeof AppearanceForm> = {
	title: "pages/UserSettingsPage/AppearanceForm",
	component: AppearanceForm,
	args: {
		activeScheme: "dark",
		onSubmit: (update) => Promise.resolve(onUpdateTheme(update)),
	},
};

export default meta;
type Story = StoryObj<typeof AppearanceForm>;

export const SingleDarkDefault: Story = {
	args: {
		initialValues: { ...baseSettings, theme_preference: "dark" },
	},
};

export const SingleLightDefault: Story = {
	args: {
		activeScheme: "light",
		initialValues: { ...baseSettings, theme_preference: "light" },
	},
};

export const SingleDarkProtanDeuter: Story = {
	args: {
		initialValues: { ...baseSettings, theme_preference: "dark-protan-deuter" },
	},
};

export const SingleLightTritan: Story = {
	args: {
		activeScheme: "light",
		initialValues: { ...baseSettings, theme_preference: "light-tritan" },
	},
};

export const SyncDefault: Story = {
	args: {
		initialValues: {
			...baseSettings,
			theme_mode: "sync",
			theme_light: "light",
			theme_dark: "dark",
		},
	},
};

export const SyncActiveLight: Story = {
	args: {
		activeScheme: "light",
		initialValues: {
			...baseSettings,
			theme_mode: "sync",
			theme_light: "light",
			theme_dark: "dark",
		},
	},
};

export const SyncProtanDeuter: Story = {
	args: {
		initialValues: {
			...baseSettings,
			theme_mode: "sync",
			theme_light: "light-protan-deuter",
			theme_dark: "dark-protan-deuter",
		},
	},
};

export const SyncTritan: Story = {
	args: {
		initialValues: {
			...baseSettings,
			theme_mode: "sync",
			theme_light: "light-tritan",
			theme_dark: "dark-tritan",
		},
	},
};

// Slots set to different families. Exercises the card UI when the
// user has picked, say, default light for day and colorblind dark for
// night (or vice versa).
export const SyncMixed: Story = {
	args: {
		initialValues: {
			...baseSettings,
			theme_mode: "sync",
			theme_light: "light",
			theme_dark: "dark-tritan",
		},
	},
};

export const SyncHoverPreview: Story = {
	args: {
		initialValues: {
			...baseSettings,
			theme_mode: "sync",
			theme_light: "light",
			theme_dark: "dark",
		},
	},
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const lightOptions = await canvas.findByRole("radiogroup", {
			name: "Light theme options",
		});
		const radio = await within(lightOptions).findByRole("radio", {
			name: "Light tritanopia",
		});
		const swatch = radio.closest("label");
		if (swatch === null) {
			throw new Error("Expected the theme radio to be inside a swatch label.");
		}
		await user.hover(swatch);
	},
};

// Migration paths: settings without the new fields but with a legacy
// `auto-*` theme_preference should render in sync mode on mount.
export const LegacyAutoTritan: Story = {
	args: {
		initialValues: {
			theme_preference: "auto-tritan",
			// Legacy rows predate the theme_mode field.
			theme_mode: "",
			theme_light: "",
			theme_dark: "",
			terminal_font: "",
		},
	},
};

export const LegacyAuto: Story = {
	args: {
		initialValues: {
			theme_preference: "auto",
			theme_mode: "",
			theme_light: "",
			theme_dark: "",
			terminal_font: "",
		},
	},
};
