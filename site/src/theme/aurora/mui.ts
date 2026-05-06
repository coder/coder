/**
 * @deprecated MUI dark theme is deprecated. Migrate to Tailwind CSS theme system.
 * This file provides MUI theme configuration for legacy compatibility only.
 *
 * "Midnight Aurora" is a deep cosmic indigo dark theme with mint-teal aurora
 * accents. The base surfaces sit on the indigo axis instead of zinc, giving
 * the UI a cooler, glow-in-the-dark feel without sacrificing legibility.
 */

/** @deprecated MUI createTheme is deprecated. Migrate to Tailwind CSS theme system. */
import { createTheme } from "@mui/material/styles";
import { BODY_FONT_FAMILY, borderRadius } from "../constants";
import { components } from "../mui";
import tw from "../tailwindColors";

// Background ladder. We use indigo for the deepest surface so the whole
// app reads as "midnight sky", with a subtle slate tint in the upper
// surfaces to keep cards and dialogs readable.
const surfaceBase = "#0d0c27"; // hsl(245 52% 10%)
const surfacePaper = "#191935"; // hsl(244 38% 15%)
const dividerColor = "#2d2956"; // hsl(245 35% 25%)

const muiTheme = createTheme({
	palette: {
		mode: "dark",
		primary: {
			// Aurora teal is the "north-star" accent. It mirrors the
			// shimmer of a real aurora against the indigo backdrop.
			main: tw.teal[300],
			contrastText: tw.indigo[950],
			light: tw.teal[200],
			dark: tw.teal[400],
		},
		secondary: {
			main: tw.violet[400],
			contrastText: tw.violet[50],
			dark: tw.violet[500],
		},
		background: {
			default: surfaceBase,
			paper: surfacePaper,
		},
		text: {
			primary: tw.violet[50],
			secondary: tw.violet[300],
			disabled: tw.indigo[500],
		},
		divider: dividerColor,
		warning: {
			light: tw.amber[300],
			main: tw.amber[700],
			dark: tw.amber[950],
		},
		success: {
			main: tw.teal[400],
			dark: tw.teal[500],
		},
		info: {
			light: tw.cyan[300],
			main: tw.cyan[500],
			dark: tw.cyan[950],
			contrastText: tw.cyan[50],
		},
		error: {
			light: tw.rose[300],
			main: tw.rose[400],
			dark: tw.rose[950],
			contrastText: tw.rose[50],
		},
		action: {
			hover: "#22214a",
		},
		neutral: {
			main: tw.violet[50],
		},
		dots: tw.violet[400],
	},
	typography: {
		fontFamily: BODY_FONT_FAMILY,

		body1: {
			fontSize: "1rem" /* 16px at default scaling */,
			lineHeight: "160%",
		},

		body2: {
			fontSize: "0.875rem" /* 14px at default scaling */,
			lineHeight: "160%",
		},
	},
	shape: {
		borderRadius,
	},
	components,
});

export default muiTheme;
