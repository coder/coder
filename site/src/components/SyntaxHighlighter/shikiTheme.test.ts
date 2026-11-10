import type * as monaco from "monaco-editor";
import { toShikiTheme } from "./shikiTheme";

const monacoTheme = {
	base: "vs-dark",
	inherit: true,
	rules: [
		{ token: "comment", foreground: "6b737c" },
		{ token: "delimiter.curly", foreground: "ebb325" },
	],
	colors: {
		"editor.foreground": "#ffffff",
		"editor.background": "#000000",
	},
} satisfies monaco.editor.IStandaloneThemeData;

describe("toShikiTheme", () => {
	it("translates Monaco token rules into TextMate scopes", async () => {
		const shikiTheme = await toShikiTheme("test", monacoTheme);

		expect(shikiTheme.type).toBe("dark");
		expect(shikiTheme.bg).toBe("#000000");
		expect(shikiTheme.fg).toBe("#ffffff");
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
		const shikiTheme = await toShikiTheme("test", {
			...monacoTheme,
			base: "vs",
		});

		expect(shikiTheme.type).toBe("light");
		expect(shikiTheme.tokenColors?.length).toBeGreaterThan(
			monacoTheme.rules.length,
		);
	});
});
