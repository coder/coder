import { act, render, screen, waitFor } from "@testing-library/react";
import {
	createMemoryRouter,
	matchRoutes,
	type RouteObject,
	RouterProvider,
} from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { preferenceSettings, preferenceSettingsKey } from "#/api/queries/users";
import type { UserPreferenceSettings } from "#/api/typesGenerated";
import {
	defaultMetadataManager,
	MetadataManager,
} from "#/hooks/useEmbeddedMetadata";
import type * as DashboardLayoutModule from "#/modules/dashboard/DashboardLayout";
import { DashboardLayout } from "#/modules/dashboard/DashboardLayout";
import { createDeferred } from "#/testHelpers/deferred";
import {
	MockPermissions,
	MockUserPreferenceSettings,
} from "#/testHelpers/entities";
import { renderWithRouter } from "#/testHelpers/renderHelpers";
import { router } from "./router";

vi.mock("#/modules/dashboard/DashboardLayout", async (importOriginal) => {
	const original = await importOriginal<typeof DashboardLayoutModule>();
	return { ...original, DashboardLayout: vi.fn(original.DashboardLayout) };
});

it("redirects the old personal skills settings path to the skills page", async () => {
	const oldPath = "/agents/settings/personal-skills";
	const match = matchRoutes(router.routes, oldPath)?.at(-1);
	const memoryRouter = createMemoryRouter(
		[
			{ path: oldPath, element: match?.route.element },
			{ path: "/agents/settings/skills", element: <div>Skills</div> },
		],
		{ initialEntries: [oldPath] },
	);

	render(<RouterProvider router={memoryRouter} />);
	await waitFor(() => {
		expect(memoryRouter.state.location.pathname).toBe(
			"/agents/settings/skills",
		);
	});
});

const renderDashboardRoot = (route = "/") => {
	vi.spyOn(window, "scrollTo").mockImplementation(() => {});
	const matches = matchRoutes(router.routes, "/") ?? [];
	const rootRoutes = matches.reduceRight<RouteObject[]>(
		(children, { route }) => {
			return [route.index ? route : { ...route, children }];
		},
		[],
	);
	return renderWithRouter(
		createMemoryRouter(
			[
				...rootRoutes,
				{ path: "/workspaces", element: <h1>Workspaces</h1> },
				{ path: "/agents", element: <h1>Agents</h1> },
			],
			{ initialEntries: [route] },
		),
	);
};

