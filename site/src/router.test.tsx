import { act, render, screen, waitFor } from "@testing-library/react";
import {
	createMemoryRouter,
	matchRoutes,
	Outlet,
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
import { createDeferred } from "#/testHelpers/deferred";
import {
	MockPermissions,
	MockUserPreferenceSettings,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
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
	await waitFor(() => {
		expect(memoryRouter.state.location.pathname).toBe(
			"/agents/settings/skills",
		);
	});
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
			name: "missing preference without Agents permission",
			preference: undefined,
			agentsEnabled: false,
			destination: "/workspaces",
		},
	])(
		"replaces root from $name without fetching",
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
			const { router, queryClient } = renderDashboardRoot();

			await waitFor(() => {
				expect(router.state.location.pathname).toBe(destination);
			});
			expect(router.state.historyAction).toBe("REPLACE");
			expect(getPreference).not.toHaveBeenCalled();
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

	it.each([false, true])(
		"waits for missing preferences before navigating, existing request=%s",
		async (existingRequest) => {
			const preference = createDeferred<UserPreferenceSettings>();
			const getPreference = vi
				.spyOn(API, "getUserPreferenceSettings")
				.mockReturnValue(preference.promise);
			const { router, queryClient } = renderDashboardRoot("/workspaces");
			await screen.findByText("Workspaces");
			const pending = existingRequest
				? queryClient.fetchQuery(preferenceSettings())
				: undefined;
			await act(() => router.navigate("/"));
			await waitFor(() => expect(getPreference).toHaveBeenCalledTimes(1));
			expect(router.state.location.pathname).toBe("/");

			const saved: UserPreferenceSettings = {
				...MockUserPreferenceSettings,
				default_landing_page: "agents",
			};
			await act(async () => {
				preference.resolve(saved);
				await pending;
			});

			await waitFor(() => {
				expect(router.state.location.pathname).toBe("/agents");
			});
			expect(router.state.historyAction).toBe("REPLACE");
			expect(queryClient.getQueryData(preferenceSettingsKey)).toEqual(saved);
			expect(getPreference).toHaveBeenCalledTimes(1);
		},
	);

	it("does not wait for a shared refresh when preferences are cached", async () => {
		const preference = createDeferred<UserPreferenceSettings>();
		const getPreference = vi
			.spyOn(API, "getUserPreferenceSettings")
			.mockReturnValue(preference.promise);
		const { router, queryClient } = renderDashboardRoot("/workspaces");
		queryClient.setQueryDefaults(preferenceSettingsKey, {
			gcTime: Number.POSITIVE_INFINITY,
		});
		queryClient.setQueryData(preferenceSettingsKey, {
			...MockUserPreferenceSettings,
			default_landing_page: "workspaces",
		});
		const pending = queryClient.fetchQuery(preferenceSettings());
		await waitFor(() => expect(getPreference).toHaveBeenCalledTimes(1));
		await act(() => router.navigate("/"));

		await waitFor(() => {
			expect(router.state.location.pathname).toBe("/workspaces");
		});
		expect(router.state.historyAction).toBe("REPLACE");

		const updated: UserPreferenceSettings = {
			...MockUserPreferenceSettings,
			default_landing_page: "agents",
		};
		await act(async () => {
			preference.resolve(updated);
			await pending;
		});
		expect(queryClient.getQueryData(preferenceSettingsKey)).toEqual(updated);
		expect(router.state.location.pathname).toBe("/workspaces");
		expect(getPreference).toHaveBeenCalledTimes(1);
	});

	it("falls back after a failed read without saving or retrying on navigation", async () => {
		const getPreference = vi
			.spyOn(API, "getUserPreferenceSettings")
			.mockRejectedValue(new Error("Preferences unavailable"));
		const updatePreference = vi.spyOn(API, "updateUserPreferenceSettings");
		const { router, queryClient } = renderDashboardRoot();

		await waitFor(() => {
			expect(router.state.location.pathname).toBe("/workspaces");
		});
		expect(getPreference).toHaveBeenCalledTimes(1);
		expect(queryClient.getQueryData(preferenceSettingsKey)).toBeUndefined();

		await act(() => router.navigate("/"));
		await waitFor(() => {
			expect(router.state.location.pathname).toBe("/workspaces");
		});
		expect(router.state.historyAction).toBe("REPLACE");
		expect(getPreference).toHaveBeenCalledTimes(1);
		expect(updatePreference).not.toHaveBeenCalled();
	});
});
