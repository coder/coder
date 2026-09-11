import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import type { FC, PropsWithChildren } from "react";
import { QueryClientProvider } from "react-query";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import { DashboardContext } from "#/modules/dashboard/DashboardProvider";
import { MockChat } from "#/testHelpers/chatEntities";
import {
	MockAppearanceConfig,
	MockBuildInfo,
	MockChatProject,
	MockDefaultOrganization,
	MockEntitlements,
} from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import AgentCreatePage from "./AgentCreatePage";

vi.mock("./components/AgentCreateForm", () => ({
	AgentCreateForm: ({
		onCreateChat,
	}: {
		onCreateChat: (options: {
			message: string;
			organizationId: string;
		}) => Promise<void>;
	}) => (
		<button
			type="button"
			onClick={() =>
				onCreateChat({
					message: "Create this chat",
					organizationId: MockDefaultOrganization.id,
				})
			}
		>
			Create chat
		</button>
	),
}));

vi.mock("./components/AgentPageHeader", () => ({
	AgentPageHeader: ({ children }: PropsWithChildren) => <div>{children}</div>,
}));
vi.mock("./components/ChimeButton", () => ({
	ChimeButton: () => null,
}));
vi.mock("./components/WebPushButton", () => ({
	WebPushButton: () => null,
}));
vi.mock("#/hooks/useAuthenticated", () => ({
	useAuthenticated: () => ({
		permissions: { createChat: true, editDeploymentConfig: false },
	}),
}));
vi.mock("#/hooks/useEmbeddedMetadata", () => ({
	useAIGatewayEnabled: () => true,
}));
vi.mock("#/contexts/useWebpushNotifications", () => ({
	useWebpushNotifications: () => ({ subscribed: false }),
}));

const Wrapper: FC<
	PropsWithChildren<{ experiments: TypesGen.Experiment[] }>
> = ({ children, experiments }) => {
	const queryClient = createTestQueryClient();
	return (
		<QueryClientProvider client={queryClient}>
			<DashboardContext.Provider
				value={{
					entitlements: MockEntitlements,
					experiments,
					appearance: MockAppearanceConfig,
					buildInfo: MockBuildInfo,
					organizations: [MockDefaultOrganization],
					showOrganizations: false,
					canViewOrganizationSettings: false,
				}}
			>
				<MemoryRouter
					initialEntries={[`/agents?project=${MockChatProject.id}`]}
				>
					<Routes>
						<Route path="/agents" element={children} />
						<Route path="/agents/:agentId" element={<div />} />
					</Routes>
				</MemoryRouter>
			</DashboardContext.Provider>
		</QueryClientProvider>
	);
};

afterEach(() => server.resetHandlers());

describe("AgentCreatePage project assignment", () => {
	it("includes the selected project ID when chat projects are enabled", async () => {
		const user = userEvent.setup();
		let requestBody: unknown;
		server.use(
			http.get("/api/experimental/chats/projects", () =>
				HttpResponse.json([MockChatProject]),
			),
			http.post("/api/v2/chats", async ({ request }) => {
				requestBody = await request.json();
				return HttpResponse.json({ ...MockChat, id: "created-chat" });
			}),
		);

		render(
			<Wrapper experiments={["chat-projects"]}>
				<AgentCreatePage />
			</Wrapper>,
		);

		await user.click(
			await screen.findByRole("button", { name: "Create chat" }),
		);

		await waitFor(() => {
			expect(requestBody).toMatchObject({ project_id: MockChatProject.id });
		});
	});

	it("omits the project ID when chat projects are disabled", async () => {
		const user = userEvent.setup();
		let requestBody: Record<string, unknown> | undefined;
		server.use(
			http.post("/api/v2/chats", async ({ request }) => {
				requestBody = (await request.json()) as Record<string, unknown>;
				return HttpResponse.json({ ...MockChat, id: "created-chat" });
			}),
		);

		render(
			<Wrapper experiments={[]}>
				<AgentCreatePage />
			</Wrapper>,
		);

		await user.click(screen.getByRole("button", { name: "Create chat" }));

		await waitFor(() => {
			expect(requestBody).toBeDefined();
		});
		expect(requestBody).not.toHaveProperty("project_id");
	});
});
