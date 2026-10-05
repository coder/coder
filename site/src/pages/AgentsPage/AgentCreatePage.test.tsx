import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { toast } from "sonner";
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
import { chatProjectsKey } from "#/api/queries/chatProjects";
import type * as TypesGen from "#/api/typesGenerated";
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
	MockChatProject,
	MockDefaultOrganization,
	MockFailedWorkspaceBuild,
	MockOrganization2,
	MockUserPreferenceSettings,
	MockWorkspaceBuildLogs,
	mockApiError,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import AgentCreatePage from "./AgentCreatePage";
import type * as AgentCreateFormModule from "./components/AgentCreateForm";
import {
	type CreateChatOptions,
	emptyInputStorageKey,
} from "./components/AgentCreateForm";
import type { WorkspaceFileUpload } from "./hooks/useWorkspaceFileUploads";
import { readAgentAttachmentText } from "./utils/fileAttachmentLimits";
import {
	debugWorkspaceBuildPrompt,
	formatWorkspaceBuildLogsForDebug,
} from "./utils/workspaceBuildDebug";

const { renderedProjects, realForm, formProps } = vi.hoisted(() => ({
	renderedProjects: [] as Array<
		Pick<TypesGen.ChatProject, "id" | "organization_id"> | undefined
	>,
	// Prefill and upload tests need the real form.
	realForm: { enabled: false },
	formProps: {
		onCreateChat: undefined as
			| ((options: CreateChatOptions) => Promise<void>)
			| undefined,
	},
}));

type AgentCreateFormProps = React.ComponentProps<
	typeof AgentCreateFormModule.AgentCreateForm
>;

// Upload tests call the captured onCreateChat to drive the page's submit path
// directly.
vi.mock("./components/AgentCreateForm", async (importOriginal) => {
	const actual = await importOriginal<typeof AgentCreateFormModule>();
	const StubAgentCreateForm = ({
		onCreateChat,
		isCreating,
		createError,
		project,
		header,
		footer,
		prefill,
	}: AgentCreateFormProps) => {
		renderedProjects.push(project);
		return (
			<div>
				<span data-testid="prefill-message">{prefill?.message}</span>
				{header}
				{createError ? <p>Create failed</p> : null}
				<button
					type="button"
					disabled={isCreating}
					onClick={() =>
						onCreateChat({
							message: "Create this chat",
							organizationId:
								project?.organization_id ?? MockDefaultOrganization.id,
							manageAutomationsEnabled: false,
						} satisfies CreateChatOptions).catch(() => {})
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
		AgentCreateForm: (props: AgentCreateFormProps) => {
			formProps.onCreateChat = props.onCreateChat;
			return realForm.enabled ? (
				<actual.AgentCreateForm {...props} />
			) : (
				<StubAgentCreateForm {...props} />
			);
		},
	};
});

// AgentPageHeader needs the layout's outlet context.
vi.mock("./components/AgentPageHeader", () => ({
	AgentPageHeader: () => null,
}));

const projectPath = (projectId: string) => `/agents/projects/${projectId}`;

const enableExperiments = (...experiments: TypesGen.Experiment[]) => {
	server.use(
		http.get("/api/v2/experiments", () => HttpResponse.json(experiments)),
	);
};

const serveProjects = (...projects: TypesGen.ChatProject[]) => {
	server.use(
		http.get("/api/experimental/chats/projects", () =>
			HttpResponse.json(projects),
		),
	);
};

const renderAgentsRoutes = (route = projectPath(MockChatProject.id)) =>
	renderWithAuth(<AgentCreatePage />, {
		path: "/agents/projects/:projectId",
		route,
		extraRoutes: [
			{ path: "/agents", element: <AgentCreatePage /> },
			{ path: "/agents/:agentId", element: null },
		],
	});

const failedBuild = MockFailedWorkspaceBuild();

const deepLink = `${buildDebugWorkspaceBuildPath(failedBuild.id)}&archived=archived`;

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
	renderedProjects.length = 0;
	realForm.enabled = false;
	vi.restoreAllMocks();
	localStorage.clear();
});

