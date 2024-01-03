import muiTheme from "./mui";
import type * as monaco from "monaco-editor";

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
    "editor.foreground": muiTheme.palette.text.primary,
    "editor.background": muiTheme.palette.background.paper,
  },
} satisfies monaco.editor.IStandaloneThemeData as monaco.editor.IStandaloneThemeData;
