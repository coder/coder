import { Theme } from "@mui/material/styles"

declare module "@mui/material/styles" {
  interface TypeBackground {
    paperLight: string
  }
}

declare module "@mui/styles/defaultTheme" {
  interface DefaultTheme extends Theme {}
}
