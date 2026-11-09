import { installThemeStyles } from "#/testHelpers/themeStyles";
import { getMonacoTheme } from "#/theme/monaco";
import { toShikiTheme } from "./shikiTheme";

describe("toShikiTheme", () => {
	beforeAll(installThemeStyles);
	beforeAll(async () => {
		vi.stubGlobal("jest", vi);
		await import("jest-canvas-mock");
	});
	it("translates Monaco token rules into TextMate scopes", async () => {
		const shikiTheme = await toShikiTheme("test", getMonacoTheme("dark"));

		expect(shikiTheme.type).toBe("dark");
		expect(shikiTheme.bg).toBe(
			getMonacoTheme("dark").colors["editor.background"],
		);
		expect(shikiTheme.fg).toBe(
			getMonacoTheme("dark").colors["editor.foreground"],
		);
		// The last rule matching a scope wins, so our rules must come after the
		// inherited base theme's rules.
		expect(shikiTheme.tokenColors?.at(-1)).toEqual({
			scope: [
				"punctuation.section.block",
				"punctuation.section.braces",
				"punctuation.definition.block",
			],
			settings: { foreground: "#ebb325" },
		});
		expect(shikiTheme.tokenColors).toContainEqual({
			scope: ["comment"],
			settings: { foreground: "#6b737c" },
		});
	});

	it("inherits from the matching Shiki base theme", async () => {
		const shikiTheme = await toShikiTheme("test", getMonacoTheme("light"));

		expect(shikiTheme.type).toBe("light");
		expect(shikiTheme.tokenColors?.length).toBeGreaterThan(
			getMonacoTheme("light").rules.length,
		);
	});
});
