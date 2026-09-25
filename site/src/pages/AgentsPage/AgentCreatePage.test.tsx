import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import type { ComponentProps, FC, PropsWithChildren } from "react";
import { QueryClientProvider } from "react-query";
import {
	MemoryRouter,
	Route,
	Routes,
	useLocation,
	useNavigate,
} from "react-router";
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
	MockOrganization2,
} from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import AgentCreatePage from "./AgentCreatePage";
import type {
	AgentCreateForm,
	CreateChatOptions,
} from "./components/AgentCreateForm";

const { mountedLockedOrganizationIds } = vi.hoisted(() => ({
	mountedLockedOrganizationIds: [] as Array<string | undefined>,
}));

type MockAgentCreateFormProps = ComponentProps<typeof AgentCreateForm>;

vi.mock("./components/AgentCreateForm", () => ({
	AgentCreateForm: ({
		onCreateChat,
		isCreating,
		lockedOrganizationId,
		header,
		footer,
	}: MockAgentCreateFormProps) => {
		mountedLockedOrganizationIds.push(lockedOrganizationId);
		return (
			<div>
				<span data-testid="locked-organization">{lockedOrganizationId}</span>
				{header}
				<button
					type="button"
					disabled={isCreating}
					onClick={() =>
						onCreateChat({
							message: "Create this chat",
							organizationId:
								lockedOrganizationId ?? MockDefaultOrganization.id,
						} satisfies CreateChatOptions)
					}
				>
					Create chat
				</button>
				{footer}
			</div>
		);
	},
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

type LocationDisplayProps = Record<string, never>;

const LocationDisplay: FC<LocationDisplayProps> = () => {
	const location = useLocation();
	return <output>{location.pathname}</output>;
};

let navigateBack: (() => void) | undefined;

type NavigationBackProps = Record<string, never>;

const NavigationBack: FC<NavigationBackProps> = () => {
	const navigate = useNavigate();
	navigateBack = () => navigate(-1);
	return null;
};

type WrapperProps = PropsWithChildren<{
	experiments: TypesGen.Experiment[];
	initialEntry?: string;
	initialEntries?: string[];
	initialIndex?: number;
}>;

const Wrapper: FC<WrapperProps> = ({
	children,
	experiments,
	initialEntry = `/agents/projects/${MockChatProject.id}`,
	initialEntries,
	initialIndex,
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
				<MemoryRouter
					initialEntries={initialEntries ?? [initialEntry]}
					initialIndex={initialIndex}
				>
					<Routes>
						<Route path="/agents" element={children} />
						<Route path="/agents/projects/:projectId" element={children} />
						<Route path="/agents/:agentId" element={<div />} />
					</Routes>
					<LocationDisplay />
					<NavigationBack />
				</MemoryRouter>
			</DashboardContext.Provider>
		</QueryClientProvider>
	);
};

afterEach(() => {
	mountedLockedOrganizationIds.length = 0;
	navigateBack = undefined;
});

describe("AgentCreatePage project assignment", () => {
	it("includes the project ID from the route when chat projects are enabled", async () => {
		const user = userEvent.setup();
		const nonDefaultProject = {
			...MockChatProject,
			organization_id: MockOrganization2.id,
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
		// The form binds attachments and remembered choices to its organization
		// on mount, so it must never render against a provisional one.
		await waitFor(() => {
			expect(screen.getByTestId("locked-organization")).toHaveTextContent(
				MockOrganization2.id,
			);
		});
		expect(mountedLockedOrganizationIds).not.toContain(undefined);
		await user.click(screen.getByRole("button", { name: "Create chat" }));

		await waitFor(() => {
			expect(requestBody).toMatchObject({
				organization_id: MockOrganization2.id,
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

	it("retries a failed project lookup before offering the composer", async () => {
		const user = userEvent.setup();
		let lookupCount = 0;
		let requestBody: unknown;
		server.use(
			http.get(`/api/experimental/chats/projects/${MockChatProject.id}`, () => {
				lookupCount++;
				return lookupCount === 1
					? HttpResponse.json(
							{ message: "Project lookup failed" },
							{ status: 500 },
						)
					: HttpResponse.json(MockChatProject);
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

		await user.click(await screen.findByRole("button", { name: "Retry" }));
		await user.click(
			await screen.findByRole("button", { name: "Create chat" }),
		);

		await waitFor(() => {
			expect(requestBody).toMatchObject({ project_id: MockChatProject.id });
		});
		expect(lookupCount).toBe(2);
		expect(mountedLockedOrganizationIds).not.toContain(undefined);
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
	it("keeps the edit target aligned after browser history navigation", async () => {
		const user = userEvent.setup();
		const projectA = {
			...MockChatProject,
			id: "project-a",
			name: "Alpha",
			description: "Alpha description",
		};
		const projectB = {
			...MockChatProject,
			id: "project-b",
			name: "Beta",
			description: "Beta description",
		};
		let patchedProjectId: string | undefined;
		let requestBody: unknown;
		server.use(
			http.get("/api/experimental/chats/projects/:projectId", ({ params }) =>
				HttpResponse.json(
					params.projectId === projectA.id ? projectA : projectB,
				),
			),
			http.patch(
				"/api/experimental/chats/projects/:projectId",
				async ({ params, request }) => {
					patchedProjectId = String(params.projectId);
					requestBody = await request.json();
					return HttpResponse.json(projectB);
				},
			),
		);

		render(
			<Wrapper
				experiments={["chat-projects"]}
				initialEntries={[
					`/agents/projects/${projectB.id}`,
					`/agents/projects/${projectA.id}`,
				]}
				initialIndex={1}
			>
				<AgentCreatePage />
			</Wrapper>,
		);

		await user.click(
			await screen.findByRole("button", { name: "Edit project" }),
		);
		act(() => navigateBack?.());
		await user.click(
			await screen.findByRole("button", { name: "Edit project" }),
		);
		await user.click(screen.getByRole("button", { name: "Save" }));

		await waitFor(() => {
			expect(patchedProjectId).toBe(projectB.id);
			expect(requestBody).toEqual({
				name: projectB.name,
				description: projectB.description,
				icon: projectB.icon,
			});
		});
	});

	it("edits the project from the composer", async () => {
		const user = userEvent.setup();
		let requestBody: unknown;
		server.use(
			http.get(`/api/experimental/chats/projects/${MockChatProject.id}`, () =>
				HttpResponse.json(MockChatProject),
			),
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
		const dialog = await screen.findByRole("dialog", {
			name: "Edit project",
		});
		const nameInput = within(dialog).getByRole("textbox", { name: /Name/ });
		await user.clear(nameInput);
		await user.type(nameInput, "Renamed");
		await user.click(screen.getByRole("button", { name: "Save" }));

		await waitFor(() => {
			expect(requestBody).toMatchObject({ name: "Renamed" });
		});
	});
});