describe("AgentCreatePage project assignment", () => {
	it("passes the route's project to the composer once it loads", async () => {
		enableExperiments("chat-projects");
		serveProjects({
			...MockChatProject,
			organization_id: MockOrganization2.id,
		});

		renderAgentsRoutes();

		await screen.findByRole("button", { name: "Create chat" });
		expect(renderedProjects).not.toContain(undefined);
		expect(renderedProjects.at(-1)).toMatchObject({
			id: MockChatProject.id,
			organization_id: MockOrganization2.id,
		});
	});

	it("creates the chat in the route's project", async () => {
		realForm.enabled = true;
		enableExperiments("chat-projects");
		serveProjects(MockChatProject);
		const { createChat } = mockPageQueries();
		const user = userEvent.setup();

		renderAgentsRoutes();

		await user.click(
			await screen.findByRole("textbox", { name: "Chat message" }),
		);
		await user.paste("Plan the launch");
		await user.click(await findEnabledSendButton());

		await waitFor(() => expect(createChat).toHaveBeenCalledTimes(1));
		expect(createChat.mock.calls[0][0]).toMatchObject({
			project_id: MockChatProject.id,
		});
	});

	it("omits the project ID on the plain new-chat route", async () => {
		enableExperiments("chat-projects");
		let requestBody: Record<string, unknown> | undefined;
		server.use(
			http.post("/api/v2/chats", async ({ request }) => {
				requestBody = (await request.json()) as Record<string, unknown>;
				return HttpResponse.json({ ...MockChat, id: "created-chat" });
			}),
		);
		const user = userEvent.setup();

		renderAgentsRoutes("/agents");

		await user.click(
			await screen.findByRole("button", { name: "Create chat" }),
		);
		await waitFor(() => {
			expect(requestBody).toBeDefined();
		});
		expect(requestBody).not.toHaveProperty("project_id");
	});

	it("retries a failed project lookup before offering the composer", async () => {
		enableExperiments("chat-projects");
		let lookupCount = 0;
		server.use(
			http.get("/api/experimental/chats/projects", () => {
				lookupCount++;
				return lookupCount === 1
					? HttpResponse.json({ message: "List failed" }, { status: 500 })
					: HttpResponse.json([MockChatProject]);
			}),
		);
		const user = userEvent.setup();

		renderAgentsRoutes();

		await screen.findByText("Failed to load project");
		expect(screen.getByText("List failed")).toBeInTheDocument();
		await user.click(screen.getByRole("button", { name: "Retry" }));
		await screen.findByRole("button", { name: "Create chat" });
		expect(lookupCount).toBe(2);
		expect(renderedProjects).not.toContain(undefined);
	});

	it("keeps a loaded project usable when a background refetch fails", async () => {
		enableExperiments("chat-projects");
		let lookupCount = 0;
		server.use(
			http.get("/api/experimental/chats/projects", () => {
				lookupCount++;
				return lookupCount === 1
					? HttpResponse.json([MockChatProject])
					: HttpResponse.json({ message: "List failed" }, { status: 500 });
			}),
		);

		const { queryClient } = renderAgentsRoutes();
		await screen.findByRole("button", { name: "Create chat" });
		await act(() =>
			queryClient.invalidateQueries({ queryKey: chatProjectsKey }),
		);
		await waitFor(() =>
			expect(queryClient.getQueryState(chatProjectsKey)?.status).toBe("error"),
		);

		expect(lookupCount).toBe(2);
		expect(screen.getByRole("button", { name: "Create chat" })).toBeEnabled();
		expect(
			screen.queryByText("Failed to load project"),
		).not.toBeInTheDocument();
	});

	it("shows a not-found message when the project is missing", async () => {
		enableExperiments("chat-projects");
		serveProjects();

		const { router } = renderAgentsRoutes();

		await screen.findByText("Project not found");
		expect(
			screen.getByRole("link", { name: "Start a new chat" }),
		).toHaveAttribute("href", "/agents");
		expect(router.state.location.pathname).toBe(
			projectPath(MockChatProject.id),
		);
	});

	it("waits for the refetch before treating a cached list as final", async () => {
		enableExperiments("chat-projects");
		serveProjects(MockChatProject);

		const { router, queryClient } = renderAgentsRoutes("/agents");
		await screen.findByRole("button", { name: "Create chat" });
		// A list cached before the project existed.
		act(() => {
			queryClient.setQueryData(chatProjectsKey, []);
			void router.navigate(projectPath(MockChatProject.id));
		});

		expect(screen.queryByText("Project not found")).not.toBeInTheDocument();
		await screen.findByRole("heading", { name: MockChatProject.name });
	});

	it("shows the plain composer on /agents after a project list is cached", async () => {
		enableExperiments("chat-projects");
		serveProjects(MockChatProject);

		const { router } = renderAgentsRoutes();
		await screen.findByRole("heading", { name: MockChatProject.name });
		await act(() => router.navigate("/agents"));

		await screen.findByRole("button", { name: "Create chat" });
		expect(screen.queryByText("Project not found")).toBeNull();
		expect(renderedProjects.at(-1)).toBeUndefined();
	});

	it("offers a retry when refreshing a stale list fails", async () => {
		enableExperiments("chat-projects");
		server.use(
			http.get("/api/experimental/chats/projects", () =>
				HttpResponse.json({ message: "List failed" }, { status: 500 }),
			),
		);

		const { router, queryClient } = renderAgentsRoutes("/agents");
		await screen.findByRole("button", { name: "Create chat" });
		act(() => {
			queryClient.setQueryData(chatProjectsKey, []);
			void router.navigate(projectPath(MockChatProject.id));
		});

		await screen.findByText("Failed to load project");
		expect(screen.queryByText("Project not found")).toBeNull();

		const retry = Promise.withResolvers<void>();
		server.use(
			http.get("/api/experimental/chats/projects", async () => {
				await retry.promise;
				return HttpResponse.json({ message: "List failed" }, { status: 500 });
			}),
		);
		const user = userEvent.setup();
		await user.click(screen.getByRole("button", { name: "Retry" }));
		await waitFor(() =>
			expect(screen.getByRole("button", { name: /Retry/ })).toBeDisabled(),
		);
		retry.resolve();
		await waitFor(() =>
			expect(screen.getByRole("button", { name: /Retry/ })).toBeEnabled(),
		);
	});

	it("redirects to the new chat page when chat projects are disabled", async () => {
		const { router } = renderAgentsRoutes();

		await waitFor(() => {
			expect(router.state.location.pathname).toBe("/agents");
		});
	});
});

