import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import type { FC, PropsWithChildren, ReactNode } from "react";
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
		header,
		footer,
	}: {
		onCreateChat: (options: {
			message: string;
			organizationId: string;
		}) => Promise<void>;
		isCreating: boolean;
		header?: ReactNode;
		footer?: ReactNode;
	}) => (
		<div>
			{header}
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
			{footer}
		</div>
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
	return <output>{location.pathname}</output>;
};

const projectPath = `/agents/projects/${MockChatProject.id}`;

const Wrapper: FC<
	PropsWithChildren<{
		experiments: TypesGen.Experiment[];
		initialEntry?: string;
	}>
> = ({ children, experiments, initialEntry = projectPath }) => {
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
						<Route path="/agents/projects/:projectId" element={children} />
						<Route path="/agents/:agentId" element={<div />} />
					</Routes>
					<LocationDisplay />
				</MemoryRouter>
			</DashboardContext.Provider>
		</QueryClientProvider>
	);
};

const grantProjectPermissions = (granted: boolean) =>
	http.post("/api/v2/authcheck", async ({ request }) => {
		const { checks } = (await request.json()) as {
			checks: Record<string, unknown>;
		};
		return HttpResponse.json(
			Object.fromEntries(Object.keys(checks).map((key) => [key, granted])),
		);
	});

afterEach(() => server.resetHandlers());

describe("AgentCreatePage project assignment", () => {
	it("includes the project ID from the route when chat projects are enabled", async () => {
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
			<Wrapper experiments={[]} initialEntry="/agents">
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

	it("redirects to the new chat page when the project is missing", async () => {
		server.use(
			http.get(`/api/experimental/chats/projects/${MockChatProject.id}`, () =>
				HttpResponse.json({ message: "Not found." }, { status: 404 }),
			),
		);

		render(
			<Wrapper experiments={["chat-projects"]}>
				<AgentCreatePage />
			</Wrapper>,
		);

		await waitFor(() => {
			expect(screen.getByRole("status")).toHaveTextContent(/^\/agents$/);
		});
	});

	it("redirects to the new chat page when chat projects are disabled", async () => {
		render(
			<Wrapper experiments={[]}>
				<AgentCreatePage />
			</Wrapper>,
		);

		await waitFor(() => {
			expect(screen.getByRole("status")).toHaveTextContent(/^\/agents$/);
		});
	});
});

describe("AgentCreatePage project frame", () => {
	it("shows the project name and description around the composer", async () => {
		server.use(
			http.get(`/api/experimental/chats/projects/${MockChatProject.id}`, () =>
				HttpResponse.json(MockChatProject),
			),
			grantProjectPermissions(false),
		);

		render(
			<Wrapper experiments={["chat-projects"]}>
				<AgentCreatePage />
			</Wrapper>,
		);

		expect(
			await screen.findByRole("heading", { name: MockChatProject.name }),
		).toBeVisible();
		expect(screen.getByText(MockChatProject.description)).toBeVisible();
		await waitFor(() => {
			expect(screen.getByRole("button", { name: "Memory" })).toBeVisible();
		});
		expect(screen.queryByRole("button", { name: "Edit project" })).toBeNull();
	});

	it("edits the project when the user may update it", async () => {
		const user = userEvent.setup();
		let requestBody: unknown;
		server.use(
			http.get(`/api/experimental/chats/projects/${MockChatProject.id}`, () =>
				HttpResponse.json(MockChatProject),
			),
			grantProjectPermissions(true),
			http.patch(
				`/api/experimental/chats/projects/${MockChatProject.id}`,
				async ({ request }) => {
					requestBody = await request.json();
					return HttpResponse.json({ ...MockChatProject, name: "Renamed" });
				},
			),
		);

		render(
			<Wrapper experiments={["chat-projects"]}>
				<AgentCreatePage />
			</Wrapper>,
		);

		await user.click(
			await screen.findByRole("button", { name: "Edit project" }),
		);
		const nameInput = screen.getByLabelText("Name");
		await user.clear(nameInput);
		await user.type(nameInput, "Renamed");
		await user.click(screen.getByRole("button", { name: "Save" }));

		await waitFor(() => {
			expect(requestBody).toMatchObject({ name: "Renamed" });
		});
	});
});
