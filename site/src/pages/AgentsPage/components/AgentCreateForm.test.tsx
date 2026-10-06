import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { StrictMode } from "react";
import { QueryClient } from "react-query";
import { MemoryRouter } from "react-router";
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
import { AppProviders } from "#/App";
import { API } from "#/api/api";
import {
	mcpServerConfigsKey,
	organizationChatModelsKey,
	userChatPersonalModelOverrides,
} from "#/api/queries/chats";
import { permittedOrganizationsKey } from "#/api/queries/organizations";
import { preferenceSettingsKey } from "#/api/queries/users";
import type * as TypesGen from "#/api/typesGenerated";
import { MockMCPServerConfig } from "#/testHelpers/chatEntities";
import {
	MockChatModel,
	MockChatModelProviderDescriptor,
	MockDefaultChatModel,
	MockUnsetUserChatPersonalModelOverrides,
} from "#/testHelpers/chatModels";
import { createDeferred } from "#/testHelpers/deferred";
import {
	MockDefaultOrganization,
	MockOrganization2,
	MockUserPreferenceSettings,
	MockWorkspace,
	MockWorkspaceAgent,
} from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import { persistedAttachmentsStorageKey } from "../hooks/useFileAttachments";
import { readAgentAttachmentText } from "../utils/fileAttachmentLimits";
import { mcpSelectionStorageKey } from "../utils/mcpSelection";
import {
	AgentCreateForm,
	type CreateChatOptions,
	draftStorageKeys,
	emptyInputStorageKey,
	selectedOrganizationIdStorageKey,
	selectedWorkspaceIdStorageKey,
} from "./AgentCreateForm";

const dashboard = vi.hoisted(
	(): { showOrganizations: boolean; experiments: string[] } => ({
		showOrganizations: false,
		experiments: [],
	}),
);

vi.mock("#/modules/dashboard/useDashboard", async () => {
	const { MockDefaultOrganization, MockOrganization2 } = await import(
		"#/testHelpers/entities"
	);
	return {
		useDashboard: () => ({
			organizations: [MockDefaultOrganization, MockOrganization2],
			showOrganizations: dashboard.showOrganizations,
			experiments: dashboard.experiments,
		}),
	};
});

const workspaceUploadUnavailableMessage =
	"This file type is uploaded into the chat's workspace. Select a running workspace, then try again.";
const removedQueuedFileMessage = "Removed 1 file that uploads to the workspace";
const attachDuringSubmitMessage =
	"Wait for the current message to finish sending, then add the file again.";

const mockModelCatalog: TypesGen.OrganizationChatModelsResponse = {
	models: [
		{
			...MockChatModel,
			organization_id: MockDefaultOrganization.id,
			is_default: true,
		},
	],
	providers: [MockChatModelProviderDescriptor],
	unsupported_providers: [],
};

const mockPersonalModelOverrides: TypesGen.UserChatPersonalModelOverridesResponse =
	{
		enabled: false,
		root: {
			context: "root",
			mode: "chat_default",
			model_config_id: "",
			is_set: false,
		},
		general: {
			context: "general",
			mode: "deployment_default",
			model_config_id: "",
			is_set: false,
		},
		explore: {
			context: "explore",
			mode: "deployment_default",
			model_config_id: "",
			is_set: false,
		},
		deployment_defaults: {
			general: { context: "general", model_config_id: "" },
			explore: { context: "explore", model_config_id: "" },
		},
	};

const mockWorkspace: TypesGen.Workspace = {
	...MockWorkspace,
	id: "ws-1",
	name: "my-project",
};

const mockDisconnectedWorkspace: TypesGen.Workspace = {
	...mockWorkspace,
	latest_build: {
		...mockWorkspace.latest_build,
		resources: [
			{
				...mockWorkspace.latest_build.resources[0],
				agents: [{ ...MockWorkspaceAgent, status: "disconnected" }],
			},
		],
	},
};

const mockStoppedWorkspace: TypesGen.Workspace = {
	...mockDisconnectedWorkspace,
	id: "ws-stopped",
	name: "stopped-project",
	latest_build: {
		...mockDisconnectedWorkspace.latest_build,
		status: "stopped",
	},
};

