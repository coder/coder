import ThemeProvider from "@material-ui/styles/ThemeProvider"
import {
  render as wrappedRender,
  RenderResult,
  screen,
  waitForElementToBeRemoved,
} from "@testing-library/react"
import { createMemoryHistory } from "history"
import { i18n } from "i18n"
import { FC, ReactElement } from "react"
import { HelmetProvider } from "react-helmet-async"
import { I18nextProvider } from "react-i18next"
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

export const WrapperComponent: FC<React.PropsWithChildren<unknown>> = ({
  children,
}) => {
  return (
    <HelmetProvider>
      <HistoryRouter history={history}>
        <XServiceProvider>
          <ThemeProvider theme={dark}>{children}</ThemeProvider>
        </XServiceProvider>
      </HistoryRouter>
    </HelmetProvider>
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
  {
    route = "/",
    path,
    routes,
  }: { route?: string; path?: string; routes?: JSX.Element } = {},
): RenderWithAuthResult {
  const renderResult = wrappedRender(
    <HelmetProvider>
      <MemoryRouter initialEntries={[route]}>
        <XServiceProvider>
          <I18nextProvider i18n={i18n}>
            <ThemeProvider theme={dark}>
              <Routes>
                <Route element={<RequireAuth />}>
                  <Route path={path ?? route} element={ui} />
                </Route>
                {routes}
              </Routes>
            </ThemeProvider>
          </I18nextProvider>
        </XServiceProvider>
      </MemoryRouter>
    </HelmetProvider>,
  )

  return {
    user: MockUser,
    ...renderResult,
  }
}

export const waitForLoaderToBeRemoved = (): Promise<void> =>
  waitForElementToBeRemoved(() => screen.getByRole("progressbar"))

export * from "./entities"
