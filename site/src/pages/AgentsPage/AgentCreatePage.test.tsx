import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import type { FC, PropsWithChildren } from "react";
import { QueryClientProvider } from "react-query";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
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
	MockOrganization,
} from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import AgentCreatePage from "./AgentCreatePage";

vi.mock("./components/AgentCreateForm", () => ({
	AgentCreateForm: ({
		onCreateChat,
		isCreating,
	}: {
		onCreateChat: (options: {
			message: string;
			organizationId: string;
		}) => Promise<void>;
		isCreating: boolean;
	}) => (
		<button
			type="button"
			disabled={isCreating}
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

const LocationDisplay: FC = () => {
	const location = useLocation();
	return <output>{location.search}</output>;
};

const AgentCreatePageWithLocation: FC = () => (
	<>
		<AgentCreatePage />
		<LocationDisplay />
	</>
);

const Wrapper: FC<
	PropsWithChildren<{
		experiments: TypesGen.Experiment[];
		initialEntry?: string;
	}>
> = ({
	children,
	experiments,
	initialEntry = `/agents?project=${MockChatProject.id}`,
}) => {
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
				<MemoryRouter initialEntries={[initialEntry]}>
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
		const nonDefaultProject = {
			...MockChatProject,
			organization_id: MockOrganization.id,
		};
		let projectRequested = false;
		let requestBody: unknown;
		server.use(
			http.get(`/api/experimental/chats/projects/${MockChatProject.id}`, () => {
				projectRequested = true;
				return HttpResponse.json(nonDefaultProject);
			}),
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

		await waitFor(() => {
			expect(projectRequested).toBe(true);
		});
		await user.click(
			await screen.findByRole("button", { name: "Create chat" }),
		);

		await waitFor(() => {
			expect(requestBody).toMatchObject({
				organization_id: MockOrganization.id,
				project_id: MockChatProject.id,
			});
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

	it("blocks chat creation when the project lookup fails", async () => {
		const user = userEvent.setup();
		let chatPostCount = 0;
		server.use(
			http.get(`/api/experimental/chats/projects/${MockChatProject.id}`, () =>
				HttpResponse.json(
					{ message: "Project lookup failed" },
					{ status: 500 },
				),
			),
			http.post("/api/v2/chats", () => {
				chatPostCount++;
				return HttpResponse.json({ ...MockChat, id: "created-chat" });
			}),
		);

		render(
			<Wrapper experiments={["chat-projects"]}>
				<AgentCreatePage />
			</Wrapper>,
		);

		await screen.findByText("Project lookup failed");
		await user.click(screen.getByRole("button", { name: "Create chat" }));

		expect(chatPostCount).toBe(0);
	});

	it("removes an unavailable project from the URL", async () => {
		server.use(
			http.get(`/api/experimental/chats/projects/${MockChatProject.id}`, () =>
				HttpResponse.json({ message: "Not found." }, { status: 404 }),
			),
		);

		render(
			<Wrapper experiments={["chat-projects"]}>
				<AgentCreatePageWithLocation />
			</Wrapper>,
		);

		await waitFor(() => {
			expect(screen.getByText("", { selector: "output" })).toHaveTextContent(
				"",
			);
		});
	});
});