const createQueryClient = () => {
	const queryClient = new QueryClient({
		defaultOptions: {
			queries: { retry: false, staleTime: Number.POSITIVE_INFINITY },
		},
	});
	queryClient.setQueryData(
		organizationChatModelsKey(MockDefaultOrganization.id),
		mockModelCatalog,
	);
	queryClient.setQueryData(
		userChatPersonalModelOverrides(MockDefaultOrganization.id).queryKey,
		mockPersonalModelOverrides,
	);
	queryClient.setQueryData(mcpServerConfigsKey(MockDefaultOrganization.id), []);
	queryClient.setQueryData(preferenceSettingsKey, MockUserPreferenceSettings);
	queryClient.setQueryData(
		permittedOrganizationsKey({
			object: { resource_type: "chat", owner_id: "me" },
			action: "create",
		}),
		[MockDefaultOrganization, MockOrganization2],
	);
	return queryClient;
};

type FormProps = React.ComponentProps<typeof AgentCreateForm>;

const renderForm = (
	props: Partial<FormProps> = {},
	{ queryClient = createQueryClient() } = {},
) => {
	const onCreateChat = vi
		.fn<FormProps["onCreateChat"]>()
		.mockResolvedValue(undefined);
	const renderTree = (overrides: Partial<FormProps>) => (
		<AppProviders queryClient={queryClient}>
			<MemoryRouter>
				<AgentCreateForm
					onCreateChat={onCreateChat}
					isCreating={false}
					createError={undefined}
					canCreateChat
					canConfigureAgentSetup={false}
					workspaceCount={1}
					workspaceOptions={[mockWorkspace]}
					workspacesError={undefined}
					isWorkspacesLoading={false}
					{...props}
					{...overrides}
				/>
			</MemoryRouter>
		</AppProviders>
	);
	const { rerender, unmount } = render(renderTree({}));
	return {
		onCreateChat,
		unmount,
		rerender: (overrides: Partial<FormProps>) =>
			rerender(renderTree(overrides)),
	};
};

// applyAccept stays on so these tests fail if the picker's accept
// attribute filters out workspace-routed types.
const user = () => userEvent.setup({ applyAccept: true });

const attachZipFile = async () => {
	const zip = new File([new Uint8Array([0x50, 0x4b, 3, 4])], "bundle.zip", {
		type: "application/zip",
	});
	await user().upload(screen.getByTestId("chat-attachment-file-input"), zip);
};

const attachImageFile = async () => {
	const image = new File(["png"], "image.png", { type: "image/png" });
	await user().upload(screen.getByTestId("chat-attachment-file-input"), image);
};

const typeMessage = async (message: string) => {
	await user().click(screen.getByRole("textbox", { name: "Chat message" }));
	await user().paste(message);
};

const clickSend = async () => {
	const sendButton = screen.getByRole("button", { name: "Send" });
	await waitFor(() => expect(sendButton).toHaveProperty("disabled", false));
	await user().click(sendButton);
};

const submitMessage = async (message: string) => {
	await typeMessage(message);
	await clickSend();
};

const submittedOptions = (
	onCreateChat: ReturnType<typeof renderForm>["onCreateChat"],
): CreateChatOptions => {
	const options = onCreateChat.mock.calls[0]?.[0];
	if (!options) {
		throw new Error("Expected onCreateChat to be called.");
	}
	return options;
};

beforeAll(() => {
	Object.defineProperty(Range.prototype, "getBoundingClientRect", {
		configurable: true,
		value: () => new DOMRect(0, 0, 1, 16),
	});
});

