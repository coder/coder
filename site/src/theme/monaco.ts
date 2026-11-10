import type * as monaco from "monaco-editor";
import { readThemeColors } from "./cssColors";

/** Builds Monaco's hex color configuration from the Tailwind theme's CSS tokens. */
export function getMonacoTheme(
	mode: "dark" | "light",
): monaco.editor.IStandaloneThemeData {
	const tokens = [
		"foreground",
		"background",
		"comment",
		"type",
		"string",
		"variable",
		"brace",
	];
	const colors = readThemeColors(
		mode,
		tokens.map((token) => `--editor-${token}`),
		"hex",
	);
	const color = (token: string) => colors[`--editor-${token}`];
	return {
		base: mode === "dark" ? "vs-dark" : "vs",
		inherit: true,
		rules: [
			{ token: "comment", foreground: color("comment").replace("#", "") },
			{ token: "type", foreground: color("type").replace("#", "") },
			{ token: "string", foreground: color("string").replace("#", "") },
			{ token: "variable", foreground: color("variable").replace("#", "") },
			{ token: "identifier", foreground: color("type").replace("#", "") },
			{ token: "delimiter.curly", foreground: color("brace").replace("#", "") },
		],
		colors: {
			"editor.foreground": color("foreground"),
			"editor.background": color("background"),
		},
	};
}
