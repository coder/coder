import type { TerminalFontName } from "#/api/typesGenerated";
import {
	type ConcreteThemeName,
	DEFAULT_THEME,
	isConcreteThemeName,
	legacyAutoToSync,
} from ".";

type ThemeMode = "sync" | "single";

type SyncState = {
	mode: "sync";
	light: ConcreteThemeName;
	dark: ConcreteThemeName;
};

type SingleState = {
	mode: "single";
	theme: ConcreteThemeName;
};

type ThemeModeState = SyncState | SingleState;

export type ThemeModeDraft = {
	mode: ThemeMode;
	single: ConcreteThemeName;
	light: ConcreteThemeName;
	dark: ConcreteThemeName;
};

type AppearanceSettingsLike = {
	theme_preference?: string;
	theme_mode?: string;
	theme_light?: string;
	theme_dark?: string;
};

const coerceConcrete = (
	value: string | undefined,
	fallback: ConcreteThemeName,
): ConcreteThemeName => (isConcreteThemeName(value) ? value : fallback);

/**
 * Maps every concrete theme to its opposite-scheme counterpart in the
 * same palette family. Written out explicitly so the mapping is easy
 * to review; a string-manipulation version would obscure edge cases
 * like the bare `dark`/`light` pair versus the hyphenated variants.
 */
type ThemeFamilyPair = {
	light: ConcreteThemeName;
	dark: ConcreteThemeName;
};

const FAMILY_PAIR = {
	light: { light: "light", dark: "dark" },
	dark: { light: "light", dark: "dark" },
	"light-protan-deuter": {
		light: "light-protan-deuter",
		dark: "dark-protan-deuter",
	},
	"dark-protan-deuter": {
		light: "light-protan-deuter",
		dark: "dark-protan-deuter",
	},
	"light-tritan": { light: "light-tritan", dark: "dark-tritan" },
	"dark-tritan": { light: "light-tritan", dark: "dark-tritan" },
} satisfies Record<ConcreteThemeName, ThemeFamilyPair>;

/**
 * Decodes the persisted appearance settings into the form's working
 * state, applying the one-time migration of legacy `auto-*` values.
 */
export const migrateLegacyPreference = (
	settings: AppearanceSettingsLike,
): ThemeModeState => {
	const mode = settings.theme_mode;

	if (mode === "sync") {
		return {
			mode: "sync",
			light: coerceConcrete(settings.theme_light, "light"),
			dark: coerceConcrete(settings.theme_dark, "dark"),
		};
	}

	if (mode === "single") {
		return {
			mode: "single",
			theme: coerceConcrete(settings.theme_preference, DEFAULT_THEME),
		};
	}

	// No recognized theme_mode. Inspect the legacy theme_preference.
	const legacySync = legacyAutoToSync(settings.theme_preference);
	if (legacySync) {
		return {
			mode: "sync",
			light: coerceConcrete(settings.theme_light, legacySync.light),
			dark: coerceConcrete(settings.theme_dark, legacySync.dark),
		};
	}

	return {
		mode: "single",
		theme: coerceConcrete(settings.theme_preference, DEFAULT_THEME),
	};
};

export const resolveActiveThemeName = (
	state: ThemeModeState,
	osScheme: "dark" | "light",
): ConcreteThemeName => {
	if (state.mode === "sync") {
		return osScheme === "dark" ? state.dark : state.light;
	}
	return state.theme;
};

export const switchToSingle = (
	state: ThemeModeState,
	osScheme: "dark" | "light",
): SingleState => {
	if (state.mode === "single") {
		return state;
	}
	return {
		mode: "single",
		theme: osScheme === "dark" ? state.dark : state.light,
	};
};

/**
 * Flat request shape sent to the backend. Kept in sync with
 * `codersdk.UpdateUserAppearanceSettingsRequest`; this helper lets the
 * form code stay ignorant of the exact field ordering.
 */
type AppearanceUpdate = {
	theme_preference: string;
	theme_mode: ThemeMode;
	theme_light: ConcreteThemeName;
	theme_dark: ConcreteThemeName;
	terminal_font: TerminalFontName;
};

export const draftToUpdate = (
	draft: ThemeModeDraft,
	terminalFont: TerminalFontName,
	activeScheme: "dark" | "light",
): AppearanceUpdate => {
	const themePreference =
		draft.mode === "single"
			? draft.single
			: activeScheme === "dark"
				? draft.dark
				: draft.light;
	return {
		theme_preference: themePreference,
		theme_mode: draft.mode,
		theme_light: draft.light,
		theme_dark: draft.dark,
		terminal_font: terminalFont,
	};
};

export const draftFromState = (
	state: ThemeModeState,
	persistedSlots?: { light?: string; dark?: string },
): ThemeModeDraft => {
	if (state.mode === "sync") {
		// Seed the "single" slot from the dark slot since historical
		// default behavior preferred dark. If the user later switches
		// modes via the dropdown, `switchToSingle` overrides this with
		// the OS-matching slot anyway.
		return {
			mode: "sync",
			single: state.dark,
			light: state.light,
			dark: state.dark,
		};
	}
	const pair = FAMILY_PAIR[state.theme];
	return {
		mode: "single",
		single: state.theme,
		light: coerceConcrete(persistedSlots?.light, pair.light),
		dark: coerceConcrete(persistedSlots?.dark, pair.dark),
	};
};
