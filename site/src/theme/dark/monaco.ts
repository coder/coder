import type * as monaco from "monaco-editor";
import tw from "../tailwindColors";
import palette from "./palette";

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
			foreground: "B392F0",
		},
		{
			token: "string",
			foreground: "9DB1C5",
		},
		{
			token: "variable",
			foreground: "DDDDDD",
		},
		{
			token: "identifier",
			foreground: "B392F0",
		},
		{
			token: "delimiter.curly",
			foreground: "EBB325",
		},
	],
	colors: {
		"editor.foreground": palette.text.primary,
		"editor.background": palette.background.paper,
		// Matches --scrollbar-thumb so editor scrollbars meet 3:1 contrast.
		"scrollbarSlider.background": tw.zinc[400],
		"scrollbarSlider.hoverBackground": tw.zinc[300],
		"scrollbarSlider.activeBackground": tw.zinc[300],
	},
} satisfies monaco.editor.IStandaloneThemeData as monaco.editor.IStandaloneThemeData;
