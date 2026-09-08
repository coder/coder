import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "react-query";
import { createMemoryRouter, RouterProvider } from "react-router";
import { beforeEach, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type { Permissions } from "#/modules/permissions";
import {
	MockDefaultOrganization,
	MockNoPermissions,
	MockUserOwner,
} from "#/testHelpers/entities";
import { AISettingsIndexRedirect } from "./AISettingsIndexRedirect";

let permissions: Permissions = MockNoPermissions;

vi.mock("#/hooks/useAuthenticated", () => ({
	useAuthenticated: () => ({
		user: MockUserOwner,
		permissions,
	}),
}));

vi.mock("#/modules/dashboard/useDashboard", () => ({
	useDashboard: () => ({ organizations: [MockDefaultOrganization] }),
}));

beforeEach(() => {
	permissions = MockNoPermissions;
});

it("redirects a deployment administrator to Coder Agents", async () => {
	permissions = { ...MockNoPermissions, editDeploymentConfig: true };
	const queryClient = new QueryClient({
		defaultOptions: { queries: { retry: false } },
	});
	vi.spyOn(API.experimental, "getChatModels").mockRejectedValue({
		isAxiosError: true,
		response: { status: 403 },
	});
	const router = createMemoryRouter(
		[
			{ path: "/ai/settings", element: <AISettingsIndexRedirect /> },
			{
				path: "/ai/settings/coder-agents",
				element: <div>Coder Agents</div>,
			},
		],
		{ initialEntries: ["/ai/settings"] },
	);

	render(
		<QueryClientProvider client={queryClient}>
			<RouterProvider router={router} />
		</QueryClientProvider>,
	);

	await screen.findByText("Coder Agents");
	expect(router.state.location.pathname).toBe("/ai/settings/coder-agents");
});
