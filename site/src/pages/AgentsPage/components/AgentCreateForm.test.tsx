import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { type ComponentProps, StrictMode } from "react";
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
import { persistedAttachmentsStorageKey } from "../hooks/useFileAttachments";
import { readAgentAttachmentText } from "../utils/fileAttachmentLimits";
import {
	AgentCreateForm,
	type CreateChatOptions,
	emptyInputStorageKey,
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

const mainAgent: TypesGen.WorkspaceAgent = {
	...MockWorkspaceAgent,
	id: "agent-main",
	name: "main",
	status: "connected",
};

const chatAgent: TypesGen.WorkspaceAgent = {
	...MockWorkspaceAgent,
	id: "agent-chat",
	name: "dev-coderd-chat",
	status: "disconnected",
};

const mockMultiAgentWorkspace: TypesGen.Workspace = {
	...mockWorkspace,
	id: "ws-multi",
	name: "multi-agent-project",
	latest_build: {
		...mockWorkspace.latest_build,
		id: "build-multi",
		resources: [
			{
				...mockWorkspace.latest_build.resources[0],
				agents: [mainAgent, chatAgent],
			},
		],
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

type FormProps = ComponentProps<typeof AgentCreateForm>;

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
		vi.spyOn(API.experimental, "getChatWorkspaceAgent").mockResolvedValue({
			agent_id: MockWorkspaceAgent.id,
		});
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

	it("rejects workspace files when the server-selected agent is disconnected", async () => {
		vi.mocked(API.experimental.getChatWorkspaceAgent).mockResolvedValue({
			agent_id: chatAgent.id,
		});
		localStorage.setItem(
			"agents.selected-workspace-id",
			mockMultiAgentWorkspace.id,
		);
		const { onCreateChat } = renderForm({
			workspaceOptions: [mockMultiAgentWorkspace],
		});

		await waitFor(() =>
			expect(API.experimental.getChatWorkspaceAgent).toHaveBeenCalledWith(
				mockMultiAgentWorkspace.id,
			),
		);
		await attachZipFile();
		await submitMessage("inspect this archive");

		expect(toast.error).toHaveBeenCalledWith(workspaceUploadUnavailableMessage);
		await waitFor(() => expect(onCreateChat).toHaveBeenCalledTimes(1));
		expect(submittedOptions(onCreateChat).uploadWorkspaceFiles).toBeUndefined();
	});

	it("accepts workspace files when the server-selected agent is connected", async () => {
		vi.mocked(API.experimental.getChatWorkspaceAgent).mockResolvedValue({
			agent_id: mainAgent.id,
		});
		localStorage.setItem(
			"agents.selected-workspace-id",
			mockMultiAgentWorkspace.id,
		);
		const { onCreateChat } = renderForm({
			workspaceOptions: [
				{
					...mockMultiAgentWorkspace,
					latest_build: {
						...mockMultiAgentWorkspace.latest_build,
						resources: [
							{
								...mockMultiAgentWorkspace.latest_build.resources[0],
								agents: [mainAgent, { ...chatAgent, name: "sidecar" }],
							},
						],
					},
				},
			],
		});

		await waitFor(() =>
			expect(API.experimental.getChatWorkspaceAgent).toHaveBeenCalled(),
		);
		await attachZipFile();
		await submitMessage("inspect this archive");

		await waitFor(() => expect(onCreateChat).toHaveBeenCalledTimes(1));
		expect(submittedOptions(onCreateChat).uploadWorkspaceFiles).toEqual(
			expect.any(Function),
		);
		expect(toast.error).not.toHaveBeenCalled();
	});

	it("keeps queued files while the new workspace's agent selection loads", async () => {
		let resolveSelection: (value: TypesGen.ChatWorkspaceAgent) => void =
			() => {};
		vi.mocked(API.experimental.getChatWorkspaceAgent).mockImplementation(
			async (workspaceId) => {
				if (workspaceId !== mockMultiAgentWorkspace.id) {
					return { agent_id: MockWorkspaceAgent.id };
				}
				return new Promise((resolve) => {
					resolveSelection = resolve;
				});
			},
		);
		localStorage.setItem("agents.selected-workspace-id", mockWorkspace.id);
		const { onCreateChat } = renderForm({
			workspaceCount: 2,
			workspaceOptions: [mockWorkspace, mockMultiAgentWorkspace],
		});

		await attachZipFile();
		await user().click(screen.getByRole("button", { name: "More options" }));
		await user().click(
			await screen.findByRole("button", { name: /Attach workspace/ }),
		);
		await user().click(await screen.findByText("multi-agent-project"));
		await waitFor(() =>
			expect(API.experimental.getChatWorkspaceAgent).toHaveBeenCalledWith(
				mockMultiAgentWorkspace.id,
			),
		);

		await typeMessage("inspect this archive");
		await user().click(screen.getByRole("button", { name: "Send" }));
		expect(toast.error).toHaveBeenCalledWith(workspaceUploadUnavailableMessage);
		expect(onCreateChat).not.toHaveBeenCalled();
		expect(toast.warning).not.toHaveBeenCalledWith(removedQueuedFileMessage);

		resolveSelection({ agent_id: mainAgent.id });
		await clickSend();

		await waitFor(() => expect(onCreateChat).toHaveBeenCalledTimes(1));
		expect(submittedOptions(onCreateChat)).toMatchObject({
			workspaceId: mockMultiAgentWorkspace.id,
			uploadWorkspaceFiles: expect.any(Function),
		});
		expect(toast.warning).not.toHaveBeenCalledWith(removedQueuedFileMessage);
	});

	it("keeps queued files when the new workspace's agent selection fails", async () => {
		vi.mocked(API.experimental.getChatWorkspaceAgent).mockImplementation(
			async (workspaceId) => {
				if (workspaceId === mockMultiAgentWorkspace.id) {
					throw new Error("selection failed");
				}
				return { agent_id: MockWorkspaceAgent.id };
			},
		);
		localStorage.setItem("agents.selected-workspace-id", mockWorkspace.id);
		const { onCreateChat } = renderForm({
			workspaceCount: 2,
			workspaceOptions: [mockWorkspace, mockMultiAgentWorkspace],
		});

		await attachZipFile();
		await user().click(screen.getByRole("button", { name: "More options" }));
		await user().click(
			await screen.findByRole("button", { name: /Attach workspace/ }),
		);
		await user().click(await screen.findByText("multi-agent-project"));
		await waitFor(() =>
			expect(API.experimental.getChatWorkspaceAgent).toHaveBeenCalledWith(
				mockMultiAgentWorkspace.id,
			),
		);
		await submitMessage("inspect this archive");

		expect(toast.error).toHaveBeenCalledWith(workspaceUploadUnavailableMessage);
		expect(onCreateChat).not.toHaveBeenCalled();
		expect(toast.warning).not.toHaveBeenCalledWith(removedQueuedFileMessage);
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
