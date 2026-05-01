import themes, {
	baseModeFor,
	CONCRETE_THEMES,
	isConcreteThemeName,
	legacyAutoToSync,
	resolveThemeName,
} from ".";

describe("resolveThemeName", () => {
	it("returns the stored preference as-is for concrete themes", () => {
		expect(resolveThemeName("dark", "light")).toBe("dark");
		expect(resolveThemeName("light", "dark")).toBe("light");
		expect(resolveThemeName("dark-protan-deuter", "light")).toBe(
			"dark-protan-deuter",
		);
		expect(resolveThemeName("light-protan-deuter", "dark")).toBe(
			"light-protan-deuter",
		);
		expect(resolveThemeName("dark-tritan", "light")).toBe("dark-tritan");
		expect(resolveThemeName("light-tritan", "dark")).toBe("light-tritan");
	});

	it("resolves auto to the OS preference", () => {
		expect(resolveThemeName("auto", "dark")).toBe("dark");
		expect(resolveThemeName("auto", "light")).toBe("light");
	});

	it("falls back to the OS scheme for unknown values", () => {
		// Empty string is persisted when the user has never set a preference,
		// so it must resolve to the OS scheme rather than erroring.
		expect(resolveThemeName("", "dark")).toBe("dark");
		expect(resolveThemeName("", "light")).toBe("light");
		expect(resolveThemeName(undefined, "dark")).toBe("dark");
		// Legacy value from an earlier cleanup migration (000260) must still
		// resolve safely.
		expect(resolveThemeName("darkBlue", "light")).toBe("light");
		expect(resolveThemeName("garbage", "dark")).toBe("dark");
	});
});

describe("theme registry", () => {
	it("contains every concrete theme name", () => {
		for (const name of CONCRETE_THEMES) {
			expect(themes).toHaveProperty(name);
		}
	});

	it("exports exactly the themes registered in CONCRETE_THEMES", () => {
		expect(new Set(Object.keys(themes))).toEqual(new Set(CONCRETE_THEMES));
	});

	it("always resolves to a theme that exists in the registry", () => {
		const preferences: (string | undefined)[] = [
			undefined,
			"",
			"auto",
			...CONCRETE_THEMES,
		];
		for (const pref of preferences) {
			for (const scheme of ["dark", "light"] as const) {
				const resolved = resolveThemeName(pref, scheme);
				expect(themes[resolved]).toBeDefined();
			}
		}
	});
});

describe("isConcreteThemeName", () => {
	it("returns true for every concrete theme name", () => {
		for (const name of CONCRETE_THEMES) {
			expect(isConcreteThemeName(name)).toBe(true);
		}
	});

	it("rejects legacy auto-family preferences", () => {
		// Embeds and any other caller validating a concrete theme must
		// reject the auto-family strings. They no longer carry meaning as
		// concrete themes.
		expect(isConcreteThemeName("auto")).toBe(false);
		expect(isConcreteThemeName("auto-protan-deuter")).toBe(false);
		expect(isConcreteThemeName("auto-tritan")).toBe(false);
	});

	it("rejects non-string and empty values", () => {
		expect(isConcreteThemeName("")).toBe(false);
		expect(isConcreteThemeName(undefined)).toBe(false);
		expect(isConcreteThemeName(null)).toBe(false);
		expect(isConcreteThemeName(42)).toBe(false);
		expect(isConcreteThemeName({})).toBe(false);
	});
});

describe("legacyAutoToSync", () => {
	it("maps each legacy auto value to its sync pair", () => {
		expect(legacyAutoToSync("auto")).toEqual({
			mode: "sync",
			light: "light",
			dark: "dark",
		});
		expect(legacyAutoToSync("auto-protan-deuter")).toEqual({
			mode: "sync",
			light: "light-protan-deuter",
			dark: "dark-protan-deuter",
		});
		expect(legacyAutoToSync("auto-tritan")).toEqual({
			mode: "sync",
			light: "light-tritan",
			dark: "dark-tritan",
		});
	});

	it("returns null for concrete theme names and unrelated values", () => {
		expect(legacyAutoToSync("dark")).toBeNull();
		expect(legacyAutoToSync("dark-tritan")).toBeNull();
		expect(legacyAutoToSync("")).toBeNull();
		expect(legacyAutoToSync(undefined)).toBeNull();
		expect(legacyAutoToSync("garbage")).toBeNull();
	});
});

describe("baseModeFor", () => {
	// ThemeProvider applies both the concrete theme class and the base
	// mode class to `<html>` so Tailwind's `dark:` variant keeps matching
	// on colorblind variants. Assert the mapping for every concrete
	// theme so a new variant whose name does not start with `dark` or
	// `light` is caught by this test instead of silently regressing the
	// UI.
	it("maps every concrete theme to its base mode", () => {
		for (const name of CONCRETE_THEMES) {
			const expected = name.startsWith("dark") ? "dark" : "light";
			expect(baseModeFor(name)).toBe(expected);
		}
	});

	it("returns the expected mode for the documented concrete names", () => {
		expect(baseModeFor("dark")).toBe("dark");
		expect(baseModeFor("dark-protan-deuter")).toBe("dark");
		expect(baseModeFor("dark-tritan")).toBe("dark");
		expect(baseModeFor("light")).toBe("light");
		expect(baseModeFor("light-protan-deuter")).toBe("light");
		expect(baseModeFor("light-tritan")).toBe("light");
	});
});

describe("colorblind role palettes", () => {
	it("keeps protan-deuter error distinct from danger", () => {
		expect(themes["light-protan-deuter"].roles.error).not.toEqual(
			themes["light-protan-deuter"].roles.danger,
		);
		expect(themes["dark-protan-deuter"].roles.error).not.toEqual(
			themes["dark-protan-deuter"].roles.danger,
		);
	});

	it("keeps tritan danger on the base orange role", () => {
		expect(themes["light-tritan"].roles.danger).toEqual(
			themes.light.roles.danger,
		);
		expect(themes["dark-tritan"].roles.danger).toEqual(
			themes.dark.roles.danger,
		);
	});
});
