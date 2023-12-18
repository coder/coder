import muiTheme from "./mui";
import type * as monaco from "monaco-editor";

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
    "editor.foreground": muiTheme.palette.text.primary,
    "editor.background": muiTheme.palette.background.paper,
  },
} satisfies monaco.editor.IStandaloneThemeData as monaco.editor.IStandaloneThemeData;
