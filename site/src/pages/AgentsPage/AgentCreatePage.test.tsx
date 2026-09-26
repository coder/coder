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
import {
	afterEach,
	beforeAll,
	beforeEach,
	describe,
	expect,
	it,
	vi,
} from "vitest";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import type * as embeddedMetadata from "#/hooks/useEmbeddedMetadata";
import { DashboardContext } from "#/modules/dashboard/DashboardProvider";
import {
	buildDebugWorkspaceBuildPath,
	debugWorkspaceBuildSearchParam,
} from "#/modules/workspaces/workspaceBuildDebugLink";
import { MockChat } from "#/testHelpers/chatEntities";
import {
	MockChatModelProviderDescriptor,
	MockDefaultChatModel,
	MockUnsetUserChatPersonalModelOverrides,
} from "#/testHelpers/chatModels";
import {
	MockAppearanceConfig,
	MockBuildInfo,
	MockChatProject,
	MockDefaultOrganization,
	MockEntitlements,
	MockFailedWorkspaceBuild,
	MockOrganization2,
	MockUserPreferenceSettings,
	MockWorkspaceBuildLogs,
} from "#/testHelpers/entities";
import {
	createTestQueryClient,
	renderWithAuth,
} from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import AgentCreatePage from "./AgentCreatePage";
import type * as agentCreateForm from "./components/AgentCreateForm";
import {
	type AgentCreateForm,
	type CreateChatOptions,
	emptyInputStorageKey,
} from "./components/AgentCreateForm";
import { readAgentAttachmentText } from "./utils/fileAttachmentLimits";
import {
	debugWorkspaceBuildPrompt,
	formatWorkspaceBuildLogsForDebug,
} from "./utils/workspaceBuildDebug";

const { mountedLockedOrganizationIds, realForm } = vi.hoisted(() => ({
	mountedLockedOrganizationIds: [] as Array<string | undefined>,
	// The debug deep link tests need the real form to prefill and upload.
	realForm: { enabled: false },
}));

type MockAgentCreateFormProps = ComponentProps<typeof AgentCreateForm>;

