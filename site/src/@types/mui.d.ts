/**
 * @deprecated MUI components and types are deprecated. Migrate to shadcn/ui components and Tailwind CSS.
 * This file extends MUI types for legacy compatibility only.
 */

// biome-ignore lint/style/noRestrictedImports: base theme types
import type { PaletteColor, PaletteColorOptions } from "@mui/material/styles";

/**
 * @deprecated MUI theme module is deprecated. Migrate to Tailwind CSS theme system.
 */
declare module "@mui/material/styles" {
	interface Palette {
		neutral: PaletteColor;
		dots: string;
	}

	interface PaletteOptions {
		neutral?: PaletteColorOptions;
		dots?: string;
	}
}

/**
 * @deprecated MUI Checkbox is deprecated. Use shadcn/ui Checkbox component instead.
 */
declare module "@mui/material/Checkbox" {
	interface CheckboxPropsSizeOverrides {
		xsmall: true;
	}
}
