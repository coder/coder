import type * as monaco from "monaco-editor";
import tw from "../tailwindColors";
import palette from "./palette";

export default {
	base: "vs",
	inherit: true,
	rules: [
		{
			token: "comment",
			foreground: "6B737C",
		},
		{
			token: "type",
			foreground: "682CD7",
		},
		{
			token: "string",
			foreground: "1766B4",
		},
		{
			token: "variable",
			foreground: "444444",
		},
		{
			token: "identifier",
			foreground: "682CD7",
		},
		{
			token: "delimiter.curly",
			foreground: "EBB325",
		},
	],
	colors: {
		"editor.foreground": palette.text.primary,
		"editor.background": palette.background.paper,
		// Matches --surface-invert-secondary so editor scrollbars meet 3:1 contrast.
		"scrollbarSlider.background": tw.zinc[700],
		"scrollbarSlider.hoverBackground": tw.zinc[800],
		"scrollbarSlider.activeBackground": tw.zinc[800],
	},
} satisfies monaco.editor.IStandaloneThemeData as monaco.editor.IStandaloneThemeData;
