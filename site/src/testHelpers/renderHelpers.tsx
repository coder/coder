import {
  render as tlRender,
  screen,
  waitFor,
  renderHook,
} from "@testing-library/react";
import { type ReactNode, useState } from "react";
import { QueryClient } from "react-query";
import { AppProviders } from "App";
import { ThemeProvider } from "contexts/ThemeProvider";
import { DashboardLayout } from "components/Dashboard/DashboardLayout";
import { RequireAuth } from "components/RequireAuth/RequireAuth";
import { TemplateSettingsLayout } from "pages/TemplateSettingsPage/TemplateSettingsLayout";
import { WorkspaceSettingsLayout } from "pages/WorkspaceSettingsPage/WorkspaceSettingsLayout";
import {
  createMemoryRouter,
  RouterProvider,
  type RouteObject,
} from "react-router-dom";
import { MockUser } from "./entities";

function createTestQueryClient() {
  // Helps create one query client for each test case, to make sure that tests
  // are isolated and can't affect each other
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        cacheTime: 0,
        refetchOnWindowFocus: false,
        networkMode: "offlineFirst",
      },
    },
  });
}

export const renderWithRouter = (
  router: ReturnType<typeof createMemoryRouter>,
) => {
  const queryClient = createTestQueryClient();

  return {
    ...tlRender(
      <AppProviders queryClient={queryClient}>
        <RouterProvider router={router} />
      </AppProviders>,
    ),
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

export type RenderWithAuthOptions = {
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

type RenderHookWithAuthOptions<Props> = Partial<
  Readonly<
    Omit<RenderWithAuthOptions, "children"> & {
      initialProps: Props;
    }
  >
>;

/**
 * Custom version of renderHook that is aware of all our App providers.
 *
 * Had to do some nasty, cursed things in the implementation to make sure that
 * the tests using this function remained simple.
 *
 * @see {@link https://github.com/coder/coder/pull/10362#discussion_r1380852725}
 */
export async function renderHookWithAuth<Result, Props>(
  render: (initialProps: Props) => Result,
  options: RenderHookWithAuthOptions<Props> = {},
) {
  const { initialProps, path = "/", route = "/", extraRoutes = [] } = options;
  const queryClient = createTestQueryClient();

  // Easy to miss – there's an evil definite assignment via the !
  let escapedRouter!: ReturnType<typeof createMemoryRouter>;

  const { result, rerender, unmount } = renderHook(render, {
    initialProps,
    wrapper: ({ children }) => {
      /**
       * Unfortunately, there isn't a way to define the router outside the
       * wrapper while keeping it aware of children, meaning that we need to
       * define the router as readonly state in the component instance. This
       * ensures the value remains stable across all re-renders
       */
      // eslint-disable-next-line react-hooks/rules-of-hooks -- This is actually processed as a component; the linter just isn't aware of that
      const [readonlyStatefulRouter] = useState(() => {
        return createMemoryRouter(
          [{ path, element: <>{children}</> }, ...extraRoutes],
          { initialEntries: [route] },
        );
      });

      /**
       * Leaks the wrapper component's state outside React's render cycles.
       */
      escapedRouter = readonlyStatefulRouter;

      return (
        <AppProviders queryClient={queryClient}>
          <RouterProvider router={readonlyStatefulRouter} />
        </AppProviders>
      );
    },
  });

  /**
   * This is necessary to get around some providers in AppProviders having
   * conditional rendering and not always rendering their children immediately.
   *
   * The hook result won't actually exist until the children defined via wrapper
   * render in full.
   */
  await waitFor(() => expect(result.current).not.toBe(null));

  return {
    result,
    rerender,
    unmount,
    router: escapedRouter,
  } as const;
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

export const waitForLoaderToBeRemoved = async (): Promise<void> => {
  return waitFor(
    () => {
      expect(screen.queryByTestId("loader")).not.toBeInTheDocument();
    },
    {
      timeout: 5_000,
    },
  );
};

export const renderComponent = (component: React.ReactElement) => {
  return tlRender(component, {
    wrapper: ({ children }) => <ThemeProvider>{children}</ThemeProvider>,
  });
};