describe("AgentCreateForm workspace file uploads", () => {
	beforeEach(() => {
		localStorage.clear();
		dashboard.showOrganizations = false;
		vi.spyOn(toast, "error");
		vi.spyOn(toast, "warning");
	});

	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("submits queued workspace files with an upload callback", async () => {
		localStorage.setItem("agents.selected-workspace-id", mockWorkspace.id);
		const { onCreateChat } = renderForm();

		await attachZipFile();
		await submitMessage("inspect this archive");

		await waitFor(() => expect(onCreateChat).toHaveBeenCalledTimes(1));
		expect(submittedOptions(onCreateChat)).toMatchObject({
			message: "inspect this archive",
			workspaceId: mockWorkspace.id,
			uploadWorkspaceFiles: expect.any(Function),
		});
	});

	it("omits the upload callback for text-only submits", async () => {
		localStorage.setItem("agents.selected-workspace-id", mockWorkspace.id);
		const { onCreateChat } = renderForm();

		await submitMessage("plain text message");

		await waitFor(() => expect(onCreateChat).toHaveBeenCalledTimes(1));
		expect(submittedOptions(onCreateChat).uploadWorkspaceFiles).toBeUndefined();
	});

	it("rejects workspace files when no workspace is selected", async () => {
		const { onCreateChat } = renderForm();

		await attachZipFile();
		await submitMessage("inspect this archive");

		expect(toast.error).toHaveBeenCalledWith(workspaceUploadUnavailableMessage);
		await waitFor(() => expect(onCreateChat).toHaveBeenCalledTimes(1));
		expect(submittedOptions(onCreateChat).uploadWorkspaceFiles).toBeUndefined();
	});

	it("rejects workspace files when the selected agent is disconnected", async () => {
		localStorage.setItem(
			"agents.selected-workspace-id",
			mockStoppedWorkspace.id,
		);
		const { onCreateChat } = renderForm({
			workspaceOptions: [mockStoppedWorkspace],
		});

		await attachZipFile();
		await submitMessage("inspect this archive");

		expect(toast.error).toHaveBeenCalledWith(workspaceUploadUnavailableMessage);
		await waitFor(() => expect(onCreateChat).toHaveBeenCalledTimes(1));
		expect(submittedOptions(onCreateChat).uploadWorkspaceFiles).toBeUndefined();
	});

	it("locks the scope controls while the upload submit is pending", async () => {
		dashboard.showOrganizations = true;
		localStorage.setItem("agents.selected-workspace-id", mockWorkspace.id);
		const { onCreateChat } = renderForm();
		onCreateChat.mockReturnValue(new Promise<void>(() => {}));

		await attachZipFile();
		await submitMessage("inspect this archive");

		await waitFor(() => expect(onCreateChat).toHaveBeenCalledTimes(1));
		expect(
			screen.getByRole("button", { name: /^Organization:/ }),
		).toHaveProperty("disabled", true);
		expect(screen.getByRole("button", { name: "More options" })).toHaveProperty(
			"disabled",
			true,
		);
	});

	it("ignores further submits after the chat was created", async () => {
		localStorage.setItem("agents.selected-workspace-id", mockWorkspace.id);
		const { onCreateChat } = renderForm();

		await attachZipFile();
		await submitMessage("inspect this archive");
		await waitFor(() => expect(onCreateChat).toHaveBeenCalledTimes(1));
		// The page navigates after onCreateChat resolves; until then the
		// retained draft must not create a second chat.
		await user().click(screen.getByRole("button", { name: "Send" }));

		expect(onCreateChat).toHaveBeenCalledTimes(1);
	});

	it("rejects attachments while the submit is pending", async () => {
		localStorage.setItem("agents.selected-workspace-id", mockWorkspace.id);
		const { onCreateChat } = renderForm();
		onCreateChat.mockReturnValue(new Promise<void>(() => {}));

		await attachZipFile();
		await submitMessage("inspect this archive");
		await waitFor(() => expect(onCreateChat).toHaveBeenCalledTimes(1));
		await attachImageFile();

		expect(toast.error).toHaveBeenCalledWith(attachDuringSubmitMessage);
	});

	it("rejects pasted files while the submit is pending", async () => {
		localStorage.setItem("agents.selected-workspace-id", mockWorkspace.id);
		const { onCreateChat } = renderForm();
		onCreateChat.mockReturnValue(new Promise<void>(() => {}));

		await attachZipFile();
		await submitMessage("inspect this archive");
		await waitFor(() => expect(onCreateChat).toHaveBeenCalledTimes(1));
		fireEvent.paste(screen.getByRole("textbox", { name: "Chat message" }), {
			clipboardData: {
				files: [new File(["png"], "image.png", { type: "image/png" })],
				types: ["Files"],
				getData: () => "",
			},
		});

		expect(toast.error).toHaveBeenCalledWith(attachDuringSubmitMessage);
	});

	it("rejects workspace files after the chat was created", async () => {
		localStorage.setItem("agents.selected-workspace-id", mockWorkspace.id);
		const { onCreateChat } = renderForm();

		await attachZipFile();
		await submitMessage("inspect this archive");
		await waitFor(() => expect(onCreateChat).toHaveBeenCalledTimes(1));
		await attachZipFile();

		expect(toast.error).toHaveBeenCalledWith(attachDuringSubmitMessage);
	});

	it("drops queued files when the workspace is detached", async () => {
		localStorage.setItem("agents.selected-workspace-id", mockWorkspace.id);
		const { onCreateChat } = renderForm();

		await attachZipFile();
		await user().click(
			screen.getByRole("button", { name: "Remove workspace my-project" }),
		);

		await waitFor(() =>
			expect(toast.warning).toHaveBeenCalledWith(removedQueuedFileMessage),
		);
		await submitMessage("plain text message");
		await waitFor(() => expect(onCreateChat).toHaveBeenCalledTimes(1));
		expect(submittedOptions(onCreateChat).uploadWorkspaceFiles).toBeUndefined();
	});

	it("drops queued files when switching to a stopped workspace", async () => {
		localStorage.setItem("agents.selected-workspace-id", mockWorkspace.id);
		renderForm({
			workspaceCount: 2,
			workspaceOptions: [mockWorkspace, mockStoppedWorkspace],
		});

		await attachZipFile();
		await user().click(screen.getByRole("button", { name: "More options" }));
		await user().click(
			await screen.findByRole("button", { name: /Attach workspace/ }),
		);
		await user().click(await screen.findByText("stopped-project"));

		await waitFor(() =>
			expect(toast.warning).toHaveBeenCalledWith(removedQueuedFileMessage),
		);
	});

	it("keeps queued files when an organization change is cancelled", async () => {
		dashboard.showOrganizations = true;
		localStorage.setItem("agents.selected-workspace-id", mockWorkspace.id);
		const { onCreateChat } = renderForm();

		await attachZipFile();
		await user().click(screen.getByRole("button", { name: /^Organization:/ }));
		await user().click(await screen.findByText("My Organization 2"));
		await user().click(await screen.findByRole("button", { name: /cancel/i }));
		await submitMessage("inspect this archive");

		await waitFor(() => expect(onCreateChat).toHaveBeenCalledTimes(1));
		expect(submittedOptions(onCreateChat)).toMatchObject({
			organizationId: MockDefaultOrganization.id,
			workspaceId: mockWorkspace.id,
			uploadWorkspaceFiles: expect.any(Function),
		});
	});

	it("keeps queued files across an agent status flap on the same workspace", async () => {
		localStorage.setItem("agents.selected-workspace-id", mockWorkspace.id);
		const { onCreateChat, rerender } = renderForm();

		await attachZipFile();
		rerender({ workspaceOptions: [mockDisconnectedWorkspace] });
		await submitMessage("inspect this archive");

		expect(toast.error).toHaveBeenCalledWith(workspaceUploadUnavailableMessage);
		expect(onCreateChat).not.toHaveBeenCalled();

		rerender({ workspaceOptions: [mockWorkspace] });
		await clickSend();

		await waitFor(() => expect(onCreateChat).toHaveBeenCalledTimes(1));
		expect(submittedOptions(onCreateChat).uploadWorkspaceFiles).toEqual(
			expect.any(Function),
		);
		expect(toast.warning).not.toHaveBeenCalledWith(removedQueuedFileMessage);
	});
});

