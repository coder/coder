import {
  render as tlRender,
  screen,
  waitForElementToBeRemoved,
} from "@testing-library/react";
import { AppProviders } from "App";
import { DashboardLayout } from "components/Dashboard/DashboardLayout";
import { i18n } from "i18n";
import { TemplateSettingsLayout } from "pages/TemplateSettingsPage/TemplateSettingsLayout";
import { WorkspaceSettingsLayout } from "pages/WorkspaceSettingsPage/WorkspaceSettingsLayout";
import { I18nextProvider } from "react-i18next";
import {
  RouterProvider,
  createMemoryRouter,
  RouteObject,
} from "react-router-dom";
import { RequireAuth } from "../components/RequireAuth/RequireAuth";
import { MockUser } from "./entities";
import { ReactNode } from "react";

const baseRender = (element: ReactNode) => {
  return tlRender(
    <I18nextProvider i18n={i18n}>
      <AppProviders>{element}</AppProviders>
    </I18nextProvider>,
  );
};

export const renderWithRouter = (
  router: ReturnType<typeof createMemoryRouter>,
) => {
  return {
    ...baseRender(<RouterProvider router={router} />),
    router,
  };
};

export const render = (element: ReactNode) => {
  return renderWithRouter(
    createMemoryRouter(
      [
        {
          path: "/",
          element,
        },
      ],
      { initialEntries: ["/"] },
    ),
  );
};

type RenderWithAuthOptions = {
  // The current URL, /workspaces/123
  route?: string;
  // The route path, /workspaces/:workspaceId
  path?: string;
  // Extra routes to add to the router. It is helpful when having redirecting
  // routes or multiple routes during the test flow
  extraRoutes?: RouteObject[];
  // The same as extraRoutes but for routes that don't require authentication
  nonAuthenticatedRoutes?: RouteObject[];
  // In case you want to render a layout inside of it
  children?: RouteObject["children"];
};

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
  ];

  const renderResult = renderWithRouter(
    createMemoryRouter(routes, { initialEntries: [route] }),
  );

  return {
    user: MockUser,
    ...renderResult,
  };
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
  ];

  const renderResult = renderWithRouter(
    createMemoryRouter(routes, { initialEntries: [route] }),
  );

  return {
    user: MockUser,
    ...renderResult,
  };
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
              children: [{ element, path }, ...extraRoutes],
            },
          ],
        },
      ],
    },
    ...nonAuthenticatedRoutes,
  ];

  const renderResult = renderWithRouter(
    createMemoryRouter(routes, { initialEntries: [route] }),
  );

  return {
    user: MockUser,
    ...renderResult,
  };
}

export const waitForLoaderToBeRemoved = (): Promise<void> =>
  // Sometimes, we have pages that are doing a lot of requests to get done, so the
  // default timeout of 1_000 is not enough. We should revisit this when we unify
  // some of the endpoints
  waitForElementToBeRemoved(() => screen.queryByTestId("loader"), {
    timeout: 5_000,
  });
