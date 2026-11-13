import {
	registerCustomTheme,
	resolveTheme,
	type ThemeRegistration,
	type ThemesType,
} from "@pierre/diffs";
import type * as monaco from "monaco-editor";
import { getMonacoTheme } from "#/theme/monaco";

// Monarch token names used by our Monaco themes, mapped to the TextMate scopes
// that Shiki grammars emit for the same constructs.
const scopesByMonacoToken: Record<string, string[]> = {
	comment: ["comment"],
	string: ["string"],
	variable: ["variable"],
	type: [
		"entity.name.type",
		"storage.type",
		"support.type",
		"support.function",
	],
	identifier: ["entity.name.type"],
	"delimiter.curly": [
		"punctuation.section.block",
		"punctuation.section.braces",
		"punctuation.definition.block",
	],
};

// Shiki's equivalents of Monaco's built-in base themes, which our Monaco
// themes inherit from.
const shikiBaseByMonacoBase: Record<monaco.editor.BuiltinTheme, string> = {
	vs: "light-plus",
	"vs-dark": "dark-plus",
	"hc-black": "dark-plus",
	"hc-light": "light-plus",
};

export const toShikiTheme = async (
	name: string,
	monacoTheme: monaco.editor.IStandaloneThemeData,
): Promise<ThemeRegistration> => {
	const base = monacoTheme.inherit
		? await resolveTheme(shikiBaseByMonacoBase[monacoTheme.base])
		: undefined;

	const tokenColors = monacoTheme.rules.flatMap((rule) => {
		const scope = scopesByMonacoToken[rule.token];
		if (!scope) {
			return [];
		}
		return [
			{
				scope,
				settings: {
					foreground: rule.foreground && `#${rule.foreground}`,
					background: rule.background && `#${rule.background}`,
					fontStyle: rule.fontStyle,
				},
			},
		];
	});

	const colors = { ...base?.colors, ...monacoTheme.colors };

	return {
		name,
		type:
			monacoTheme.base === "vs" || monacoTheme.base === "hc-light"
				? "light"
				: "dark",
		colors,
		// Shiki prefers these over `colors`, and would otherwise take them from
		// the base theme's global settings.
		fg: colors["editor.foreground"] ?? base?.fg,
		bg: colors["editor.background"] ?? base?.bg,
		// Later rules win over earlier rules with equally specific scopes.
		tokenColors: [
			...(base?.settings.filter((setting) => setting.scope) ?? []),
			...tokenColors,
		],
	};
};

/**
 * Shiki themes derived from the dashboard's Monaco themes, registered for use
 * with `@pierre/diffs`.
 */
export const coderShikiThemes = {
	dark: "coder-dark",
	light: "coder-light",
} as const satisfies ThemesType;

for (const mode of ["dark", "light"] as const) {
	const name = coderShikiThemes[mode];
	registerCustomTheme(name, () => toShikiTheme(name, getMonacoTheme(mode)));
}
