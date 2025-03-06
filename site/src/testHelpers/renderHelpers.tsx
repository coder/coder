import {
	screen,
	render as testingLibraryRender,
	waitFor,
} from "@testing-library/react";
import { AppProviders } from "App";
import type { ProxyProvider } from "contexts/ProxyContext";
import { ThemeOverride } from "contexts/ThemeProvider";
import { RequireAuth } from "contexts/auth/RequireAuth";
import { DashboardLayout } from "modules/dashboard/DashboardLayout";
import type { DashboardProvider } from "modules/dashboard/DashboardProvider";
import OrganizationSettingsLayout from "modules/management/OrganizationSettingsLayout";
import { TemplateSettingsLayout } from "pages/TemplateSettingsPage/TemplateSettingsLayout";
import { WorkspaceSettingsLayout } from "pages/WorkspaceSettingsPage/WorkspaceSettingsLayout";
import type { ReactNode } from "react";
import { QueryClient } from "react-query";
import {
	type RouteObject,
	RouterProvider,
	createMemoryRouter,
} from "react-router-dom";
import themes, { DEFAULT_THEME } from "theme";
import { MockUser } from "./entities";

export function createTestQueryClient() {
	// Helps create one query client for each test case, to make sure that tests
	// are isolated and can't affect each other
	return new QueryClient({
		logger: {
			...console,
			// Some tests are designed to throw errors as part of their functionality.
			// To avoid unnecessary noise from these expected errors, the code is
			// structured to suppress them. If this suppression becomes problematic,
			// the code can be refactored to handle query errors on a per-test basis.
			error: () => {},
		},
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

	mockAuthProviders?: Readonly<{
		DashboardProvider?: typeof DashboardProvider;
		ProxyProvider?: typeof ProxyProvider;
	}>;
};

export function renderWithAuth(
	element: JSX.Element,
	{
		path = "/",
		route = "/",
		extraRoutes = [],
		nonAuthenticatedRoutes = [],
		mockAuthProviders = {},
		children,
	}: RenderWithAuthOptions = {},
) {
	const routes: RouteObject[] = [
		{
			element: <RequireAuth {...mockAuthProviders} />,
			children: [{ path, element, children }, ...extraRoutes],
		},
		...nonAuthenticatedRoutes,
	];

	const renderResult = renderWithRouter(
		createMemoryRouter(routes, { initialEntries: [route] }),
	);

	return {
		...renderResult,
		user: MockUser,
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

export function renderWithOrganizationSettingsLayout(
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
							element: <OrganizationSettingsLayout />,
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
		wrapper: ({ children }) => (
			<ThemeOverride theme={themes[DEFAULT_THEME]}>{children}</ThemeOverride>
		),
	});
};
