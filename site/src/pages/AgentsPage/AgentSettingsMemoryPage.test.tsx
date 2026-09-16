import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import type { FC, PropsWithChildren } from "react";
import { QueryClientProvider } from "react-query";
import { MemoryRouter } from "react-router";
import { afterEach, describe, expect, it } from "vitest";
import { DashboardContext } from "#/modules/dashboard/DashboardProvider";
import {
	MockAppearanceConfig,
	MockBuildInfo,
	MockDefaultOrganization,
	MockEntitlements,
} from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import AgentSettingsMemoryPage from "./AgentSettingsMemoryPage";

const Wrapper: FC<PropsWithChildren> = ({ children }) => {
	const queryClient = createTestQueryClient();
	return (
		<QueryClientProvider client={queryClient}>
			<DashboardContext.Provider
				value={{
					entitlements: MockEntitlements,
					experiments: ["chat-projects"],
					appearance: MockAppearanceConfig,
					buildInfo: MockBuildInfo,
					organizations: [MockDefaultOrganization],
					showOrganizations: false,
					canViewOrganizationSettings: false,
				}}
			>
				<MemoryRouter>{children}</MemoryRouter>
			</DashboardContext.Provider>
		</QueryClientProvider>
	);
};

afterEach(() => server.resetHandlers());

describe("AgentSettingsMemoryPage", () => {
	it("disables personal memory with a PUT request", async () => {
		const user = userEvent.setup();
		let requestBody: unknown;
		server.use(
			http.get("/api/v2/chats/config/user-memory", () =>
				HttpResponse.json({ enabled: true }),
			),
			http.get("/api/experimental/chats/memories", () => HttpResponse.json([])),
			http.put("/api/v2/chats/config/user-memory", async ({ request }) => {
				requestBody = await request.json();
				return new HttpResponse(null, { status: 204 });
			}),
		);

		render(
			<Wrapper>
				<AgentSettingsMemoryPage />
			</Wrapper>,
		);

		await user.click(
			await screen.findByRole("switch", {
				name: "Save and use personal memory",
			}),
		);

		await waitFor(() => {
			expect(requestBody).toEqual({ enabled: false });
		});
	});
});
