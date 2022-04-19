import ThemeProvider from "@material-ui/styles/ThemeProvider"
import { render as wrappedRender, RenderResult } from "@testing-library/react"
import { createMemoryHistory } from "history"
import React from "react"
import { MemoryRouter, Route, Routes, unstable_HistoryRouter as HistoryRouter } from "react-router-dom"
import { RequireAuth } from "../components/RequireAuth/RequireAuth"
import { dark } from "../theme"
import { XServiceProvider } from "../xServices/StateContext"
import { MockUser } from "./entities"

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

type RenderWithAuthResult = RenderResult & { user: typeof MockUser }

export function renderWithAuth(ui: JSX.Element, { route = "/" }: { route?: string } = {}): RenderWithAuthResult {
  const renderResult = wrappedRender(
    <MemoryRouter initialEntries={[route]}>
      <XServiceProvider>
        <Routes>
          <Route path={route} element={<RequireAuth>{ui}</RequireAuth>} />
        </Routes>
      </XServiceProvider>
    </MemoryRouter>,
  )

  return {
    user: MockUser,
    ...renderResult,
  }
}

export * from "./entities"
