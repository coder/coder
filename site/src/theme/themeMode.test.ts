import { DEFAULT_THEME } from ".";
import {
	migrateLegacyPreference,
	resolveActiveThemeName,
	stateToUpdate,
	switchToSingle,
	switchToSync,
} from "./themeMode";

// A fake `UserAppearanceSettings` shape. We avoid importing the real
// type to keep these tests independent of codegen ordering.
const settings = (overrides: Record<string, string | undefined> = {}) => ({
	theme_preference: overrides.theme_preference ?? "",
	theme_mode: overrides.theme_mode ?? "",
	theme_light: overrides.theme_light ?? "",
	theme_dark: overrides.theme_dark ?? "",
	terminal_font: overrides.terminal_font ?? "",
});

describe("migrateLegacyPreference", () => {
	it("prefers the new fields when theme_mode=sync is set", () => {
		expect(
			migrateLegacyPreference(
				settings({
					theme_mode: "sync",
					theme_light: "light-tritan",
					theme_dark: "dark-tritan",
					// Legacy field is ignored in sync mode.
					theme_preference: "dark",
				}),
			),
		).toEqual({
			mode: "sync",
			light: "light-tritan",
			dark: "dark-tritan",
		});
	});

	it("prefers the new fields when theme_mode=single is set", () => {
		expect(
			migrateLegacyPreference(
				settings({
					theme_mode: "single",
					theme_preference: "dark-protan-deuter",
				}),
			),
		).toEqual({
			mode: "single",
			theme: "dark-protan-deuter",
		});
	});

	it("falls back to the OS-default light when theme_light is invalid in sync mode", () => {
		expect(
			migrateLegacyPreference(
				settings({
					theme_mode: "sync",
					theme_light: "garbage",
					theme_dark: "dark-tritan",
				}),
			),
		).toEqual({
			mode: "sync",
			light: "light",
			dark: "dark-tritan",
		});
	});

	it("falls back to the OS-default dark when theme_dark is invalid in sync mode", () => {
		expect(
			migrateLegacyPreference(
				settings({
					theme_mode: "sync",
					theme_light: "light-tritan",
					theme_dark: "garbage",
				}),
			),
		).toEqual({
			mode: "sync",
			light: "light-tritan",
			dark: "dark",
		});
	});

	it("migrates legacy auto to sync mode on read", () => {
		expect(
			migrateLegacyPreference(settings({ theme_preference: "auto" })),
		).toEqual({ mode: "sync", light: "light", dark: "dark" });
	});

	it("migrates legacy auto-protan-deuter to the tritan pair", () => {
		expect(
			migrateLegacyPreference(
				settings({ theme_preference: "auto-protan-deuter" }),
			),
		).toEqual({
			mode: "sync",
			light: "light-protan-deuter",
			dark: "dark-protan-deuter",
		});
	});

	it("migrates legacy auto-tritan to the tritan pair", () => {
		expect(
			migrateLegacyPreference(settings({ theme_preference: "auto-tritan" })),
		).toEqual({
			mode: "sync",
			light: "light-tritan",
			dark: "dark-tritan",
		});
	});

	it("treats a concrete legacy preference as single mode", () => {
		expect(
			migrateLegacyPreference(settings({ theme_preference: "dark" })),
		).toEqual({ mode: "single", theme: "dark" });
	});

	it("falls back to DEFAULT_THEME for empty or unknown legacy values", () => {
		expect(migrateLegacyPreference(settings({}))).toEqual({
			mode: "single",
			theme: DEFAULT_THEME,
		});
		expect(
			migrateLegacyPreference(settings({ theme_preference: "garbage" })),
		).toEqual({ mode: "single", theme: DEFAULT_THEME });
	});

	it("falls back when theme_mode is unrecognized", () => {
		// Defensive: an old client (or a hand-edited row) could set
		// theme_mode to something we don't support. We should not crash.
		expect(
			migrateLegacyPreference(
				settings({ theme_mode: "wizard", theme_preference: "light" }),
			),
		).toEqual({ mode: "single", theme: "light" });
	});
});

