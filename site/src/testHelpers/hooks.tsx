import {
  type RenderHookOptions,
  type RenderHookResult,
  waitFor,
  renderHook,
  act,
} from "@testing-library/react";
import { type ReactNode, useReducer, FC, PropsWithChildren } from "react";
import { QueryClient } from "react-query";
import { AppProviders } from "App";
import { RequireAuth } from "contexts/auth/RequireAuth";
import {
  type RenderWithAuthOptions,
  createTestQueryClient,
} from "./renderHelpers";
import {
  type Location,
  createMemoryRouter,
  RouterProvider,
  useLocation,
} from "react-router-dom";

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
  const { routingOptions = {}, renderOptions = {} } = config;
  const {
    path = "/",
    route = "/",
    extraRoutes = [],
    nonAuthenticatedRoutes = [],
  } = routingOptions;

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
   * The current solution side-steps that with a Rube Goldberg approach:
   * 1. When renderHook's wrapper renders/re-renders, store its children value
   *    (the JSX for the shell component it uses to expose hook values) in a
   *    separate variable
   * 2. Create a RenderHookEscapeHatch component that has two jobs:
   *    1. Read from the most recent child value escaped from the wrapper
   *    2. Expose a function for manually triggering re-renders for
   *       RenderHookEscapeHatch only (parent components will be unaffected)
   * 3. When we call renderHook for the mounting render, RouterProvider will
   *    be getting initialized, so it will let the render go through
   *    1. We eject the children value outside the wrapper and catch it with the
   *       variable.
   *    2. RenderHookEscapeHatch renders and uses the variable for its output,
   *       and exposes the forced re-render helper
   *    3. We eventually get our full JSX output, and React DOM turns that into
   *       some UI values
   * 4. If we have no need for manual re-renders, we're done at this point. Any
   *    re-renders done via the functions on the custom hook you're trying to
   *    test will have no problems, and will not have re-render issues
   *
   * But if we re-render manually via renderHook's rerender function:
   * 1. We grab a copy of the reference to the result.current property
   * 2. We trigger a re-render via renderHook to make sure that we're exposing
   *    the new re-render props to the variable
   * 3. RouterProvider will block the re-render, so RenderHookEscapeHatch does
   *    not produce a new value
   * 4. We call the force re-render helper to make RenderHookEscapeHatch
   *    re-render as a child node.
   * 5. It reads from the variable, so it will inject the most up to date
   *    version of the renderHook shell component JSX into its output
   * 6. Just to be on the safe side, we wait for result.current not to equal
   *    the snapshot we grabbed (even if the inner values are the same, the
   *    reference values will be different), resolving that via a promise
   */
  // Easy to miss - definite assignments via !

  let escapedLocation!: Location;
  const LocationLeaker: FC<PropsWithChildren> = ({ children }) => {
    const location = useLocation();
    escapedLocation = location;
    return <>{children}</>;
  };

  let forceRenderHookChildrenUpdate!: () => void;
  let currentRenderHookChildren: ReactNode = undefined;

  const InitialRoute: FC = () => {
    const [, forceInternalRerender] = useReducer((b: boolean) => !b, false);
    forceRenderHookChildrenUpdate = () => act(() => forceInternalRerender());
    return <LocationLeaker>{currentRenderHookChildren}</LocationLeaker>;
  };

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
   * wrapper render in full. Ignore result.current's type signature; it lies to
   * you, which is normally a good thing (no needless null checks), but not here
   */
  await waitFor(() => expect(result.current).not.toBe(null));

  return {
    result,
    queryClient,
    unmount,
    rerender: (newProps) => {
      const currentValue = result.current;
      rerender(newProps);
      forceRenderHookChildrenUpdate();
      return waitFor(() => expect(result.current).not.toBe(currentValue));
    },
    getLocationSnapshot: () => {
      return {
        pathname: escapedLocation.pathname,
        search: new URLSearchParams(escapedLocation.search),
        state: escapedLocation.state,
      };
    },
  } as const;
}
