import ThemeProvider from "@material-ui/styles/ThemeProvider"
import { render as wrappedRender, RenderResult } from "@testing-library/react"
import { createMemoryHistory } from "history"
import { FC, ReactElement } from "react"
import {
  MemoryRouter,
  Route,
  Routes,
  unstable_HistoryRouter as HistoryRouter,
} from "react-router-dom"
import { RequireAuth } from "../components/RequireAuth/RequireAuth"
import { dark } from "../theme"
import { XServiceProvider } from "../xServices/StateContext"
import { MockUser } from "./entities"

export const history = createMemoryHistory()

export const WrapperComponent: FC<React.PropsWithChildren<unknown>> = ({ children }) => {
  return (
    <HistoryRouter history={history}>
      <XServiceProvider>
        <ThemeProvider theme={dark}>{children}</ThemeProvider>
      </XServiceProvider>
    </HistoryRouter>
  )
}

export const render = (component: ReactElement): RenderResult => {
  return wrappedRender(<WrapperComponent>{component}</WrapperComponent>)
}

type RenderWithAuthResult = RenderResult & { user: typeof MockUser }

/**
 *
 * @param ui The component to render and test
 * @param options Can contain `route`, the URL to use, such as /users/user1, and `path`,
 * such as /users/:userid. When there are no parameters, they are the same and you can just supply `route`.
 */
export function renderWithAuth(
  ui: JSX.Element,
  { route = "/", path }: { route?: string; path?: string } = {},
): RenderWithAuthResult {
  const renderResult = wrappedRender(
    <MemoryRouter initialEntries={[route]}>
      <XServiceProvider>
        <ThemeProvider theme={dark}>
          <Routes>
            <Route path={path ?? route} element={<RequireAuth>{ui}</RequireAuth>} />
          </Routes>
        </ThemeProvider>
      </XServiceProvider>
    </MemoryRouter>,
  )

  return {
    user: MockUser,
    ...renderResult,
  }
}

export * from "./entities"
