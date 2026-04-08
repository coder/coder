import type * as monaco from "monaco-editor";
import muiTheme from "./mui";

export default {
	base: "vs-dark",
	inherit: true,
	rules: [
		{
			token: "comment",
			foreground: "7B6F9C",
		},
		{
			token: "type",
			foreground: "C4B5FD",
		},
		{
			token: "string",
			foreground: "D8B4FE",
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
			foreground: "A78BFA",
		},
	],
	colors: {
		"editor.foreground": muiTheme.palette.text.primary,
		"editor.background": muiTheme.palette.background.paper,
	},
} satisfies monaco.editor.IStandaloneThemeData as monaco.editor.IStandaloneThemeData;
