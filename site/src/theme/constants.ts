import type { TerminalFontName } from "#/api/typesGenerated";

export const borderRadius = 8;

const MONOSPACE_DEFAULT_FONT = "Geist Mono Variable";
const TERMINAL_SYMBOL_FONT = "'Coder Terminal Symbols'";
export const MONOSPACE_FONT_FAMILY =
	"'Geist Mono Variable', 'IBM Plex Mono', 'Lucida Console', 'Lucida Sans Typewriter', 'Liberation Mono', 'Monaco', 'Courier New', Courier, monospace";
export const BODY_FONT_FAMILY = `"Geist Variable", system-ui, sans-serif`;

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

export const navHeight = 62;
export const containerWidth = 1380;
export const containerWidthMedium = 1080;
export const sidePadding = 24;

// MUI does not have aligned heights for buttons and inputs so we have to "hack" it a little bit
export const BUTTON_LG_HEIGHT = 40;
export const BUTTON_MD_HEIGHT = 36;
