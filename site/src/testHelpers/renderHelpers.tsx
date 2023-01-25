import {
  render as wrappedRender,
  RenderResult,
  screen,
  waitForElementToBeRemoved,
} from "@testing-library/react"
import { AppProviders } from "app"
import { DashboardLayout } from "components/Dashboard/DashboardLayout"
import { createMemoryHistory } from "history"
import { i18n } from "i18n"
import { FC, ReactElement } from "react"
import { I18nextProvider } from "react-i18next"
import {
  MemoryRouter,
  Route,
  Routes,
  unstable_HistoryRouter as HistoryRouter,
} from "react-router-dom"
import { RequireAuth } from "../components/RequireAuth/RequireAuth"
import { MockUser } from "./entities"

export const history = createMemoryHistory()

export const WrapperComponent: FC<React.PropsWithChildren<unknown>> = ({
  children,
}) => {
  return (
    <AppProviders>
      <HistoryRouter history={history}>{children}</HistoryRouter>
    </AppProviders>
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
    <I18nextProvider i18n={i18n}>
      <AppProviders>
        <MemoryRouter initialEntries={[route]}>
          <Routes>
            <Route element={<RequireAuth />}>
              <Route element={<DashboardLayout />}>
                <Route path={path ?? route} element={ui} />
              </Route>
            </Route>
            {routes}
          </Routes>
        </MemoryRouter>
      </AppProviders>
    </I18nextProvider>,
  )

  return {
    user: MockUser,
    ...renderResult,
  }
}

export const waitForLoaderToBeRemoved = (): Promise<void> =>
  waitForElementToBeRemoved(() => screen.getByRole("progressbar"))

export * from "./entities"