describe("AgentCreatePage project frame", () => {
	it("keeps the project route and organization after the debug prefill loads", async () => {
		enableExperiments("chat-projects", "enable-ai-workspace-debug");
		serveProjects(MockChatProject);
		const logs = Promise.withResolvers<TypesGen.ProvisionerJobLog[]>();
		vi.spyOn(API, "getWorkspaceBuild").mockResolvedValue(failedBuild);
		vi.spyOn(API, "getWorkspaceBuildLogs").mockReturnValue(logs.promise);

		const { router } = renderAgentsRoutes(
			`${projectPath(MockChatProject.id)}?${debugWorkspaceBuildSearchParam}=${failedBuild.id}`,
		);

		await screen.findByRole("status", { name: "Loading workspace build logs" });
		expect(renderedProjects).toEqual([]);
		act(() => logs.resolve(MockWorkspaceBuildLogs));

		expect(await screen.findByTestId("prefill-message")).toHaveTextContent(
			debugWorkspaceBuildPrompt(failedBuild),
		);
		expect(renderedProjects).not.toContain(undefined);
		expect(renderedProjects.at(-1)?.organization_id).toBe(
			MockChatProject.organization_id,
		);
		expect(router.state.location.pathname).toBe(
			projectPath(MockChatProject.id),
		);
	});

	it("prefills the project composer from a prompt link", async () => {
		enableExperiments("chat-projects");
		serveProjects(MockChatProject);

		renderAgentsRoutes(`${projectPath(MockChatProject.id)}?prompt=hi`);

		expect(await screen.findByTestId("prefill-message")).toHaveTextContent(
			"hi",
		);
		expect(renderedProjects).not.toContain(undefined);
		expect(renderedProjects.at(-1)?.organization_id).toBe(
			MockChatProject.organization_id,
		);
	});

	it("shows a create error only under the project it was attempted for", async () => {
		enableExperiments("chat-projects");
		const projectB = { ...MockChatProject, id: "project-b", name: "Beta" };
		serveProjects(MockChatProject, projectB);
		server.use(
			http.post("/api/v2/chats", () =>
				HttpResponse.json({ message: "Create failed" }, { status: 500 }),
			),
		);
		const user = userEvent.setup();

		const { router } = renderAgentsRoutes();
		await user.click(
			await screen.findByRole("button", { name: "Create chat" }),
		);
		await screen.findByText("Create failed");
		await act(() => router.navigate(projectPath(projectB.id)));

		await screen.findByRole("heading", { name: "Beta" });
		expect(screen.queryByText("Create failed")).not.toBeInTheDocument();
	});

	it("keeps the edit target aligned after browser history navigation", async () => {
		enableExperiments("chat-projects");
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
		serveProjects(projectA, projectB);
		let patchedProjectId: string | undefined;
		server.use(
			http.patch(
				"/api/experimental/organizations/:organizationId/chats/projects/:projectId",
				({ params }) => {
					patchedProjectId = String(params.projectId);
					return HttpResponse.json(projectB);
				},
			),
		);
		const user = userEvent.setup();

		const { router } = renderAgentsRoutes(projectPath(projectB.id));
		await screen.findByRole("heading", { name: "Beta" });
		await act(() => router.navigate(projectPath(projectA.id)));
		await user.click(
			await screen.findByRole("button", { name: "Edit project" }),
		);
		await act(() => router.navigate(-1));
		await screen.findByRole("heading", { name: "Beta" });
		await user.click(screen.getByRole("button", { name: "Edit project" }));
		const dialog = await screen.findByRole("dialog", { name: "Edit project" });
		expect(within(dialog).getByRole("textbox", { name: /Name/ })).toHaveValue(
			"Beta",
		);
		const nameInput = within(dialog).getByRole("textbox", { name: /Name/ });
		await user.type(nameInput, "!");
		await user.click(within(dialog).getByRole("button", { name: "Save" }));

		await waitFor(() => {
			expect(patchedProjectId).toBe(projectB.id);
		});
	});

	it("edits the project from the composer and shows the saved name", async () => {
		enableExperiments("chat-projects");
		let project = MockChatProject;
		let requestBody: unknown;
		server.use(
			http.get("/api/experimental/chats/projects", () =>
				HttpResponse.json([project]),
			),
			http.patch(
				`/api/experimental/organizations/${MockChatProject.organization_id}/chats/projects/${MockChatProject.id}`,
				async ({ request }) => {
					requestBody = await request.json();
					project = { ...MockChatProject, name: "Renamed" };
					return HttpResponse.json(project);
				},
			),
		);
		const user = userEvent.setup();

		renderAgentsRoutes();

		await screen.findByRole("heading", { name: MockChatProject.name });
		await user.click(screen.getByRole("button", { name: "Edit project" }));
		const dialog = await screen.findByRole("dialog", { name: "Edit project" });
		const nameInput = within(dialog).getByRole("textbox", { name: /Name/ });
		await user.clear(nameInput);
		await user.type(nameInput, "Renamed");
		await user.click(within(dialog).getByRole("button", { name: "Save" }));

		await screen.findByRole("heading", { name: "Renamed" });
		expect(requestBody).toMatchObject({ name: "Renamed" });
		await waitFor(() => {
			expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
		});
	});

	it("clears a failed save when the edit dialog is reopened", async () => {
		enableExperiments("chat-projects");
		serveProjects(MockChatProject);
		server.use(
			http.patch(
				`/api/experimental/organizations/${MockChatProject.organization_id}/chats/projects/${MockChatProject.id}`,
				() => HttpResponse.json({ message: "Save failed" }, { status: 500 }),
			),
		);
		const user = userEvent.setup();

		renderAgentsRoutes();

		await user.click(
			await screen.findByRole("button", { name: "Edit project" }),
		);
		let dialog = await screen.findByRole("dialog", { name: "Edit project" });
		await user.type(within(dialog).getByRole("textbox", { name: /Name/ }), "!");
		await user.click(within(dialog).getByRole("button", { name: "Save" }));
		await within(dialog).findByText("Save failed");
		await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
		await waitFor(() => {
			expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
		});

		await user.click(screen.getByRole("button", { name: "Edit project" }));
		dialog = await screen.findByRole("dialog", { name: "Edit project" });
		expect(within(dialog).queryByText("Save failed")).not.toBeInTheDocument();
	});
});

