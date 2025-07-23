// biome-ignore lint/nursery/noRestrictedImports: createTheme
import { createTheme } from "@mui/material/styles";
import { BODY_FONT_FAMILY, borderRadius } from "../constants";
import { components } from "../mui";
import tw from "../tailwindColors";

const muiTheme = createTheme({
	palette: {
		mode: "dark",
		primary: {
			main: tw.sky[500],
			contrastText: tw.white,
			light: tw.sky[400],
			dark: tw.sky[600],
		},
		secondary: {
			main: tw.zinc[500],
			contrastText: tw.zinc[200],
			dark: tw.zinc[400],
		},
		background: {
			default: "#ff1493", // Deep Pink for dark mode
			paper: "#c71585", // Medium Violet Red for paper in dark mode
		},
		text: {
			primary: "#00FF00", // Bright Green
			secondary: "#7CFC00", // Lawn Green for secondary text
			disabled: "#90EE90", // Light Green for disabled text
		},
		divider: tw.zinc[700],
		warning: {
			light: tw.amber[500],
			main: tw.amber[800],
			dark: tw.amber[950],
		},
		success: {
			main: tw.green[500],
			dark: tw.green[600],
		},
		info: {
			light: tw.blue[400],
			main: tw.blue[600],
			dark: tw.blue[950],
			contrastText: tw.zinc[200],
		},
		error: {
			light: tw.red[400],
			main: tw.red[500],
			dark: tw.red[950],
			contrastText: tw.zinc[200],
		},
		action: {
			hover: tw.zinc[800],
		},
		neutral: {
			main: tw.zinc[50],
		},
		dots: tw.zinc[500],
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
