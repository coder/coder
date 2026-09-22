import { render, screen, waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import type { FC, PropsWithChildren } from "react";
import { QueryClientProvider } from "react-query";
import { MemoryRouter } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
import { ThemeOverride } from "#/contexts/ThemeProvider";
import { DashboardContext } from "#/modules/dashboard/DashboardProvider";
import {
	MockAppearanceConfig,
	MockBuildInfo,
	MockDefaultOrganization,
	MockEntitlements,
	MockOrganization2,
} from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import themes, { DEFAULT_THEME } from "#/theme";
import {
	AgentCreateForm,
	selectedOrganizationIdStorageKey,
} from "./AgentCreateForm";

const Wrapper: FC<PropsWithChildren> = ({ children }) => {
	const queryClient = createTestQueryClient();
	return (
		<QueryClientProvider client={queryClient}>
			<DashboardContext.Provider
				value={{
					entitlements: MockEntitlements,
					experiments: [],
					appearance: MockAppearanceConfig,
					buildInfo: MockBuildInfo,
					organizations: [MockDefaultOrganization, MockOrganization2],
					showOrganizations: true,
					canViewOrganizationSettings: false,
				}}
			>
				<ThemeOverride theme={themes[DEFAULT_THEME]}>
					<TooltipProvider>
						<MemoryRouter>{children}</MemoryRouter>
					</TooltipProvider>
				</ThemeOverride>
			</DashboardContext.Provider>
		</QueryClientProvider>
	);
};

afterEach(() => {
	server.resetHandlers();
	localStorage.clear();
});

describe("AgentCreateForm", () => {
	it("keeps the remembered organization while a project locks another", async () => {
		localStorage.setItem(
			selectedOrganizationIdStorageKey,
			MockDefaultOrganization.id,
		);
		const mcpRequests: string[] = [];
		server.use(
			http.get("/api/v2/organizations", () =>
				HttpResponse.json([MockDefaultOrganization, MockOrganization2]),
			),
			http.post("/api/v2/authcheck", async ({ request }) => {
				const { checks } = (await request.json()) as {
					checks: Record<string, unknown>;
				};
				return HttpResponse.json(
					Object.fromEntries(Object.keys(checks).map((key) => [key, true])),
				);
			}),
			http.get(
				"/api/v2/organizations/:organization/mcp-servers",
				({ params }) => {
					mcpRequests.push(String(params.organization));
					return HttpResponse.json([]);
				},
			),
		);
		const formProps = {
			onCreateChat: vi.fn(),
			isCreating: false,
			createError: undefined,
			canCreateChat: true,
			canConfigureAgentSetup: false,
			aiGatewayDisabled: false,
			workspaceCount: 0,
			workspaceOptions: [],
			workspacesError: undefined,
			isWorkspacesLoading: false,
		};

		const { rerender } = render(
			<Wrapper>
				<AgentCreateForm
					{...formProps}
					lockedOrganizationId={MockOrganization2.id}
				/>
			</Wrapper>,
		);
		// The project's organization is in effect once permissions settle.
		await waitFor(() => {
			expect(mcpRequests).toContain(MockOrganization2.id);
		});
		// The same form instance survives navigating from the project page
		// back to the plain composer, as it does under the router.
		rerender(
			<Wrapper>
				<AgentCreateForm {...formProps} />
			</Wrapper>,
		);

		await waitFor(() => {
			expect(mcpRequests).toContain(MockDefaultOrganization.id);
		});
		expect(localStorage.getItem(selectedOrganizationIdStorageKey)).toBe(
			MockDefaultOrganization.id,
		);
	});

	it("denies access when the locked organization is not permitted", async () => {
		server.use(
			http.get("/api/v2/organizations", () =>
				HttpResponse.json([MockDefaultOrganization, MockOrganization2]),
			),
			http.post("/api/v2/authcheck", async ({ request }) => {
				const { checks } = (await request.json()) as {
					checks: Record<string, unknown>;
				};
				return HttpResponse.json(
					Object.fromEntries(
						Object.keys(checks).map((key) => [
							key,
							key === MockDefaultOrganization.id,
						]),
					),
				);
			}),
		);

		render(
			<Wrapper>
				<AgentCreateForm
					lockedOrganizationId={MockOrganization2.id}
					onCreateChat={vi.fn()}
					isCreating={false}
					createError={undefined}
					canCreateChat
					canConfigureAgentSetup={false}
					aiGatewayDisabled={false}
					workspaceCount={0}
					workspaceOptions={[]}
					workspacesError={undefined}
					isWorkspacesLoading={false}
				/>
			</Wrapper>,
		);

		// The user may create chats elsewhere, but not in the project's
		// organization, so the composer must explain the denial.
		await screen.findByText("Permission required");
	});
});
