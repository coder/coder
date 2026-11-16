import { act, render, screen, waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import {
	createMemoryRouter,
	matchRoutes,
	Outlet,
	RouterProvider,
} from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { preferenceSettingsKey } from "#/api/queries/users";
import type { UserPreferenceSettings } from "#/api/typesGenerated";
import { createDeferred } from "#/testHelpers/deferred";
import {
	MockPermissions,
	MockUserPreferenceSettings,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import { router } from "./router";

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

	await screen.findByText("Skills");
	expect(memoryRouter.state.location.pathname).toBe("/agents/settings/skills");
});

const renderDashboardRoot = (route = "/") => {
	const match = matchRoutes(router.routes, "/")?.at(-1);
	return renderWithAuth(<Outlet />, {
		route,
		children: [{ index: true, element: match?.route.element }],
		extraRoutes: [
			{ path: "/workspaces", element: <div>Workspaces</div> },
			{ path: "/agents", element: <div>Agents</div> },
		],
	});
};

describe("dashboard landing page", () => {
	afterEach(() => vi.restoreAllMocks());

	it.each([
		{ preference: "workspaces", destination: "/workspaces" },
		{ preference: "agents", destination: "/agents" },
		{ preference: undefined, destination: "/workspaces" },
		{ preference: "https://example.com", destination: "/workspaces" },
	])(
		"replaces the root with $destination for $preference",
		async ({ preference, destination }) => {
			server.use(
				http.get("/api/v2/users/me/preferences", () =>
					HttpResponse.json({
						...MockUserPreferenceSettings,
						default_landing_page: preference,
					}),
				),
			);
			const { router } = renderDashboardRoot();

			await waitFor(() => {
				expect(router.state.location.pathname).toBe(destination);
			});
			expect(router.state.historyAction).toBe("REPLACE");
		},
	);

	it.each([false, true])(
		"waits for the latest saved preference before leaving the root (cached: %s)",
		async (cached) => {
			const preference = createDeferred<UserPreferenceSettings>();
			const getPreference = vi
				.spyOn(API, "getUserPreferenceSettings")
				.mockReturnValue(preference.promise);
			const { router, queryClient } = renderDashboardRoot(
				cached ? "/workspaces" : "/",
			);

			if (cached) {
				await screen.findByText("Workspaces");
				queryClient.setQueryDefaults(preferenceSettingsKey, {
					staleTime: Number.POSITIVE_INFINITY,
				});
				queryClient.setQueryData(
					preferenceSettingsKey,
					MockUserPreferenceSettings,
				);
				await act(() => router.navigate("/"));
			}

			await waitFor(() => expect(getPreference).toHaveBeenCalled());
			expect(router.state.location.pathname).toBe("/");

			preference.resolve({
				...MockUserPreferenceSettings,
				default_landing_page: "agents",
			});
			await waitFor(() => {
				expect(router.state.location.pathname).toBe("/agents");
			});
		},
	);

	it("falls back to Workspaces when Agents is unavailable", async () => {
		vi.spyOn(API, "getUserPreferenceSettings").mockResolvedValue({
			...MockUserPreferenceSettings,
			default_landing_page: "agents",
		});
		vi.spyOn(API, "checkAuthorization").mockResolvedValue({
			...MockPermissions,
			createChat: false,
		});
		const { router } = renderDashboardRoot();

		await waitFor(() => {
			expect(router.state.location.pathname).toBe("/workspaces");
		});
	});

	it.each([
		{ cached: false, destination: "/workspaces" },
		{ cached: true, destination: "/agents" },
	])(
		"falls back to $destination when preferences cannot be loaded (cached: $cached)",
		async ({ cached, destination }) => {
			const preference = createDeferred<UserPreferenceSettings>();
			const getPreference = vi
				.spyOn(API, "getUserPreferenceSettings")
				.mockReturnValue(preference.promise);
			const { router, queryClient } = renderDashboardRoot(
				cached ? "/workspaces" : "/",
			);

			if (cached) {
				await screen.findByText("Workspaces");
				queryClient.setQueryData<UserPreferenceSettings>(
					preferenceSettingsKey,
					{
						...MockUserPreferenceSettings,
						default_landing_page: "agents",
					},
				);
				await act(() => router.navigate("/"));
			}

			await waitFor(() => expect(getPreference).toHaveBeenCalled());
			expect(router.state.location.pathname).toBe("/");
			preference.reject(new Error("Preferences unavailable"));
			await waitFor(() => {
				expect(router.state.location.pathname).toBe(destination);
			});
		},
	);
});
