import { render, screen, waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import type { FC, PropsWithChildren } from "react";
import { QueryClientProvider } from "react-query";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, describe, expect, it } from "vitest";
import { DashboardContext } from "#/modules/dashboard/DashboardProvider";
import {
	MockAppearanceConfig,
	MockBuildInfo,
	MockChatProject,
	MockDefaultOrganization,
	MockEntitlements,
} from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import AgentProjectPage from "./AgentProjectPage";

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
				<MemoryRouter
					initialEntries={[`/agents/projects/${MockChatProject.id}`]}
				>
					<Routes>
						<Route path="/agents/projects/:projectId" element={children} />
					</Routes>
				</MemoryRouter>
			</DashboardContext.Provider>
		</QueryClientProvider>
	);
};

const projectHandlers = (canUpdate: boolean, onChecked?: () => void) => [
	http.get(`/api/experimental/chats/projects/${MockChatProject.id}`, () =>
		HttpResponse.json(MockChatProject),
	),
	http.get("/api/v2/chats", () => HttpResponse.json([])),
	http.post("/api/v2/authcheck", async ({ request }) => {
		const { checks } = (await request.json()) as {
			checks: Record<string, unknown>;
		};
		onChecked?.();
		return HttpResponse.json(
			Object.fromEntries(Object.keys(checks).map((key) => [key, canUpdate])),
		);
	}),
];

afterEach(() => server.resetHandlers());

describe("AgentProjectPage", () => {
	it("shows Edit when the user may update the project", async () => {
		server.use(...projectHandlers(true));

		render(
			<Wrapper>
				<AgentProjectPage />
			</Wrapper>,
		);

		await screen.findByRole("heading", { name: MockChatProject.name });
		await screen.findByRole("button", { name: "Edit" });
	});

	it("hides Edit when the user may not update the project", async () => {
		let authChecked = false;
		server.use(
			...projectHandlers(false, () => {
				authChecked = true;
			}),
		);

		render(
			<Wrapper>
				<AgentProjectPage />
			</Wrapper>,
		);

		await screen.findByRole("heading", { name: MockChatProject.name });
		await waitFor(() => expect(authChecked).toBe(true));
		expect(
			screen.queryByRole("button", { name: "Edit" }),
		).not.toBeInTheDocument();
		expect(screen.getByRole("link", { name: "New chat" })).toBeInTheDocument();
	});

	it("shows the error when the project fails to load", async () => {
		server.use(
			http.get(`/api/experimental/chats/projects/${MockChatProject.id}`, () =>
				HttpResponse.json(
					{ message: "Project unavailable" },
					{
						status: 500,
					},
				),
			),
			http.get("/api/v2/chats", () => HttpResponse.json([])),
		);

		render(
			<Wrapper>
				<AgentProjectPage />
			</Wrapper>,
		);

		await screen.findByText("Project unavailable");
	});
});
