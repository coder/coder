import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "react-query";
import { createMemoryRouter, RouterProvider } from "react-router";
import { beforeEach, expect, it, vi } from "vitest";
import { API, withDefaultFeatures } from "#/api/api";
import type { Permissions } from "#/modules/permissions";
import {
	MockDefaultOrganization,
	MockEntitlements,
	MockNoPermissions,
	MockUserOwner,
} from "#/testHelpers/entities";
import { AISettingsIndexRedirect } from "./AISettingsIndexRedirect";

let permissions: Permissions = MockNoPermissions;
let entitlements = MockEntitlements;

vi.mock("#/hooks/useAuthenticated", () => ({
	useAuthenticated: () => ({
		user: MockUserOwner,
		permissions,
	}),
}));

vi.mock("#/modules/dashboard/useDashboard", () => ({
	useDashboard: () => ({
		organizations: [MockDefaultOrganization],
		entitlements,
	}),
}));

beforeEach(() => {
	permissions = MockNoPermissions;
	entitlements = MockEntitlements;
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

it("redirects an organization group member reader to Spend", async () => {
	entitlements = {
		...MockEntitlements,
		features: withDefaultFeatures({
			aibridge: { enabled: true, entitlement: "entitled" },
		}),
	};
	vi.spyOn(API.experimental, "getChatModels").mockRejectedValue({
		isAxiosError: true,
		response: { status: 403 },
	});
	vi.spyOn(API, "getOrganizations").mockResolvedValue([
		MockDefaultOrganization,
	]);
	vi.spyOn(API, "checkAuthorization").mockResolvedValue({
		[MockDefaultOrganization.id]: true,
	});
	const queryClient = new QueryClient({
		defaultOptions: { queries: { retry: false } },
	});
	const router = createMemoryRouter(
		[
			{ path: "/ai/settings", element: <AISettingsIndexRedirect /> },
			{ path: "/ai/settings/spend", element: <div>Spend</div> },
		],
		{ initialEntries: ["/ai/settings"] },
	);

	render(
		<QueryClientProvider client={queryClient}>
			<RouterProvider router={router} />
		</QueryClientProvider>,
	);

	await screen.findByText("Spend");
	expect(router.state.location.pathname).toBe("/ai/settings/spend");
});

it("redirects an organization group manager to Agent Hours", async () => {
	entitlements = {
		...MockEntitlements,
		features: withDefaultFeatures({
			agent_runtime_hours: { enabled: true, entitlement: "entitled" },
		}),
	};
	vi.spyOn(API.experimental, "getChatModels").mockRejectedValue({
		isAxiosError: true,
		response: { status: 403 },
	});
	vi.spyOn(API, "getOrganizations").mockResolvedValue([
		MockDefaultOrganization,
	]);
	vi.spyOn(API, "checkAuthorization").mockResolvedValue({
		[MockDefaultOrganization.id]: true,
	});
	const queryClient = new QueryClient({
		defaultOptions: { queries: { retry: false } },
	});
	const router = createMemoryRouter(
		[
			{ path: "/ai/settings", element: <AISettingsIndexRedirect /> },
			{ path: "/ai/settings/agent-hours", element: <div>Agent Hours</div> },
		],
		{ initialEntries: ["/ai/settings"] },
	);

	render(
		<QueryClientProvider client={queryClient}>
			<RouterProvider router={router} />
		</QueryClientProvider>,
	);

	await screen.findByText("Agent Hours");
	expect(router.state.location.pathname).toBe("/ai/settings/agent-hours");
});
