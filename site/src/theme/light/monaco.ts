import type * as monaco from "monaco-editor";
import muiTheme from "./mui";

export default {
	base: "vs",
	inherit: true,
	rules: [
		{
			token: "comment",
			foreground: "5A6B73", // Darker gray for better readability on light background
		},
		{
			token: "type",
			foreground: "5B2C87", // Darker purple for better contrast
		},
		{
			token: "string",
			foreground: "0D7377", // Dark teal for strings
		},
		{
			token: "variable",
			foreground: "2D2D2D", // Darker gray for variables
		},
		{
			token: "identifier",
			foreground: "1565C0", // Dark blue for identifiers
		},
		{
			token: "delimiter.curly",
			foreground: "E65100", // Dark orange for delimiters
		},
		{
			token: "keyword",
			foreground: "C2185B", // Dark pink for keywords
		},
		{
			token: "number",
			foreground: "D84315", // Dark orange-red for numbers
		},
		{
			token: "operator",
			foreground: "0277BD", // Dark blue for operators
		},
	],
	colors: {
		"editor.foreground": muiTheme.palette.text.primary,
		"editor.background": muiTheme.palette.background.paper,
	},
} satisfies monaco.editor.IStandaloneThemeData as monaco.editor.IStandaloneThemeData;