describe("resolveActiveThemeName", () => {
	it("returns the single theme regardless of OS scheme", () => {
		expect(
			resolveActiveThemeName({ mode: "single", theme: "dark-tritan" }, "light"),
		).toBe("dark-tritan");
		expect(
			resolveActiveThemeName({ mode: "single", theme: "dark-tritan" }, "dark"),
		).toBe("dark-tritan");
	});

	it("returns the matching sync slot for the current OS scheme", () => {
		const state = {
			mode: "sync" as const,
			light: "light-protan-deuter" as const,
			dark: "dark-tritan" as const,
		};
		expect(resolveActiveThemeName(state, "light")).toBe("light-protan-deuter");
		expect(resolveActiveThemeName(state, "dark")).toBe("dark-tritan");
	});
});

describe("switchToSingle", () => {
	it("picks the dark slot when the OS scheme is dark", () => {
		expect(
			switchToSingle(
				{ mode: "sync", light: "light-tritan", dark: "dark-tritan" },
				"dark",
			),
		).toEqual({ mode: "single", theme: "dark-tritan" });
	});

	it("picks the light slot when the OS scheme is light", () => {
		expect(
			switchToSingle(
				{ mode: "sync", light: "light-tritan", dark: "dark-tritan" },
				"light",
			),
		).toEqual({ mode: "single", theme: "light-tritan" });
	});

	it("is a no-op when the input is already single", () => {
		expect(
			switchToSingle({ mode: "single", theme: "dark-protan-deuter" }, "light"),
		).toEqual({ mode: "single", theme: "dark-protan-deuter" });
	});
});

describe("switchToSync", () => {
	it("pairs a dark-tritan single theme with its light counterpart", () => {
		expect(switchToSync({ mode: "single", theme: "dark-tritan" })).toEqual({
			mode: "sync",
			light: "light-tritan",
			dark: "dark-tritan",
		});
	});

	it("pairs a light-protan-deuter single theme with its dark counterpart", () => {
		expect(
			switchToSync({ mode: "single", theme: "light-protan-deuter" }),
		).toEqual({
			mode: "sync",
			light: "light-protan-deuter",
			dark: "dark-protan-deuter",
		});
	});

	it("pairs a plain dark with the plain light", () => {
		expect(switchToSync({ mode: "single", theme: "dark" })).toEqual({
			mode: "sync",
			light: "light",
			dark: "dark",
		});
	});

	it("is a no-op when the input is already sync", () => {
		const state = {
			mode: "sync" as const,
			light: "light" as const,
			dark: "dark-tritan" as const,
		};
		expect(switchToSync(state)).toEqual(state);
	});
});

describe("stateToUpdate", () => {
	// The form's internal draft always carries all four values (mode +
	// single theme + light slot + dark slot). stateToUpdate is a flat
	// mapping to the API request shape with no computation: switching
	// mode on the form does not erase the "other" slots.
	it("encodes a sync draft straight through", () => {
		expect(
			stateToUpdate(
				{
					mode: "sync",
					single: "dark",
					light: "light-protan-deuter",
					dark: "dark-tritan",
				},
				"fira-code",
			),
		).toEqual({
			// When mode=sync we use the dark slot as the legacy mirror.
			// Old clients that still read theme_preference will see a
			// plausible theme rather than an unrelated single-mode pick.
			theme_preference: "dark-tritan",
			theme_mode: "sync",
			theme_light: "light-protan-deuter",
			theme_dark: "dark-tritan",
			terminal_font: "fira-code",
		});
	});

	it("encodes a single draft so the legacy field mirrors the single pick", () => {
		expect(
			stateToUpdate(
				{
					mode: "single",
					single: "dark-protan-deuter",
					light: "light-tritan",
					dark: "dark-tritan",
				},
				"",
			),
		).toEqual({
			theme_preference: "dark-protan-deuter",
			theme_mode: "single",
			theme_light: "light-tritan",
			theme_dark: "dark-tritan",
			terminal_font: "",
		});
	});
});