describe("AgentCreatePage debug deep link", () => {
	beforeEach(() => {
		realForm.enabled = true;
	});

	it("prefills the prompt and the build logs, and sends them on Send", async () => {
		enableExperiments("enable-ai-workspace-debug");
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
		enableExperiments("enable-ai-workspace-debug");
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
		enableExperiments("enable-ai-workspace-debug");
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

const mockUploadedFile: WorkspaceFileUpload = {
	id: "upload-1",
	file: new File(["PK"], "bundle.zip", { type: "application/zip" }),
	status: "uploaded",
	response: {
		path: "/home/coder/bundle.zip",
		name: "bundle.zip",
		size: 2,
		media_type: "application/zip",
		workspace_id: "ws-1",
	},
};

const mockConflictError = {
	...mockApiError({ message: "Cannot archive an active chat." }),
	response: {
		status: 409,
		data: { message: "Cannot archive an active chat." },
	},
};

const chatPath = `/agents/${MockChat.id}`;

const renderUploadPage = async () => {
	const { router } = renderWithAuth(<AgentCreatePage />, {
		path: "/agents",
		route: "/agents",
		extraRoutes: [{ path: "/agents/:agentId", element: <div /> }],
	});
	await waitFor(() => expect(formProps.onCreateChat).toBeDefined());
	return router;
};

const submit = (options: Partial<CreateChatOptions>) => {
	const onCreateChat = formProps.onCreateChat;
	if (!onCreateChat) {
		throw new Error("AgentCreateForm was not rendered.");
	}
	return act(() =>
		onCreateChat({
			message: "inspect this archive",
			organizationId: MockDefaultOrganization.id,
			workspaceId: "ws-1",
			manageAutomationsEnabled: false,
			...options,
		}),
	);
};

describe("AgentCreatePage workspace uploads", () => {
	let events: string[];

	beforeEach(() => {
		realForm.enabled = true;
		formProps.onCreateChat = undefined;
		events = [];
		vi.spyOn(API.experimental, "createChat").mockImplementation(async () => {
			events.push("create");
			return MockChat;
		});
		vi.spyOn(API.experimental, "createChatMessage").mockImplementation(
			async () => {
				events.push("send");
				return { queued: false };
			},
		);
		vi.spyOn(API.experimental, "updateChat").mockImplementation(async () => {
			events.push("archive");
		});
		vi.spyOn(toast, "error");
	});

	it("creates an idle chat, uploads, then sends the first message", async () => {
		const router = await renderUploadPage();
		const uploadWorkspaceFiles = vi.fn(async (chatId: string) => {
			events.push(`upload:${chatId}`);
			return [mockUploadedFile];
		});

		await submit({ uploadWorkspaceFiles });

		expect(events).toEqual(["create", `upload:${MockChat.id}`, "send"]);
		expect(API.experimental.createChat).toHaveBeenCalledWith(
			expect.objectContaining({ content: [], workspace_id: "ws-1" }),
		);
		expect(API.experimental.createChatMessage).toHaveBeenCalledWith(
			MockChat.id,
			expect.objectContaining({
				content: [
					{ type: "text", text: "inspect this archive" },
					{
						type: "workspace-file-reference",
						workspace_file_path: "/home/coder/bundle.zip",
						workspace_file_name: "bundle.zip",
						workspace_file_size: 2,
						workspace_file_media_type: "application/zip",
						workspace_file_workspace_id: "ws-1",
					},
				],
			}),
		);
		expect(router.state.location.pathname).toBe(chatPath);
	});

	it("sends text-only submits with the create request", async () => {
		const router = await renderUploadPage();

		await submit({});

		expect(events).toEqual(["create"]);
		expect(API.experimental.createChat).toHaveBeenCalledWith(
			expect.objectContaining({
				content: [{ type: "text", text: "inspect this archive" }],
			}),
		);
		expect(router.state.location.pathname).toBe(chatPath);
	});

	it("sends manage_automations_enabled matching the toggle", async () => {
		await renderUploadPage();

		await submit({ manageAutomationsEnabled: false });
		await submit({ manageAutomationsEnabled: true });

		const [offRequest, onRequest] = vi
			.mocked(API.experimental.createChat)
			.mock.calls.map(([request]) => request);
		expect(offRequest).toMatchObject({ manage_automations_enabled: false });
		expect(onRequest).toMatchObject({ manage_automations_enabled: true });
	});

	it("archives the chat when the upload fails", async () => {
		const router = await renderUploadPage();
		const uploadWorkspaceFiles = vi
			.fn()
			.mockRejectedValue(mockApiError({ message: "Agent unreachable." }));

		await expect(submit({ uploadWorkspaceFiles })).rejects.toBeDefined();

		await waitFor(() => expect(events).toEqual(["create", "archive"]));
		expect(API.experimental.updateChat).toHaveBeenCalledWith(MockChat.id, {
			archived: true,
		});
		expect(toast.error).toHaveBeenCalledWith("Agent unreachable.");
		expect(router.state.location.pathname).toBe("/agents");
	});

	it("archives the chat without an upload toast when the upload is aborted", async () => {
		const router = await renderUploadPage();
		const abortError = new Error("The upload was aborted.");
		abortError.name = "AbortError";
		const uploadWorkspaceFiles = vi.fn().mockRejectedValue(abortError);

		await expect(submit({ uploadWorkspaceFiles })).rejects.toBe(abortError);

		await waitFor(() => expect(events).toEqual(["create", "archive"]));
		expect(toast.error).not.toHaveBeenCalled();
		expect(router.state.location.pathname).toBe("/agents");
	});

	it("archives the chat when an upload entry failed", async () => {
		const router = await renderUploadPage();
		const uploadWorkspaceFiles = vi.fn().mockResolvedValue([
			mockUploadedFile,
			{
				...mockUploadedFile,
				id: "upload-2",
				status: "error",
				response: undefined,
			},
		]);

		await expect(submit({ uploadWorkspaceFiles })).rejects.toBeDefined();

		await waitFor(() => expect(events).toEqual(["create", "archive"]));
		expect(toast.error).toHaveBeenCalledWith(
			"1 file failed to upload to the workspace. Remove or retry the failed files, then send again.",
		);
		expect(router.state.location.pathname).toBe("/agents");
	});

	it("reports a failed cleanup after an upload failure", async () => {
		await renderUploadPage();
		vi.mocked(API.experimental.updateChat).mockRejectedValue(
			mockApiError({ message: "Archive failed." }),
		);
		const uploadWorkspaceFiles = vi
			.fn()
			.mockRejectedValue(mockApiError({ message: "Agent unreachable." }));

		await expect(submit({ uploadWorkspaceFiles })).rejects.toBeDefined();

		await waitFor(() =>
			expect(toast.error).toHaveBeenCalledWith("Archive failed."),
		);
	});

	it("archives the chat when the first message fails", async () => {
		const router = await renderUploadPage();
		vi.mocked(API.experimental.createChatMessage).mockImplementation(
			async () => {
				events.push("send");
				throw mockApiError({ message: "Send failed." });
			},
		);
		const uploadWorkspaceFiles = vi.fn().mockResolvedValue([mockUploadedFile]);

		await expect(submit({ uploadWorkspaceFiles })).rejects.toBeDefined();

		expect(events).toEqual(["create", "send", "archive"]);
		expect(toast.error).toHaveBeenCalledTimes(1);
		expect(toast.error).toHaveBeenCalledWith("Send failed.");
		expect(router.state.location.pathname).toBe("/agents");
	});

	it("reports a failed cleanup after the first message fails", async () => {
		await renderUploadPage();
		vi.mocked(API.experimental.createChatMessage).mockRejectedValue(
			mockApiError({ message: "Send failed." }),
		);
		vi.mocked(API.experimental.updateChat).mockRejectedValue(
			mockApiError({ message: "Archive failed." }),
		);
		const uploadWorkspaceFiles = vi.fn().mockResolvedValue([mockUploadedFile]);

		await expect(submit({ uploadWorkspaceFiles })).rejects.toBeDefined();

		expect(toast.error).toHaveBeenCalledWith("Archive failed.");
		expect(toast.error).toHaveBeenCalledWith("Send failed.");
	});

	it("navigates to the chat when the failed send was committed", async () => {
		const router = await renderUploadPage();
		vi.mocked(API.experimental.createChatMessage).mockRejectedValue(
			mockApiError({ message: "Network Error" }),
		);
		vi.mocked(API.experimental.updateChat).mockRejectedValue(mockConflictError);
		const uploadWorkspaceFiles = vi.fn().mockResolvedValue([mockUploadedFile]);

		await submit({ uploadWorkspaceFiles });

		expect(toast.error).not.toHaveBeenCalled();
		expect(router.state.location.pathname).toBe(chatPath);
	});
});

describe("AgentCreatePage prompt link", () => {
	beforeEach(() => {
		realForm.enabled = true;
	});

	it("prefills the prompt from a prompt link and sends it only on Send", async () => {
		const { uploadChatFile, createChat } = mockPageQueries();
		const prompt = "Fix the flaky test\nin a & b = c";
		const user = userEvent.setup();

		renderPage(`/agents?prompt=${encodeURIComponent(prompt)}`);

		const sendButton = await findEnabledSendButton();
		expect(createChat).not.toHaveBeenCalled();
		expect(uploadChatFile).not.toHaveBeenCalled();
		// Screen readers announce the caution when focus lands in the composer.
		expect(
			screen.getByRole("textbox", { name: "Chat message" }),
		).toHaveAccessibleDescription(/^Use caution before running this prompt\./);

		await user.click(sendButton);

		await waitFor(() => expect(createChat).toHaveBeenCalledTimes(1));
		expect(createChat.mock.calls[0][0].content).toEqual([
			{ type: "text", text: prompt },
		]);
	});

	it("ignores the prompt when a debug link is also present", async () => {
		enableExperiments("enable-ai-workspace-debug");
		const { createChat } = mockPageQueries();
		const user = userEvent.setup();

		const { router } = renderPage(`${deepLink}&prompt=hi`);

		const sendButton = await findEnabledSendButton();
		expect(router.state.location.search).toBe("?archived=archived");
		// Exact match: a prompt left in state would show the link alert.
		expect(router.state.location.state).toEqual({
			debugWorkspaceBuildId: failedBuild.id,
		});
		await user.click(sendButton);

		await waitFor(() => expect(createChat).toHaveBeenCalledTimes(1));
		expect(createChat.mock.calls[0][0].content).toEqual([
			{ type: "text", text: debugWorkspaceBuildPrompt(failedBuild) },
			{ type: "file", file_id: "uploaded-logs" },
		]);
	});

	it("moves the prompt out of the URL so New chat gets a plain composer", async () => {
		const { createChat } = mockPageQueries();
		localStorage.setItem(emptyInputStorageKey, "draft the user typed earlier");
		const user = userEvent.setup();

		const { router } = renderPage("/agents?archived=archived&prompt=hi");

		const promptSendButton = await findEnabledSendButton();
		expect(router.state.location).toMatchObject({
			search: "?archived=archived",
			state: { prompt: "hi" },
		});
		// The layout's links forward location.search to a new history entry.
		await router.navigate({
			pathname: "/agents",
			search: router.state.location.search,
		});
		// Wait for the form to remount before sending from it.
		await waitFor(() =>
			expect(screen.getByRole("button", { name: "Send" })).not.toBe(
				promptSendButton,
			),
		);

		await user.click(await findEnabledSendButton());

		await waitFor(() => expect(createChat).toHaveBeenCalledTimes(1));
		expect(createChat.mock.calls[0][0].content).toEqual([
			{ type: "text", text: "draft the user typed earlier" },
		]);
	});
});
