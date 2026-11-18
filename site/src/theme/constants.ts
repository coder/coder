import type { TerminalFontName } from "#/api/typesGenerated";

const MONOSPACE_DEFAULT_FONT = "Geist Mono Variable";
const TERMINAL_SYMBOL_FONT = "'Terminal Symbols'";
export const MONOSPACE_FONT_FAMILY =
	"'Geist Mono Variable', 'IBM Plex Mono', 'Lucida Console', 'Lucida Sans Typewriter', 'Liberation Mono', 'Monaco', 'Courier New', Courier, monospace";

const withTerminalSymbolFallback = (fontFamily: string) =>
	fontFamily.replace(", monospace", `, ${TERMINAL_SYMBOL_FONT}, monospace`);

export const terminalFonts: Record<TerminalFontName, string> = {
	"fira-code": withTerminalSymbolFallback(
		MONOSPACE_FONT_FAMILY.replace(MONOSPACE_DEFAULT_FONT, "Fira Code"),
	),
	"jetbrains-mono": withTerminalSymbolFallback(
		MONOSPACE_FONT_FAMILY.replace(MONOSPACE_DEFAULT_FONT, "JetBrains Mono"),
	),
	"source-code-pro": withTerminalSymbolFallback(
		MONOSPACE_FONT_FAMILY.replace(MONOSPACE_DEFAULT_FONT, "Source Code Pro"),
	),
	"ibm-plex-mono": withTerminalSymbolFallback(
		MONOSPACE_FONT_FAMILY.replace(MONOSPACE_DEFAULT_FONT, "IBM Plex Mono"),
	),
	"geist-mono": withTerminalSymbolFallback(MONOSPACE_FONT_FAMILY),

	"": withTerminalSymbolFallback(MONOSPACE_FONT_FAMILY),
};
export const terminalFontLabels: Record<TerminalFontName, string> = {
	"geist-mono": "Geist Mono",
	"fira-code": "Fira Code",
	"jetbrains-mono": "JetBrains Mono",
	"source-code-pro": "Source Code Pro",
	"ibm-plex-mono": "IBM Plex Mono",
	"": "", // needed for enum completeness, otherwise fails the build
};
export const DEFAULT_TERMINAL_FONT = "geist-mono";
export const DEFAULT_TERMINAL_FONT_SIZE = 16;
export const terminalFontSizes = [
	10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 22, 24, 28, 32,
];

export const navHeight = 62;
export const containerWidth = 1380;
export const containerWidthMedium = 1080;
export const sidePadding = 24;
