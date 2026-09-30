import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { StrictMode } from "react";
import { QueryClient, QueryClientProvider } from "react-query";
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
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
import { ThemeOverride } from "#/contexts/ThemeProvider";
import {
	MockChatModel,
	MockChatModelProviderDescriptor,
	MockDefaultChatModel,
	MockUnsetUserChatPersonalModelOverrides,
} from "#/testHelpers/chatModels";
import {
	MockDefaultOrganization,
	MockOrganization2,
	MockUserPreferenceSettings,
	MockWorkspace,
	MockWorkspaceAgent,
} from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import themes, { DEFAULT_THEME } from "#/theme";
import { persistedAttachmentsStorageKey } from "../hooks/useFileAttachments";
import { readAgentAttachmentText } from "../utils/fileAttachmentLimits";
import {
	AgentCreateForm,
	type CreateChatOptions,
	emptyInputStorageKey,
	selectedOrganizationIdStorageKey,
	selectedWorkspaceIdStorageKey,
} from "./AgentCreateForm";

const dashboard = vi.hoisted(() => ({ showOrganizations: false }));

vi.mock("#/modules/dashboard/useDashboard", async () => {
	const { MockDefaultOrganization, MockOrganization2 } = await import(
		"#/testHelpers/entities"
	);
	return {
		useDashboard: () => ({
			organizations: [MockDefaultOrganization, MockOrganization2],
			showOrganizations: dashboard.showOrganizations,
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

const renderForm = (props: Partial<FormProps> = {}) => {
	const queryClient = createQueryClient();
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
	const { rerender } = render(renderTree({}));
	return {
		onCreateChat,
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

type WrapperProps = React.PropsWithChildren<{
	queryClient?: ReturnType<typeof createTestQueryClient>;
}>;

const Wrapper: React.FC<WrapperProps> = ({
	children,
	queryClient = createTestQueryClient(),
}) => {
	return (
		<QueryClientProvider client={queryClient}>
			<ThemeOverride theme={themes[DEFAULT_THEME]}>
				<TooltipProvider>
					<MemoryRouter>{children}</MemoryRouter>
				</TooltipProvider>
			</ThemeOverride>
		</QueryClientProvider>
	);
};

const formProps = {
	onCreateChat: vi.fn(),
	isCreating: false,
	createError: undefined,
	canCreateChat: true,
	canConfigureAgentSetup: false,
	aiGatewayDisabled: false,
	workspaceCount: 0,
	workspaceOptions: [],
	workspacesError: undefined,
	isWorkspacesLoading: false,
};

const chatCreatePermission = {
	object: { resource_type: "chat", owner_id: "me" },
	action: "create",
} as const;

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

describe("AgentCreateForm", () => {
	beforeEach(() => {
		dashboard.showOrganizations = true;
	});

	it("keeps the remembered organization and workspace while a project locks another", async () => {
		localStorage.setItem(
			selectedOrganizationIdStorageKey,
			MockDefaultOrganization.id,
		);
		localStorage.setItem(selectedWorkspaceIdStorageKey, "ws-default-org");
		const mcpRequests: string[] = [];
		server.use(
			http.get("/api/v2/organizations", () =>
				HttpResponse.json([MockDefaultOrganization, MockOrganization2]),
			),
			http.post("/api/v2/authcheck", async ({ request }) => {
				const { checks } = (await request.json()) as {
					checks: Record<string, unknown>;
				};
				return HttpResponse.json(
					Object.fromEntries(Object.keys(checks).map((key) => [key, true])),
				);
			}),
			http.get(
				"/api/v2/organizations/:organization/mcp-servers",
				({ params }) => {
					mcpRequests.push(String(params.organization));
					return HttpResponse.json([]);
				},
			),
		);

		const { rerender } = render(
			<Wrapper>
				<AgentCreateForm {...formProps} />
			</Wrapper>,
		);
		await waitFor(() => {
			expect(mcpRequests).toContain(MockDefaultOrganization.id);
		});
		// The same form instance survives navigating between the plain composer
		// and a cached project page, as it does under the router.
		rerender(
			<Wrapper>
				<AgentCreateForm
					{...formProps}
					lockedOrganizationId={MockOrganization2.id}
				/>
			</Wrapper>,
		);
		await waitFor(() => {
			expect(mcpRequests).toContain(MockOrganization2.id);
		});
		expect(localStorage.getItem(selectedWorkspaceIdStorageKey)).toBe(
			"ws-default-org",
		);

		mcpRequests.length = 0;
		rerender(
			<Wrapper>
				<AgentCreateForm {...formProps} />
			</Wrapper>,
		);

		await waitFor(() => {
			expect(mcpRequests).toContain(MockDefaultOrganization.id);
		});
		expect(localStorage.getItem(selectedOrganizationIdStorageKey)).toBe(
			MockDefaultOrganization.id,
		);
		expect(localStorage.getItem(selectedWorkspaceIdStorageKey)).toBe(
			"ws-default-org",
		);
	});

	it("clears a revoked workspace after leaving a lock on the same organization", async () => {
		localStorage.setItem(
			selectedOrganizationIdStorageKey,
			MockOrganization2.id,
		);
		localStorage.setItem(selectedWorkspaceIdStorageKey, "ws-org2");
		let permittedOrganizationIds = new Set([
			MockDefaultOrganization.id,
			MockOrganization2.id,
		]);
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
							permittedOrganizationIds.has(key),
						]),
					),
				);
			}),
		);
		const queryClient = createTestQueryClient();
		const { rerender } = render(
			<Wrapper queryClient={queryClient}>
				<AgentCreateForm
					{...formProps}
					lockedOrganizationId={MockOrganization2.id}
				/>
			</Wrapper>,
		);
		await waitFor(() => {
			expect(
				queryClient.getQueryData(
					permittedOrganizationsKey(chatCreatePermission),
				),
			).toBeDefined();
		});

		rerender(
			<Wrapper queryClient={queryClient}>
				<AgentCreateForm {...formProps} />
			</Wrapper>,
		);
		permittedOrganizationIds = new Set([MockDefaultOrganization.id]);
		await queryClient.invalidateQueries({
			queryKey: permittedOrganizationsKey(chatCreatePermission),
		});

		await waitFor(() => {
			expect(localStorage.getItem(selectedWorkspaceIdStorageKey)).toBeNull();
		});
	});

	it("denies access when the locked organization is not permitted", async () => {
		localStorage.setItem(selectedWorkspaceIdStorageKey, "ws-default-org");
		const persistedAttachments = JSON.stringify([
			{
				fileId: "persisted-file",
				fileName: "notes.txt",
				fileType: "text/plain",
				lastModified: 1000,
				organizationId: MockDefaultOrganization.id,
			},
		]);
		localStorage.setItem(persistedAttachmentsStorageKey, persistedAttachments);
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
					{...formProps}
					lockedOrganizationId={MockOrganization2.id}
				/>
			</Wrapper>,
		);

		await screen.findByText(
			"You don't have permission to create chats in this project's organization.",
		);
		expect(localStorage.getItem(selectedWorkspaceIdStorageKey)).toBe(
			"ws-default-org",
		);
		expect(localStorage.getItem(persistedAttachmentsStorageKey)).toBe(
			persistedAttachments,
		);
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
