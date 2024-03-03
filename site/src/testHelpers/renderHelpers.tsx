import {
  render as testingLibraryRender,
  screen,
  waitFor,
  renderHook,
  RenderHookOptions,
  RenderHookResult,
} from "@testing-library/react";
import { type ReactNode } from "react";
import { QueryClient } from "react-query";
import { AppProviders } from "App";
import { RequireAuth } from "contexts/auth/RequireAuth";
import { ThemeProvider } from "contexts/ThemeProvider";
import { DashboardLayout } from "modules/dashboard/DashboardLayout";
import { TemplateSettingsLayout } from "pages/TemplateSettingsPage/TemplateSettingsLayout";
import { WorkspaceSettingsLayout } from "pages/WorkspaceSettingsPage/WorkspaceSettingsLayout";
import {
  type Location,
  type RouteObject,
  createMemoryRouter,
  RouterProvider,
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
    ...testingLibraryRender(
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

export type RouterLocationSnapshot<TLocationState = unknown> = Readonly<{
  search: URLSearchParams;
  pathname: string;
  state: Location<TLocationState>["state"];
}>;

/**
 * Gives you back an immutable snapshot of the current location's values.
 *
 * As this is a snapshot, its values can quickly become inaccurate - as soon as
 * a new re-render (even ones you didn't trigger yourself). Keep that in mind
 * when making assertions.
 */
export type GetLocationSnapshot<TLocationState = unknown> =
  () => RouterLocationSnapshot<TLocationState>;

export type RenderHookWithAuthResult<
  TResult,
  TProps,
  TLocationState = unknown,
> = Readonly<
  RenderHookResult<TResult, TProps> & {
    queryClient: QueryClient;
    getLocationSnapshot: GetLocationSnapshot<TLocationState>;
  }
>;

export type RenderHookWithAuthConfig<TProps> = Readonly<{
  routingOptions?: Omit<RenderWithAuthOptions, "children">;
  renderOptions?: Omit<RenderHookOptions<TProps>, "wrapper">;
}>;

/**
 * Gives you a custom version of renderHook that is aware of all our App
 * providers (query, routing, etc.).
 *
 * Unfortunately, React Router does not make it easy to access the router after
 * it's been set up, which can lead to some chicken-or-the-egg situations
 * @see {@link https://github.com/coder/coder/pull/10362#discussion_r1380852725}
 *
 * Initially tried setting up the router with a useState hook in the renderHook
 * wrapper, but even though it was valid for the mounting render, the code was
 * fragile. You could only safely test re-renders via your hook's exposed
 * methods; calling renderHook's rerender method directly caused the router to
 * get lost/disconnected.
 */
// Type param order mirrors the param order for renderHook
export async function renderHookWithAuth<Result, Props>(
  render: (initialProps: Props) => Result,
  config: RenderHookWithAuthConfig<Props>,
): Promise<RenderHookWithAuthResult<Result, Props>> {
  const { routingOptions = {}, renderOptions = {} } = config;
  const {
    path = "/",
    route = "/",
    extraRoutes = [],
    nonAuthenticatedRoutes = [],
  } = routingOptions;

  /**
   * Have to do some cursed things here. Scoured the tests for the React Router
   * source code, and from their examples, there didn't appear to be any
   * examples of them letting you expose the router directly. (One of the tests
   * created a dummy paragraph, and injected the values into that...)
   *
   * This breaks some React rules, but it makes sure that the code is resilient
   * to re-renders, and hopefully removes the need to make every test file that
   * uses renderHookWithAuth be defined as a .tsx file just to add dummy JSX.
   */
  // Easy to miss - evil definite assignments via !
  let escapedRouter!: ReturnType<typeof createMemoryRouter>;
  const queryClient = createTestQueryClient();
  const { result, rerender, unmount } = renderHook(render, {
    ...renderOptions,
    wrapper: ({ children }) => {
      // Cannot use useState here because even though the wrapper is a valid
      // component, calling renderHook's rerender method will cause the state to
      // get wiped (even though the underlying component instance should stay
      // the same?)
      if (escapedRouter === undefined) {
        const routes: RouteObject[] = [
          {
            element: <RequireAuth />,
            children: [{ path, element: <>{children}</> }, ...extraRoutes],
          },
          ...nonAuthenticatedRoutes,
        ];

        escapedRouter = createMemoryRouter(routes, {
          initialEntries: [route],
        });
      }

      return (
        <AppProviders queryClient={queryClient}>
          <RouterProvider router={escapedRouter} />
        </AppProviders>
      );
    },
  });

  /**
   * This is necessary to get around some providers in AppProviders having
   * conditional rendering and not always rendering their children immediately.
   *
   * renderHook's result won't actually exist until the children defined via its
   * wrapper render in full. Ignore result.current's type signature; it lies to
   * you, which is normally a good thing (no needless null checks), but not here
   */
  await waitFor(() => expect(result.current).not.toBe(null));

  if (escapedRouter === undefined) {
    throw new Error(
      "Do not have source of truth for location snapshots, even after custom hook value is ready",
    );
  }

  return {
    result,
    rerender,
    unmount,
    queryClient,
    getLocationSnapshot: () => {
      const location = escapedRouter.state.location;
      return {
        pathname: location.pathname,
        search: new URLSearchParams(location.search),
        state: location.state,
      };
    },
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
  return testingLibraryRender(component, {
    wrapper: ({ children }) => <ThemeProvider>{children}</ThemeProvider>,
  });
};
