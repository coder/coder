import { render, screen } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import type { FC, PropsWithChildren } from "react";
import { QueryClientProvider } from "react-query";
import { MemoryRouter } from "react-router";
import { afterEach, describe, it, vi } from "vitest";
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
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
import { AgentCreateForm } from "./AgentCreateForm";

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
				<TooltipProvider>
					<MemoryRouter>{children}</MemoryRouter>
				</TooltipProvider>
			</DashboardContext.Provider>
		</QueryClientProvider>
	);
};

afterEach(() => server.resetHandlers());

describe("AgentCreateForm", () => {
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