const userDraftAttachments = JSON.stringify([
	{
		fileId: "user-draft-file",
		fileName: "notes.txt",
		fileType: "text/plain",
		lastModified: 1000,
		organizationId: MockDefaultOrganization.id,
	},
]);

afterEach(() => {
	vi.restoreAllMocks();
	localStorage.clear();
	dashboard.showOrganizations = false;
});

const mockChatCreatePermissions = (permitted: (id: string) => boolean) =>
	http.post("/api/v2/authcheck", async ({ request }) => {
		const { checks } = (await request.json()) as {
			checks: Record<string, unknown>;
		};
		return HttpResponse.json(
			Object.fromEntries(
				Object.keys(checks).map((key) => [key, permitted(key)]),
			),
		);
	});

describe("AgentCreateForm organization lock", () => {
	beforeEach(() => {
		dashboard.showOrganizations = true;
		server.use(
			http.get("/api/v2/organizations", () =>
				HttpResponse.json([MockDefaultOrganization, MockOrganization2]),
			),
		);
	});

	const projectInOrg2 = {
		id: "project-1",
		organization_id: MockOrganization2.id,
	};

	// The default renderForm client pre-seeds permissions; these tests serve
	// their own through MSW.
	const renderLockTest = (props: Partial<FormProps>) =>
		renderForm(props, { queryClient: createTestQueryClient() });

	const recordMCPRequests = () => {
		const requests: string[] = [];
		server.use(
			http.get(
				"/api/v2/organizations/:organization/mcp-servers",
				({ params }) => {
					requests.push(String(params.organization));
					return HttpResponse.json([]);
				},
			),
		);
		return requests;
	};

	it("saves a workspace picked in the plain composer", async () => {
		renderForm();

		await user().click(screen.getByRole("button", { name: "More options" }));
		await user().click(
			(await screen.findByText("Attach workspace")).closest("button")!,
		);
		await user().click(
			await screen.findByRole("option", {
				name: new RegExp(mockWorkspace.name),
			}),
		);

		await waitFor(() => {
			expect(localStorage.getItem(selectedWorkspaceIdStorageKey)).toBe(
				mockWorkspace.id,
			);
		});
	});

	it("submits the project's ID", async () => {
		const { onCreateChat } = renderForm({
			project: { id: "project-1", organization_id: MockDefaultOrganization.id },
		});

		await submitMessage("hello project");

		await waitFor(() => expect(onCreateChat).toHaveBeenCalledTimes(1));
		expect(submittedOptions(onCreateChat).projectId).toBe("project-1");
	});

	it("does not save the user's defaults from choices made in a project", async () => {
		localStorage.setItem(
			selectedOrganizationIdStorageKey,
			MockDefaultOrganization.id,
		);
		localStorage.setItem(selectedWorkspaceIdStorageKey, "ws-default-org");
		server.use(mockChatCreatePermissions(() => true));
		const mcpRequests = recordMCPRequests();
		const projectWorkspace: TypesGen.Workspace = {
			...mockWorkspace,
			id: "ws-org2",
			name: "project-workspace",
			organization_id: MockOrganization2.id,
		};

		const { unmount } = renderLockTest({
			workspaceOptions: [projectWorkspace],
			project: projectInOrg2,
		});
		await waitFor(() => {
			expect(mcpRequests).toContain(MockOrganization2.id);
		});
		await user().click(screen.getByRole("button", { name: "More options" }));
		await user().click(
			(await screen.findByText("Attach workspace")).closest("button")!,
		);
		await user().click(
			await screen.findByRole("option", { name: /project-workspace/ }),
		);

		expect(localStorage.getItem(selectedOrganizationIdStorageKey)).toBe(
			MockDefaultOrganization.id,
		);
		expect(localStorage.getItem(selectedWorkspaceIdStorageKey)).toBe(
			"ws-default-org",
		);
		unmount();
	});

	it("does not adopt the project's organization as the user's default", async () => {
		server.use(mockChatCreatePermissions(() => true));
		const mcpRequests = recordMCPRequests();

		const { unmount } = renderLockTest({ project: projectInOrg2 });
		await waitFor(() => {
			expect(mcpRequests).toContain(MockOrganization2.id);
		});
		expect(localStorage.getItem(selectedOrganizationIdStorageKey)).toBeNull();
		unmount();

		renderLockTest({});
		await waitFor(() => {
			expect(localStorage.getItem(selectedOrganizationIdStorageKey)).toBe(
				MockDefaultOrganization.id,
			);
		});
	});

	it("does not save an MCP choice made in a project", async () => {
		const queryClient = createQueryClient();
		queryClient.setQueryData(mcpServerConfigsKey(MockDefaultOrganization.id), [
			{
				...MockMCPServerConfig,
				id: "mcp-notion",
				display_name: "Notion",
				availability: "default_off",
			},
		]);

		renderForm(
			{
				project: {
					id: "project-1",
					organization_id: MockDefaultOrganization.id,
				},
			},
			{ queryClient },
		);
		await user().click(screen.getByRole("button", { name: "More options" }));
		await user().click(
			await screen.findByRole("switch", { name: "Enable Notion" }),
		);

		await screen.findByRole("switch", { name: "Disable Notion" });
		expect(
			localStorage.getItem(mcpSelectionStorageKey(MockDefaultOrganization.id)),
		).toBeNull();
	});

	it("starts a fresh draft when the project changes", async () => {
		const { rerender } = renderForm({
			project: { id: "project-a", organization_id: MockDefaultOrganization.id },
		});
		await typeMessage("draft for A");
		await waitFor(() => {
			expect(
				localStorage.getItem(draftStorageKeys("project-a").text),
			).toContain("draft for A");
		});
		const draftA = localStorage.getItem(draftStorageKeys("project-a").text);

		rerender({
			project: { id: "project-b", organization_id: MockDefaultOrganization.id },
		});

		await waitFor(() => {
			expect(
				screen.getByRole("textbox", { name: "Chat message" }),
			).not.toHaveTextContent("draft for A");
		});
		expect(localStorage.getItem(draftStorageKeys("project-a").text)).toBe(
			draftA,
		);
	});

	it("keeps text and attachment drafts separate per project", async () => {
		localStorage.setItem(emptyInputStorageKey, "plain composer draft");
		localStorage.setItem(
			draftStorageKeys("project-2").text,
			"other project draft",
		);
		localStorage.setItem(persistedAttachmentsStorageKey, userDraftAttachments);

		renderForm({
			project: { id: "project-1", organization_id: MockDefaultOrganization.id },
		});
		await typeMessage("project draft");

		await waitFor(() => {
			expect(
				localStorage.getItem(draftStorageKeys("project-1").text),
			).toContain("project draft");
		});
		// Send enables only after the attachment state is adopted, so a draft
		// restored from the wrong key would be showing by now.
		await waitFor(() =>
			expect(screen.getByRole("button", { name: "Send" })).toBeEnabled(),
		);
		expect(screen.queryByText("plain composer draft")).toBeNull();
		expect(screen.queryByText("other project draft")).toBeNull();
		expect(
			screen.queryByRole("button", { name: "Remove notes.txt" }),
		).toBeNull();
		expect(localStorage.getItem(emptyInputStorageKey)).toBe(
			"plain composer draft",
		);
		expect(localStorage.getItem(persistedAttachmentsStorageKey)).toBe(
			userDraftAttachments,
		);
	});

	it("names the project's organization when the user cannot create chats in it", async () => {
		server.use(
			mockChatCreatePermissions((id) => id === MockDefaultOrganization.id),
		);

		renderLockTest({ project: projectInOrg2 });

		await screen.findByText(
			/create chats in the My Organization 2 organization, which this project belongs to\./,
		);
	});

	it("names no organization when the dashboard does not list the project's", async () => {
		server.use(
			mockChatCreatePermissions((id) => id === MockDefaultOrganization.id),
		);

		renderLockTest({
			project: { id: "project-1", organization_id: "unlisted-org" },
		});

		await screen.findByText(
			/create chats in the organization, which this project belongs to\./,
		);
	});

	it("shows the product-wide denial when the user cannot create chats at all", async () => {
		server.use(
			mockChatCreatePermissions((id) => id === MockDefaultOrganization.id),
		);

		renderLockTest({ canCreateChat: false, project: projectInOrg2 });

		await screen.findByText(/You don't have permission to use Coder Agents\./);
		expect(screen.queryByText(/which this project belongs to/)).toBeNull();
	});

	it("shows the product-wide denial when no organization is permitted", async () => {
		server.use(mockChatCreatePermissions(() => false));

		renderLockTest({ project: projectInOrg2 });

		await screen.findByText(/You don't have permission to use Coder Agents\./);
		expect(screen.queryByText(/which this project belongs to/)).toBeNull();
	});

	it("does not deny access while permissions load", async () => {
		const permissions = createDeferred<undefined>();
		server.use(
			http.post("/api/v2/authcheck", async ({ request }) => {
				await permissions.promise;
				const { checks } = (await request.json()) as {
					checks: Record<string, unknown>;
				};
				return HttpResponse.json(
					Object.fromEntries(Object.keys(checks).map((key) => [key, true])),
				);
			}),
		);
		const mcpRequests = recordMCPRequests();

		renderLockTest({ project: projectInOrg2 });
		await screen.findByRole("textbox", { name: "Chat message" });
		expect(screen.queryByText("Permission required")).toBeNull();

		permissions.resolve(undefined);
		await waitFor(() => {
			expect(mcpRequests).toContain(MockOrganization2.id);
		});
		expect(screen.queryByText("Permission required")).toBeNull();
	});
});

describe("AgentCreateForm manage automations toggle", () => {
	beforeEach(() => {
		localStorage.clear();
		dashboard.showOrganizations = false;
		dashboard.experiments = [];
	});

	it("sends the enabled toggle with the create options", async () => {
		dashboard.experiments = ["chat-automations"];
		const { onCreateChat } = renderForm();

		await user().click(screen.getByRole("button", { name: "More options" }));
		await user().click(
			await screen.findByRole("menuitemcheckbox", {
				name: "Manage automations",
			}),
		);
		await submitMessage("check the nightly build every morning");

		await waitFor(() => expect(onCreateChat).toHaveBeenCalledTimes(1));
		expect(submittedOptions(onCreateChat).manageAutomationsEnabled).toBe(true);
	});

	it("sends the toggle as off once the chat-automations experiment turns off", async () => {
		dashboard.experiments = ["chat-automations"];
		const { onCreateChat, rerender } = renderForm();
		await user().click(screen.getByRole("button", { name: "More options" }));
		await user().click(
			await screen.findByRole("menuitemcheckbox", {
				name: "Manage automations",
			}),
		);
		await user().keyboard("{Escape}");

		dashboard.experiments = [];
		rerender({});
		await submitMessage("check the nightly build every morning");

		await waitFor(() => expect(onCreateChat).toHaveBeenCalledTimes(1));
		expect(submittedOptions(onCreateChat).manageAutomationsEnabled).toBe(false);
	});
});

describe("AgentCreateForm prefill", () => {
	it("uploads the attachment once and sends it with the message, leaving the draft alone", async () => {
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
		vi.spyOn(API, "getUserPreferenceSettings").mockResolvedValue(
			MockUserPreferenceSettings,
		);
		const uploadChatFile = vi
			.spyOn(API.experimental, "uploadChatFile")
			.mockResolvedValue({ id: "uploaded-logs" });
		localStorage.setItem(emptyInputStorageKey, "draft the user typed earlier");
		localStorage.setItem(persistedAttachmentsStorageKey, userDraftAttachments);
		const onCreateChat = vi.fn().mockResolvedValue(undefined);
		const user = userEvent.setup();

		render(
			<StrictMode>
				<AppProviders queryClient={createTestQueryClient()}>
					<AgentCreateForm
						onCreateChat={onCreateChat}
						isCreating={false}
						createError={undefined}
						canCreateChat
						canConfigureAgentSetup={false}
						workspaceCount={0}
						workspaceOptions={[]}
						workspacesError={undefined}
						isWorkspacesLoading={false}
						prefill={{
							message: "Why did this build fail?",
							attachment: {
								name: "workspace-build-logs.txt",
								text: "Error: exit status 1\n",
							},
						}}
					/>
				</AppProviders>
			</StrictMode>,
		);

		const sendButton = await screen.findByRole("button", { name: "Send" });
		await waitFor(() => expect(sendButton).toBeEnabled());
		expect(uploadChatFile).toHaveBeenCalledTimes(1);
		const [uploadedFile, uploadOrganizationId] = uploadChatFile.mock.calls[0];
		expect(uploadedFile.name).toBe("workspace-build-logs.txt");
		expect(await readAgentAttachmentText(uploadedFile)).toBe(
			"Error: exit status 1\n",
		);
		expect(uploadOrganizationId).toBe(MockDefaultOrganization.id);
		expect(onCreateChat).not.toHaveBeenCalled();

		await user.click(sendButton);

		await waitFor(() => expect(onCreateChat).toHaveBeenCalledTimes(1));
		expect(onCreateChat).toHaveBeenCalledWith(
			expect.objectContaining({
				message: "Why did this build fail?",
				fileIDs: ["uploaded-logs"],
				organizationId: MockDefaultOrganization.id,
			}),
		);
		expect(localStorage.getItem(emptyInputStorageKey)).toBe(
			"draft the user typed earlier",
		);
		expect(localStorage.getItem(persistedAttachmentsStorageKey)).toBe(
			userDraftAttachments,
		);
	});
});
