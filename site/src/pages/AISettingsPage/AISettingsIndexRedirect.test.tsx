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

const renderRedirect = async (expectedPath: string) => {
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
			{ path: expectedPath, element: <div>Landed</div> },
		],
		{ initialEntries: ["/ai/settings"] },
	);

	render(
		<QueryClientProvider client={queryClient}>
			<RouterProvider router={router} />
		</QueryClientProvider>,
	);

	await screen.findByText("Landed");
	expect(router.state.location.pathname).toBe(expectedPath);
};

it("redirects a deployment administrator to Coder Agents", async () => {
	permissions = { ...MockNoPermissions, editDeploymentConfig: true };
	await renderRedirect("/ai/settings/coder-agents");
});

it("prefers Providers over every other AI page", async () => {
	permissions = {
		...MockNoPermissions,
		viewAnyAIProvider: true,
		viewAnyChatModelConfig: true,
		viewDeploymentConfig: true,
		viewAIGatewayKeys: true,
	};
	await renderRedirect("/ai/settings/providers");
});

it("prefers Models over AI Governance and AI Gateway keys", async () => {
	permissions = {
		...MockNoPermissions,
		viewAnyChatModelConfig: true,
		viewDeploymentConfig: true,
		viewAIGatewayKeys: true,
	};
	await renderRedirect("/ai/settings/models");
});

it("prefers AI Governance over AI Gateway keys", async () => {
	permissions = {
		...MockNoPermissions,
		viewDeploymentConfig: true,
		viewAIGatewayKeys: true,
	};
	await renderRedirect("/ai/settings/governance");
});

it("falls back to AI Gateway keys before Coder Agents", async () => {
	permissions = {
		...MockNoPermissions,
		viewAIGatewayKeys: true,
		editDeploymentConfig: true,
	};
	await renderRedirect("/ai/settings/gateway-keys");
});