describe("dashboard landing page", () => {
	afterEach(() => {
		vi.restoreAllMocks();
		vi.mocked(DashboardLayout).mockClear();
	});

	it.each([
		{
			name: "embedded Agents",
			preference: "agents",
			agentsEnabled: true,
			destination: "/agents",
		},
		{
			name: "embedded Workspaces",
			preference: "workspaces",
			agentsEnabled: true,
			destination: "/workspaces",
		},
		{
			name: "Agents without permission",
			preference: "agents",
			agentsEnabled: false,
			destination: "/workspaces",
		},
		{
			name: "invalid embedded preference",
			preference: "https://example.com",
			agentsEnabled: true,
			destination: "/workspaces",
		},
		{
			name: "missing preference",
			preference: undefined,
			agentsEnabled: true,
			destination: "/workspaces",
		},
		{
			name: "missing preference without Agents permission",
			preference: undefined,
			agentsEnabled: false,
			destination: "/workspaces",
		},
	])(
		"replaces root from $name without fetching or mounting the classic dashboard",
		async ({ preference, agentsEnabled, destination }) => {
			const node = document.createElement("meta");
			node.setAttribute("data-landing-metadata", "userPreferences");
			if (preference !== undefined) {
				node.setAttribute(
					"content",
					JSON.stringify({
						...MockUserPreferenceSettings,
						default_landing_page: preference,
					}),
				);
				document.head.append(node);
			}
			const metadata = new MetadataManager("data-landing-metadata");
			node.remove();
			vi.spyOn(defaultMetadataManager, "getMetadata").mockReturnValue({
				...defaultMetadataManager.getMetadata(),
				userPreferences: metadata.getMetadata().userPreferences,
			});
			const getPreference = vi
				.spyOn(API, "getUserPreferenceSettings")
				.mockResolvedValue(MockUserPreferenceSettings);
			vi.spyOn(API, "checkAuthorization").mockResolvedValue({
				...MockPermissions,
				createChat: agentsEnabled,
			});
			const updatePreference = vi.spyOn(API, "updateUserPreferenceSettings");
			const { router, queryClient } = renderDashboardRoot();

			await waitFor(() => {
				expect(router.state.location.pathname).toBe(destination);
			});
			expect(router.state.historyAction).toBe("REPLACE");
			expect(DashboardLayout).not.toHaveBeenCalled();
			expect(getPreference).not.toHaveBeenCalled();
			expect(updatePreference).not.toHaveBeenCalled();
			if (preference === undefined) {
				expect(queryClient.getQueryData(preferenceSettingsKey)).toBeUndefined();
			}
		},
	);

	it.each([
		{ cached: "agents", embedded: "workspaces", destination: "/agents" },
		{ cached: "workspaces", embedded: "agents", destination: "/workspaces" },
		{ cached: "agents", embedded: undefined, destination: "/agents" },
	] as const)(
		"uses cached $cached over embedded $embedded",
		async ({ cached, embedded, destination }) => {
			const metadata = defaultMetadataManager.getMetadata();
			vi.spyOn(defaultMetadataManager, "getMetadata").mockReturnValue({
				...metadata,
				userPreferences: {
					available: embedded !== undefined,
					value:
						embedded === undefined
							? undefined
							: {
									...MockUserPreferenceSettings,
									default_landing_page: embedded,
								},
				},
			});
			const getPreference = vi.spyOn(API, "getUserPreferenceSettings");
			const { router, queryClient } = renderDashboardRoot("/workspaces");
			queryClient.setQueryDefaults(preferenceSettingsKey, {
				gcTime: Number.POSITIVE_INFINITY,
			});
			queryClient.setQueryData(preferenceSettingsKey, {
				...MockUserPreferenceSettings,
				default_landing_page: cached,
			});
			await act(() => router.navigate("/"));

			await waitFor(() => {
				expect(router.state.location.pathname).toBe(destination);
			});
			expect(router.state.historyAction).toBe("REPLACE");
			expect(getPreference).not.toHaveBeenCalled();
		},
	);

	it.each(["agents", "workspaces", undefined] as const)(
		"does not wait for a shared refresh with cached preference %s",
		async (cached) => {
			const preference = createDeferred<UserPreferenceSettings>();
			const getPreference = vi
				.spyOn(API, "getUserPreferenceSettings")
				.mockReturnValue(preference.promise);
			const { router, queryClient } = renderDashboardRoot("/workspaces");
			await screen.findByRole("heading", { name: "Workspaces" });
			queryClient.setQueryDefaults(preferenceSettingsKey, {
				gcTime: Number.POSITIVE_INFINITY,
			});
			if (cached !== undefined) {
				queryClient.setQueryData(preferenceSettingsKey, {
					...MockUserPreferenceSettings,
					default_landing_page: cached,
				});
			}
			const pending = queryClient.fetchQuery(preferenceSettings());
			await waitFor(() => expect(getPreference).toHaveBeenCalledTimes(1));
			await act(() => router.navigate("/"));

			const destination = cached === "agents" ? "/agents" : "/workspaces";
			await waitFor(() => {
				expect(router.state.location.pathname).toBe(destination);
			});
			expect(router.state.historyAction).toBe("REPLACE");

			const mockUpdatedPreferences: UserPreferenceSettings = {
				...MockUserPreferenceSettings,
				default_landing_page: cached === "agents" ? "workspaces" : "agents",
			};
			await act(async () => {
				preference.resolve(mockUpdatedPreferences);
				await pending;
			});
			expect(queryClient.getQueryData(preferenceSettingsKey)).toEqual(
				mockUpdatedPreferences,
			);
			expect(router.state.location.pathname).toBe(destination);
			expect(getPreference).toHaveBeenCalledTimes(1);
		},
	);
});