vi.mock("./components/AgentCreateForm", async (importOriginal) => {
	const actual = await importOriginal<typeof agentCreateForm>();
	const StubAgentCreateForm = ({
		onCreateChat,
		isCreating,
		lockedOrganizationId,
		header,
		footer,
		prefill,
	}: MockAgentCreateFormProps) => {
		mountedLockedOrganizationIds.push(lockedOrganizationId);
		return (
			<div>
				<span data-testid="locked-organization">{lockedOrganizationId}</span>
				<span data-testid="prefill-message">{prefill?.message}</span>
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
	};
	return {
		...actual,
		AgentCreateForm: (props: MockAgentCreateFormProps) =>
			realForm.enabled ? (
				<actual.AgentCreateForm {...props} />
			) : (
				<StubAgentCreateForm {...props} />
			),
	};
});

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
vi.mock("#/hooks/useEmbeddedMetadata", async (importOriginal) => ({
	...(await importOriginal<typeof embeddedMetadata>()),
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

const failedBuild = MockFailedWorkspaceBuild();

const deepLink = `${buildDebugWorkspaceBuildPath(failedBuild.id)}&archived=archived`;

const enableExperiment = () => {
	server.use(
		http.get("/api/v2/experiments", () =>
			HttpResponse.json(["enable-ai-workspace-debug"]),
		),
	);
};

const mockPageQueries = () => {
	vi.spyOn(API, "getWorkspaceBuild").mockResolvedValue(failedBuild);
	vi.spyOn(API, "getWorkspaceBuildLogs").mockResolvedValue(
		MockWorkspaceBuildLogs,
	);
	vi.spyOn(API.experimental, "getChatModels").mockResolvedValue({
		models: [MockDefaultChatModel],
		providers: [MockChatModelProviderDescriptor],
		unsupported_providers: [],
	});
	vi.spyOn(
		API.experimental,
		"getUserChatPersonalModelOverrides",
	).mockResolvedValue(MockUnsetUserChatPersonalModelOverrides);
	vi.spyOn(API.experimental, "getMCPServerConfigs").mockResolvedValue([]);
	vi.spyOn(API, "getAIProviders").mockResolvedValue([]);
	vi.spyOn(API, "getUserPreferenceSettings").mockResolvedValue(
		MockUserPreferenceSettings,
	);
	return {
		uploadChatFile: vi
			.spyOn(API.experimental, "uploadChatFile")
			.mockResolvedValue({ id: "uploaded-logs" }),
		createChat: vi
			.spyOn(API.experimental, "createChat")
			.mockResolvedValue({ ...MockChat, id: "new-chat-id" }),
	};
};

const renderPage = (route = deepLink) =>
	renderWithAuth(<AgentCreatePage />, {
		path: "/agents",
		route,
		extraRoutes: [{ path: "/agents/:agentId", element: null }],
	});

const findEnabledSendButton = async () => {
	const sendButton = await screen.findByRole("button", { name: "Send" });
	await waitFor(() => expect(sendButton).toBeEnabled());
	return sendButton;
};

// Lexical reads selection geometry when text is pasted; jsdom has none.
beforeAll(() => {
	Object.defineProperty(Range.prototype, "getBoundingClientRect", {
		configurable: true,
		value: () => new DOMRect(0, 0, 1, 16),
	});
});

afterEach(() => {
	mountedLockedOrganizationIds.length = 0;
	navigateBack = undefined;
	realForm.enabled = false;
	vi.restoreAllMocks();
	localStorage.clear();
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
	it("holds the composer until both the project and the debug prefill load", async () => {
		let resolveLogs: (logs: TypesGen.ProvisionerJobLog[]) => void = () => {};
		vi.spyOn(API, "getWorkspaceBuild").mockResolvedValue(failedBuild);
		vi.spyOn(API, "getWorkspaceBuildLogs").mockReturnValue(
			new Promise((resolve) => {
				resolveLogs = resolve;
			}),
		);
		server.use(
			http.get(`/api/experimental/chats/projects/${MockChatProject.id}`, () =>
				HttpResponse.json(MockChatProject),
			),
		);

		render(
			<Wrapper
				experiments={["chat-projects", "enable-ai-workspace-debug"]}
				initialEntry={`/agents/projects/${MockChatProject.id}?${debugWorkspaceBuildSearchParam}=${failedBuild.id}`}
			>
				<AgentCreatePage />
			</Wrapper>,
		);

		await screen.findByRole("status", { name: "Loading workspace build logs" });
		expect(mountedLockedOrganizationIds).toEqual([]);
		act(() => resolveLogs(MockWorkspaceBuildLogs));

		expect(await screen.findByTestId("prefill-message")).toHaveTextContent(
			debugWorkspaceBuildPrompt(failedBuild),
		);
		expect(mountedLockedOrganizationIds).not.toContain(undefined);
		expect(screen.getByTestId("locked-organization")).toHaveTextContent(
			MockChatProject.organization_id,
		);
		expect(screen.getByRole("status")).toHaveTextContent(
			`/agents/projects/${MockChatProject.id}`,
		);
	});

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

describe("AgentCreatePage debug deep link", () => {
	beforeEach(() => {
		realForm.enabled = true;
	});

	it("prefills the prompt and the build logs, and sends them on Send", async () => {
		enableExperiment();
		const { uploadChatFile, createChat } = mockPageQueries();
		const user = userEvent.setup();

		const { router } = renderPage();

		const sendButton = await findEnabledSendButton();
		expect(uploadChatFile).toHaveBeenCalledTimes(1);
		const [uploadedFile] = uploadChatFile.mock.calls[0];
		expect(uploadedFile.name).toBe(
			"workspace-build-logs-TestUser-test-workspace-1.txt",
		);
		expect(await readAgentAttachmentText(uploadedFile)).toBe(
			formatWorkspaceBuildLogsForDebug(failedBuild, MockWorkspaceBuildLogs),
		);
		expect(createChat).not.toHaveBeenCalled();

		await user.click(sendButton);

		await waitFor(() => expect(createChat).toHaveBeenCalledTimes(1));
		expect(createChat).toHaveBeenCalledWith(
			expect.objectContaining({
				model_config_id: MockDefaultChatModel.id,
				content: [
					{ type: "text", text: debugWorkspaceBuildPrompt(failedBuild) },
					{ type: "file", file_id: "uploaded-logs" },
				],
			}),
		);
		// The chat's URL does not carry the build ID.
		await waitFor(() =>
			expect(router.state.location).toMatchObject({
				pathname: "/agents/new-chat-id",
				search: "?archived=archived",
			}),
		);
	});

	it("moves the build ID out of the URL so New chat gets a plain composer", async () => {
		enableExperiment();
		const { uploadChatFile, createChat } = mockPageQueries();
		localStorage.setItem(emptyInputStorageKey, "draft the user typed earlier");
		const user = userEvent.setup();

		const { router } = renderPage();

		await findEnabledSendButton();
		expect(router.state.location).toMatchObject({
			search: "?archived=archived",
			state: { debugWorkspaceBuildId: failedBuild.id },
		});
		// The layout's links forward location.search to a new history entry.
		await router.navigate({
			pathname: "/agents",
			search: router.state.location.search,
		});
		await waitFor(() =>
			expect(
				screen.getByRole("textbox", { name: "Chat message" }),
			).toHaveTextContent("draft the user typed earlier"),
		);

		await user.click(await findEnabledSendButton());

		await waitFor(() => expect(createChat).toHaveBeenCalledTimes(1));
		expect(createChat.mock.calls[0][0].content).toEqual([
			{ type: "text", text: "draft the user typed earlier" },
		]);
		expect(uploadChatFile).toHaveBeenCalledTimes(1);
	});

	it("leaves a plain composer when the build fails to load", async () => {
		enableExperiment();
		const { uploadChatFile, createChat } = mockPageQueries();
		vi.spyOn(API, "getWorkspaceBuild").mockRejectedValue(new Error("boom"));
		const getWorkspaceBuildLogs = vi.spyOn(API, "getWorkspaceBuildLogs");
		const user = userEvent.setup();

		renderPage();

		await screen.findByText("Could not load the workspace build or its logs");
		await user.click(
			await screen.findByRole("textbox", { name: "Chat message" }),
		);
		await user.paste("What happened?");
		await user.click(await findEnabledSendButton());

		await waitFor(() => expect(createChat).toHaveBeenCalledTimes(1));
		expect(createChat.mock.calls[0][0].content).toEqual([
			{ type: "text", text: "What happened?" },
		]);
		expect(getWorkspaceBuildLogs).not.toHaveBeenCalled();
		expect(uploadChatFile).not.toHaveBeenCalled();
	});
});
