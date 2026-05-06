import type * as monaco from "monaco-editor";
import muiTheme from "./mui";

// Aurora syntax palette. We pick teal for types (matches the active
// accent), fuchsia for delimiters (matches preview), violet-light for
// identifiers, and rose for strings so each token category lights up
// in a different hue against the indigo editor background.
export default {
	base: "vs-dark",
	inherit: true,
	rules: [
		{
			token: "comment",
			foreground: "6E6EA8",
		},
		{
			token: "type",
			foreground: "5EEAD4",
		},
		{
			token: "string",
			foreground: "FDA4AF",
		},
		{
			token: "variable",
			foreground: "E9D5FF",
		},
		{
			token: "identifier",
			foreground: "C4B5FD",
		},
		{
			token: "delimiter.curly",
			foreground: "F0ABFC",
		},
	],
	colors: {
		"editor.foreground": muiTheme.palette.text.primary,
		"editor.background": muiTheme.palette.background.paper,
	},
} satisfies monaco.editor.IStandaloneThemeData as monaco.editor.IStandaloneThemeData;
