/**
 * Theme mode helpers for the Appearance settings page.
 *
 * The UI exposes two modes:
 * - "sync": Coder picks a concrete theme for each OS color scheme,
 *   with the current scheme deciding which slot is rendered.
 * - "single": Coder renders one concrete theme regardless of OS.
 *
 * The persisted state is four fields on `UserAppearanceSettings`:
 * `theme_mode`, `theme_light`, `theme_dark`, plus the pre-existing
 * `theme_preference`. This module owns translating between the flat
 * persisted shape and the richer form draft shape, plus the one-time
 * migration of the legacy `auto` value into sync state.
 */

import type { TerminalFontName } from "#/api/typesGenerated";
import { DEFAULT_THEME } from ".";
import {
	type ConcreteThemeName,
	isConcreteThemeName,
	legacyAutoToSync,
} from "./colorblind";

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

/**
 * The form keeps every slot populated even when the user is not
 * actively in that mode, so switching the dropdown never loses the
 * user's other-mode selection mid-interaction. Persisted storage is
 * derived from this shape.
 */
export type ThemeModeDraft = {
	mode: ThemeMode;
	single: ConcreteThemeName;
	light: ConcreteThemeName;
	dark: ConcreteThemeName;
};

/**
 * Shape of the subset of `UserAppearanceSettings` this module reads.
 * Typed structurally to avoid a circular dependency on `typesGenerated`
 * (and to keep the tests independent of codegen ordering).
 */
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
 *
 * Precedence:
 * 1. New fields (`theme_mode` + slot pair) win when present and valid.
 * 2. Legacy `auto`, `auto-protan-deuter`, `auto-tritan` become sync
 *    mode, reusing valid persisted slots when present.
 * 3. A concrete legacy `theme_preference` becomes single mode with
 *    that theme.
 * 4. Anything else falls back to single mode + DEFAULT_THEME.
 *
 * Sync-mode slots that are invalid fall back to the plain `light` or
 * `dark` theme for the respective OS scheme so the user still sees
 * something sensible even if storage gets hand-edited.
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

/**
 * Picks the concrete theme that should render right now given the
 * user's state and the OS color scheme. Used by ThemeProvider on every
 * render and by the Appearance page to decide which card gets the
 * "Active" pill.
 */
export const resolveActiveThemeName = (
	state: ThemeModeState,
	osScheme: "dark" | "light",
): ConcreteThemeName => {
	if (state.mode === "sync") {
		return osScheme === "dark" ? state.dark : state.light;
	}
	return state.theme;
};

/**
 * Switches from sync to single mode, picking whichever slot matches
 * the user's current OS color scheme so nothing visibly changes.
 * No-op when already in single mode.
 */
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

/**
 * Maps the form draft to the API request. The legacy `theme_preference`
 * field is populated with whichever concrete theme best represents the
 * current selection, so older clients still reading that column see a
 * plausible theme:
 *
 * - Single mode: mirror the single theme exactly.
 * - Sync mode: mirror the slot currently active for the OS scheme. This
 *   keeps legacy readers aligned with what the user sees right now.
 */
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

/**
 * Seeds a form draft from decoded persisted state. Slots the user
 * isn't actively using are filled with sensible defaults so switching
 * modes later feels continuous rather than destructive.
 *
 * `persistedSlots` is the raw `theme_light` / `theme_dark` from storage.
 * When the user is currently in single mode but previously persisted
 * sync slots (i.e. they had sync-mode selections before switching),
 * those slots are reused verbatim so toggling the dropdown back to
 * sync restores their prior pair. Unknown or empty slot values fall
 * back to the `FAMILY_PAIR` of the single theme so the slots are
 * always populated with concrete names.
 */
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
