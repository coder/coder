import themes from "#/theme";
import { toShikiTheme } from "./shikiTheme";

describe("toShikiTheme", () => {
	it("translates Monaco token rules into TextMate scopes", async () => {
		const shikiTheme = await toShikiTheme("test", themes.dark.monaco);

		expect(shikiTheme.type).toBe("dark");
		expect(shikiTheme.bg).toBe(themes.dark.monaco.colors["editor.background"]);
		expect(shikiTheme.fg).toBe(themes.dark.monaco.colors["editor.foreground"]);
		// The last rule matching a scope wins, so our rules must come after the
		// inherited base theme's rules.
		expect(shikiTheme.tokenColors?.at(-1)).toEqual({
			scope: [
				"punctuation.section.block",
				"punctuation.section.braces",
				"punctuation.definition.block",
			],
			settings: { foreground: "#EBB325" },
		});
		expect(shikiTheme.tokenColors).toContainEqual({
			scope: ["comment"],
			settings: { foreground: "#6B737C" },
		});
	});

	it("inherits from the matching Shiki base theme", async () => {
		const shikiTheme = await toShikiTheme("test", themes.light.monaco);

		expect(shikiTheme.type).toBe("light");
		expect(shikiTheme.tokenColors?.length).toBeGreaterThan(
			themes.light.monaco.rules.length,
		);
	});
});
