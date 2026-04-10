import type * as monaco from "monaco-editor";
import muiTheme from "./mui";

export default {
	base: "vs-dark",
	inherit: true,
	rules: [
		{
			token: "comment",
			foreground: "6B737C",
		},
		{
			token: "type",
			foreground: "C4B5FD",
		},
		{
			token: "string",
			foreground: "A78BFA",
		},
		{
			token: "variable",
			foreground: "DDDDDD",
		},
		{
			token: "identifier",
			foreground: "C4B5FD",
		},
		{
			token: "delimiter.curly",
			foreground: "EBB325",
		},
	],
	colors: {
		"editor.foreground": muiTheme.palette.text.primary,
		"editor.background": muiTheme.palette.background.paper,
	},
} satisfies monaco.editor.IStandaloneThemeData as monaco.editor.IStandaloneThemeData;
