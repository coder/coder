import ThemeProvider from "@material-ui/styles/ThemeProvider"
import { render as wrappedRender, RenderResult } from "@testing-library/react"
import { createMemoryHistory } from "history"
import { FC, ReactElement } from "react"
import { unstable_HistoryRouter as HistoryRouter } from "react-router-dom"
import { RequireAuth } from "../components/RequireAuth/RequireAuth"
import { dark } from "../theme"
import { XServiceProvider } from "../xServices/StateContext"
import { MockUser } from "./entities"

export const history = createMemoryHistory()

export const WrapperComponent: FC = ({ children }) => {
  return (
    <HistoryRouter history={history}>
      <XServiceProvider>
        <ThemeProvider theme={dark}>{children}</ThemeProvider>
      </XServiceProvider>
    </HistoryRouter>
  )
}

interface RenderOptions {
  route?: string
}

export const render = (
  component: ReactElement,
  { route = "/" }: RenderOptions = {},
): RenderResult => {
  history.replace(route)
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
  renderOptions?: RenderOptions,
): RenderWithAuthResult {
  const renderResult = render(<RequireAuth>{ui}</RequireAuth>, renderOptions)

  return {
    user: MockUser,
    ...renderResult,
  }
}

export * from "./entities"
