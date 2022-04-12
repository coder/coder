import ThemeProvider from "@material-ui/styles/ThemeProvider"
import { render as wrappedRender, RenderResult } from "@testing-library/react"
import { createMemoryHistory } from "history"
import React from "react"
import { unstable_HistoryRouter as HistoryRouter } from "react-router-dom"
import { dark } from "../theme"
import { XServiceProvider } from "../xServices/StateContext"

export const history = createMemoryHistory()

export const WrapperComponent: React.FC = ({ children }) => {
  return (
    <HistoryRouter history={history}>
      <XServiceProvider>
        <ThemeProvider theme={dark}>{children}</ThemeProvider>
      </XServiceProvider>
    </HistoryRouter>
  )
}

export const render = (component: React.ReactElement): RenderResult => {
  return wrappedRender(<WrapperComponent>{component}</WrapperComponent>)
}

export * from "./entities"
