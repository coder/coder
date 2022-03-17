import { render as wrappedRender, RenderResult } from "@testing-library/react"
import React from "react"
import ThemeProvider from "@material-ui/styles/ThemeProvider"

import { dark } from "../theme"
import { createMemoryHistory } from "history"
import { unstable_HistoryRouter as HistoryRouter } from "react-router-dom"

export const history = createMemoryHistory()

export const WrapperComponent: React.FC = ({ children }) => {
  return (
    <HistoryRouter history={history}>
      <ThemeProvider theme={dark}>{children}</ThemeProvider>
    </HistoryRouter>
  )
}

export const render = (component: React.ReactElement): RenderResult => {
  return wrappedRender(<WrapperComponent>{component}</WrapperComponent>)
}

export * from "./mocks"
