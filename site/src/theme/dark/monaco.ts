import type * as monaco from "monaco-editor";
import muiTheme from "./mui";

export default {
	base: "vs-dark",
	inherit: true,
	rules: [
		{
			token: "comment",
			foreground: "7C8B98", // Brighter gray for better readability
		},
		{
			token: "type",
			foreground: "C792EA", // More vibrant purple
		},
		{
			token: "string",
			foreground: "A8E6CF", // Bright mint green for strings
		},
		{
			token: "variable",
			foreground: "F8F8F2", // Brighter white for variables
		},
		{
			token: "identifier",
			foreground: "82AAFF", // Bright blue for identifiers
		},
		{
			token: "delimiter.curly",
			foreground: "FFD700", // Bright gold for delimiters
		},
		{
			token: "keyword",
			foreground: "FF6B9D", // Pink for keywords
		},
		{
			token: "number",
			foreground: "F78C6C", // Orange for numbers
		},
		{
			token: "operator",
			foreground: "89DDFF", // Light blue for operators
		},
	],
	colors: {
		"editor.foreground": muiTheme.palette.text.primary,
		"editor.background": muiTheme.palette.background.paper,
	},
} satisfies monaco.editor.IStandaloneThemeData as monaco.editor.IStandaloneThemeData;
