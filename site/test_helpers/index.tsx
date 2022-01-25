import { render as wrappedRender, RenderResult } from "@testing-library/react"
import React from "react"
import ThemeProvider from "@material-ui/styles/ThemeProvider"

import { dark } from "../theme"

export const WrapperComponent: React.FC = ({ children }) => {
  return <ThemeProvider theme={dark}>{children}</ThemeProvider>
}

export const render = (component: React.ReactElement): RenderResult => {
  return wrappedRender(<WrapperComponent>{component}</WrapperComponent>)
}

export * from "./user"
