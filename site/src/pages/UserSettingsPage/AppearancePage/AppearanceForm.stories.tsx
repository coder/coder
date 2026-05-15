import type { Meta, StoryObj } from "@storybook/react-vite";
import { action } from "storybook/actions";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import type {
	UpdateUserAppearanceSettingsRequest,
	UserAppearanceSettings,
} from "#/api/typesGenerated";
import { CONCRETE_THEMES } from "#/theme";
import { AppearanceForm } from "./AppearanceForm";

const onUpdateTheme = action("update");

const baseSettings: UserAppearanceSettings = {
	theme_preference: "dark",
	theme_mode: "single",
	theme_light: "light",
	theme_dark: "dark",
	terminal_font: "",
};

const resolvedSubmit = () =>
	fn((update: UpdateUserAppearanceSettingsRequest) => {
		onUpdateTheme(update);
		return Promise.resolve({ ...baseSettings, ...update });
	});

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

export const SelectSingleLightDefault: Story = {
	args: {
		initialValues: { ...baseSettings, theme_preference: "dark" },
		onSubmit: resolvedSubmit(),
	},
	play: async ({ canvasElement, args }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		await user.click(
			await canvas.findByRole("radio", { name: /light default/i }),
		);

		await waitFor(() => {
			expect(args.onSubmit).toHaveBeenCalledWith({
				theme_preference: "light",
				theme_mode: "single",
				theme_light: "light",
				theme_dark: "dark",
				terminal_font: "geist-mono",
			});
		});
		expect(
			await canvas.findByRole("radio", { name: /light default/i }),
		).toBeChecked();
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
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const lightOptions = await canvas.findByRole("group", {
			name: "Light theme options",
		});
		const darkOptions = await canvas.findByRole("group", {
			name: "Dark theme options",
		});

		expect(within(lightOptions).getAllByRole("radio")).toHaveLength(
			CONCRETE_THEMES.length,
		);
		expect(within(darkOptions).getAllByRole("radio")).toHaveLength(
			CONCRETE_THEMES.length,
		);
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

export const SelectSyncMode: Story = {
	args: {
		initialValues: { ...baseSettings, theme_preference: "dark" },
		onSubmit: resolvedSubmit(),
	},
	play: async ({ canvasElement, args }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const dropdown = await canvas.findByRole("combobox", {
			name: "Theme mode",
		});

		await user.click(dropdown);
		await user.click(
			await within(document.body).findByRole("option", {
				name: "Sync with system",
			}),
		);

		await waitFor(() => {
			expect(args.onSubmit).toHaveBeenCalledWith({
				theme_preference: "dark",
				theme_mode: "sync",
				theme_light: "light",
				theme_dark: "dark",
				terminal_font: "geist-mono",
			});
		});
		expect(dropdown).toHaveTextContent("Sync with system");
	},
};

export const SelectSingleFromSync: Story = {
	args: {
		activeScheme: "light",
		initialValues: {
			...baseSettings,
			theme_preference: "light-protan-deuter",
			theme_mode: "sync",
			theme_light: "light-protan-deuter",
			theme_dark: "dark-tritan",
		},
		onSubmit: resolvedSubmit(),
	},
	play: async ({ canvasElement, args }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const dropdown = await canvas.findByRole("combobox", {
			name: "Theme mode",
		});

		await user.click(dropdown);
		await user.click(
			await within(document.body).findByRole("option", {
				name: "Single theme",
			}),
		);

		await waitFor(() => {
			expect(args.onSubmit).toHaveBeenCalledWith({
				theme_preference: "light-protan-deuter",
				theme_mode: "single",
				theme_light: "light-protan-deuter",
				theme_dark: "dark-tritan",
				terminal_font: "geist-mono",
			});
		});
		expect(dropdown).toHaveTextContent("Single theme");
		expect(
			await canvas.findByRole("radio", {
				name: /light protanopia and deuteranopia/i,
			}),
		).toBeChecked();
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

export const SelectDarkThemeInLightSyncSlot: Story = {
	args: {
		activeScheme: "light",
		initialValues: {
			...baseSettings,
			theme_preference: "light",
			theme_mode: "sync",
			theme_light: "light",
			theme_dark: "dark",
		},
		onSubmit: resolvedSubmit(),
	},
	play: async ({ canvasElement, args }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const lightOptions = await canvas.findByRole("group", {
			name: "Light theme options",
		});
		const darkTritanopia = await within(lightOptions).findByRole("radio", {
			name: /dark tritanopia/i,
		});

		await user.click(darkTritanopia);

		await waitFor(() => {
			expect(args.onSubmit).toHaveBeenCalledWith({
				theme_preference: "dark-tritan",
				theme_mode: "sync",
				theme_light: "dark-tritan",
				theme_dark: "dark",
				terminal_font: "geist-mono",
			});
		});
		expect(darkTritanopia).toBeChecked();
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
		const lightOptions = await canvas.findByRole("group", {
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

export const SelectTerminalFont: Story = {
	args: {
		initialValues: { ...baseSettings, theme_preference: "dark" },
		onSubmit: resolvedSubmit(),
	},
	play: async ({ canvasElement, args }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		await user.click(await canvas.findByText("Fira Code"));

		await waitFor(() => {
			expect(args.onSubmit).toHaveBeenCalledWith({
				theme_preference: "dark",
				theme_mode: "single",
				theme_light: "light",
				theme_dark: "dark",
				terminal_font: "fira-code",
			});
		});
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
