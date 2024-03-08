import {
  type RenderHookOptions,
  type RenderHookResult,
  waitFor,
  renderHook,
  act,
} from "@testing-library/react";
import {
  type FC,
  type PropsWithChildren,
  type ReactNode,
  useReducer,
} from "react";
import type { QueryClient } from "react-query";
import {
  type Location,
  createMemoryRouter,
  RouterProvider,
  useLocation,
} from "react-router-dom";
import { AppProviders } from "App";
import { RequireAuth } from "contexts/auth/RequireAuth";
import {
  type RenderWithAuthOptions,
  createTestQueryClient,
} from "./renderHelpers";

export type RouterLocationSnapshot<TLocationState = unknown> = Readonly<{
  search: URLSearchParams;
  pathname: string;
  state: Location<TLocationState>["state"];
}>;

export type GetLocationSnapshot<TLocationState = unknown> =
  () => RouterLocationSnapshot<TLocationState>;

export type RenderHookWithAuthResult<
  TResult,
  TProps,
  TLocationState = unknown,
> = Readonly<
  Omit<RenderHookResult<TResult, TProps>, "rerender"> & {
    queryClient: QueryClient;
    rerender: (newProps: TProps) => Promise<void>;

    /**
     * Gives you back an immutable snapshot of the current location's values.
     *
     * As this is a snapshot, its values can quickly become inaccurate - as soon
     * as a new re-render (even ones you didn't trigger yourself). Keep that in
     * mind when making assertions.
     */
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
 */
export async function renderHookWithAuth<Result, Props>(
  render: (initialProps: Props) => Result,
  config: RenderHookWithAuthConfig<Props>,
): Promise<RenderHookWithAuthResult<Result, Props>> {
  /**
   * Our setup here is evil, gross, and cursed because of how React Router
   * itself is set up. We need to go through RouterProvider, so that we have a
   * Context for calling all the React Router hooks, but that poses two
   * problems:
   * 1. <RouterProvider> does not accept children, so there is no easy way to
   *    interface it with renderHook, which uses children as its only tool for
   *    dependency injection
   * 2. Even after you somehow jam a child value into the router, calling
   *    renderHook's rerender method will not do anything. RouterProvider is
   *    auto-memoized against re-renders, so because it thinks that its only
   *    input (the router object) hasn't changed, it will stop the re-render,
   *    and prevent any children from re-rendering (even if they would have new
   *    values).
   *
   * Have to do a lot of work to side-step those issues (best described as a
   * "Super Mario warp pipe"), and make sure that we're not relying on internal
   * React Router implementation details that could break at a moment's notice
   */
  // Some of the let variables are defined with definite assignment (! operator)
  let currentLocation!: Location;
  const LocationLeaker: FC<PropsWithChildren> = ({ children }) => {
    currentLocation = useLocation();
    return <>{children}</>;
  };

  let forceUpdateRenderHookChildren!: () => void;
  let currentRenderHookChildren: ReactNode = undefined;

  const InitialRoute: FC = () => {
    const [, forceRerender] = useReducer((b: boolean) => !b, false);
    forceUpdateRenderHookChildren = () => act(forceRerender);
    return <LocationLeaker>{currentRenderHookChildren}</LocationLeaker>;
  };

  const { routingOptions = {}, renderOptions = {} } = config;
  const {
    path = "/",
    route = "/",
    extraRoutes = [],
    nonAuthenticatedRoutes = [],
  } = routingOptions;

  const wrappedExtraRoutes = extraRoutes.map((route) => ({
    ...route,
    element: <LocationLeaker>{route.element}</LocationLeaker>,
  }));

  const wrappedNonAuthRoutes = nonAuthenticatedRoutes.map((route) => ({
    ...route,
    element: <LocationLeaker>{route.element}</LocationLeaker>,
  }));

  const router = createMemoryRouter(
    [
      {
        element: <RequireAuth />,
        children: [{ path, element: <InitialRoute /> }, ...wrappedExtraRoutes],
      },
      ...wrappedNonAuthRoutes,
    ],
    { initialEntries: [route], initialIndex: 0 },
  );

  const queryClient = createTestQueryClient();
  const { result, rerender, unmount } = renderHook<Result, Props>(render, {
    ...renderOptions,
    wrapper: ({ children }) => {
      currentRenderHookChildren = children;
      return (
        <AppProviders queryClient={queryClient}>
          <RouterProvider router={router} />
        </AppProviders>
      );
    },
  });

  /**
   * This is necessary to get around some providers in AppProviders having
   * conditional rendering and not always rendering their children immediately.
   *
   * renderHook's result won't actually exist until the children defined via its
   * wrapper render in full.
   *
   * Ignore result.current's type signature; it lies to you. This is normally a
   * good thing, because the renderHook result will usually evaluate
   * synchronously, so by the time you get the result back, you won't have to
   * worry about null checks. But because we're setting things up async,
   * result.current will be null for at least some period of time
   */
  await waitFor(() => expect(result.current).not.toBe(null));

  return {
    result,
    queryClient,
    unmount,
    rerender: async (newProps) => {
      const currentPathname = currentLocation.pathname;
      if (currentPathname !== path) {
        return;
      }

      const resultSnapshot = result.current;
      rerender(newProps);
      forceUpdateRenderHookChildren();
      return waitFor(() => expect(result.current).not.toBe(resultSnapshot));
    },
    getLocationSnapshot: () => {
      return {
        pathname: currentLocation.pathname,
        search: new URLSearchParams(currentLocation.search),
        state: currentLocation.state,
      };
    },
  } as const;
}
