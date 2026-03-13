type ThemePaletteMode = "dark" | "light";

type ThemeWithOptionalPaletteMode = {
	palette?: {
		mode?: unknown;
	};
};

const isThemePaletteMode = (value: unknown): value is ThemePaletteMode => {
	return value === "dark" || value === "light";
};

const detectDocumentPaletteMode = (): ThemePaletteMode => {
	if (typeof document === "undefined") {
		return "dark";
	}
	const rootClassList = document.documentElement.classList;
	if (rootClassList.contains("light")) {
		return "light";
	}
	const bodyClassList = document.body?.classList;
	if (
		bodyClassList?.contains("vscode-light") ||
		bodyClassList?.contains("vscode-high-contrast-light")
	) {
		return "light";
	}
	return "dark";
};

/**
 * Reads a palette mode from an optional Emotion theme.
 *
 * VS Code chat imports ai-elements from a sibling workspace. During that
 * integration, duplicate Emotion copies can leave ai-elements reading an
 * unthemed context. Fall back to document classes so the component keeps
 * rendering instead of crashing on `theme.palette.mode`.
 */
export const getThemePaletteMode = (theme: unknown): ThemePaletteMode => {
	const paletteMode = (theme as ThemeWithOptionalPaletteMode | null)?.palette
		?.mode;
	if (isThemePaletteMode(paletteMode)) {
		return paletteMode;
	}
	return detectDocumentPaletteMode();
};
