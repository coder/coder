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
import { TemplateSettingsLayout } from "pages/TemplateSettingsPage/TemplateSettingsLayout"
import { WorkspaceSettingsLayout } from "pages/WorkspaceSettingsPage/WorkspaceSettingsLayout"
import { FC, ReactElement } from "react"
import { I18nextProvider } from "react-i18next"
import {
  unstable_HistoryRouter as HistoryRouter,
  RouterProvider,
  createMemoryRouter,
  RouteObject,
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

type RenderWithAuthOptions = {
  // The current URL, /workspaces/123
  route?: string
  // The route path, /workspaces/:workspaceId
  path?: string
  // Extra routes to add to the router. It is helpful when having redirecting
  // routes or multiple routes during the test flow
  extraRoutes?: RouteObject[]
  // The same as extraRoutes but for routes that don't require authentication
  nonAuthenticatedRoutes?: RouteObject[]
  // In case you want to render a layout inside of it
  children?: RouteObject["children"]
}

export function renderWithAuth(
  element: JSX.Element,
  {
    path = "/",
    route = "/",
    extraRoutes = [],
    nonAuthenticatedRoutes = [],
    children,
  }: RenderWithAuthOptions = {},
) {
  const routes: RouteObject[] = [
    {
      element: <RequireAuth />,
      children: [{ path, element, children }, ...extraRoutes],
    },
    ...nonAuthenticatedRoutes,
  ]

  const router = createMemoryRouter(routes, { initialEntries: [route] })

  const renderResult = wrappedRender(
    <I18nextProvider i18n={i18n}>
      <AppProviders>
        <RouterProvider router={router} />
      </AppProviders>
    </I18nextProvider>,
  )

  return {
    user: MockUser,
    router,
    ...renderResult,
  }
}

export function renderWithTemplateSettingsLayout(
  element: JSX.Element,
  {
    path = "/",
    route = "/",
    extraRoutes = [],
    nonAuthenticatedRoutes = [],
  }: RenderWithAuthOptions = {},
) {
  const routes: RouteObject[] = [
    {
      element: <RequireAuth />,
      children: [
        {
          element: <DashboardLayout />,
          children: [
            {
              element: <TemplateSettingsLayout />,
              children: [{ path, element }, ...extraRoutes],
            },
          ],
        },
      ],
    },
    ...nonAuthenticatedRoutes,
  ]

  const router = createMemoryRouter(routes, { initialEntries: [route] })

  const renderResult = wrappedRender(
    <I18nextProvider i18n={i18n}>
      <AppProviders>
        <RouterProvider router={router} />
      </AppProviders>
    </I18nextProvider>,
  )

  return {
    user: MockUser,
    router,
    ...renderResult,
  }
}

export function renderWithWorkspaceSettingsLayout(
  element: JSX.Element,
  {
    path = "/",
    route = "/",
    extraRoutes = [],
    nonAuthenticatedRoutes = [],
  }: RenderWithAuthOptions = {},
) {
  const routes: RouteObject[] = [
    {
      element: <RequireAuth />,
      children: [
        {
          element: <DashboardLayout />,
          children: [
            {
              element: <WorkspaceSettingsLayout />,
              children: [{ path, element }, ...extraRoutes],
            },
          ],
        },
      ],
    },
    ...nonAuthenticatedRoutes,
  ]

  const router = createMemoryRouter(routes, { initialEntries: [route] })

  const renderResult = wrappedRender(
    <I18nextProvider i18n={i18n}>
      <AppProviders>
        <RouterProvider router={router} />
      </AppProviders>
    </I18nextProvider>,
  )

  return {
    user: MockUser,
    router,
    ...renderResult,
  }
}

export const waitForLoaderToBeRemoved = (): Promise<void> =>
  // Sometimes, we have pages that are doing a lot of requests to get done, so the
  // default timeout of 1_000 is not enough. We should revisit this when we unify
  // some of the endpoints
  waitForElementToBeRemoved(() => screen.queryByTestId("loader"), {
    timeout: 5_000,
  })
