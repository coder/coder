import type { TerminalFontName } from "api/typesGenerated";

export const borderRadius = 8;
export const MONOSPACE_FONT_FAMILY =
	"'IBM Plex Mono', 'Lucida Console', 'Lucida Sans Typewriter', 'Liberation Mono', 'Monaco', 'Courier New', Courier, monospace";
export const terminalFonts: Record<TerminalFontName, string> = {
	"fira-code": MONOSPACE_FONT_FAMILY.replace("IBM Plex Mono", "Fira Code"),
	"ibm-plex-mono": MONOSPACE_FONT_FAMILY,

	"": MONOSPACE_FONT_FAMILY,
};

export const BODY_FONT_FAMILY = `"Inter Variable", system-ui, sans-serif`;
export const navHeight = 62;
export const containerWidth = 1380;
export const containerWidthMedium = 1080;
export const sidePadding = 24;
export const dashboardContentBottomPadding = 8 * 6;

// MUI does not have aligned heights for buttons and inputs so we have to "hack" it a little bit
export const BUTTON_XL_HEIGHT = 44;
export const BUTTON_LG_HEIGHT = 40;
export const BUTTON_MD_HEIGHT = 36;
export const BUTTON_SM_HEIGHT = 32;
