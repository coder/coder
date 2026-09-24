import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { act, StrictMode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AppProviders } from "#/App";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import {
	MockChatModel,
	MockChatModelProviderDescriptor,
	MockUnsetUserChatPersonalModelOverrides,
} from "#/testHelpers/chatModels";
import { createDeferred } from "#/testHelpers/deferred";
import {
	MockDefaultOrganization,
	MockUserPreferenceSettings,
	MockWorkspace,
} from "#/testHelpers/entities";
import { readMockFileText } from "#/testHelpers/files";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { persistedAttachmentsStorageKey } from "../hooks/useFileAttachments";
import {
	AgentCreateForm,
	type AgentCreatePrefill,
	emptyInputStorageKey,
	selectedWorkspaceIdStorageKey,
} from "./AgentCreateForm";

vi.mock("#/modules/dashboard/useDashboard", () => ({
	useDashboard: () => ({
		organizations: [MockDefaultOrganization],
		showOrganizations: false,
	}),
}));

const defaultModel: TypesGen.ChatModel = {
	...MockChatModel,
	id: "model-config-1",
	organization_id: MockDefaultOrganization.id,
	is_default: true,
};

const modelCatalog: TypesGen.OrganizationChatModelsResponse = {
	models: [defaultModel],
	providers: [MockChatModelProviderDescriptor],
	unsupported_providers: [],
};

const prefill: AgentCreatePrefill = {
	message: "Why did this build fail?",
	attachment: {
		name: "workspace-build-logs.txt",
		text: "Error: exit status 1\n",
	},
	organizationId: MockDefaultOrganization.id,
	autoSend: true,
};

const userDraftAttachments = JSON.stringify([
	{
		fileId: "user-draft-file",
		fileName: "notes.txt",
		fileType: "text/plain",
		lastModified: 1000,
		organizationId: MockDefaultOrganization.id,
	},
]);

const mockFormQueries = () => {
	vi.spyOn(API.experimental, "getChatModels").mockResolvedValue(modelCatalog);
	vi.spyOn(
		API.experimental,
		"getUserChatPersonalModelOverrides",
	).mockResolvedValue(MockUnsetUserChatPersonalModelOverrides);
	vi.spyOn(API.experimental, "getMCPServerConfigs").mockResolvedValue([]);
	vi.spyOn(API, "getUserPreferenceSettings").mockResolvedValue(
		MockUserPreferenceSettings,
	);
};

const renderForm = (
	onCreateChat: () => Promise<void>,
	props: Partial<Parameters<typeof AgentCreateForm>[0]> = {},
) =>
	render(
		<StrictMode>
			<AppProviders queryClient={createTestQueryClient()}>
				<AgentCreateForm
					onCreateChat={onCreateChat}
					isCreating={false}
					createError={undefined}
					canCreateChat
					canConfigureAgentSetup={false}
					workspaceCount={1}
					workspaceOptions={[MockWorkspace]}
					workspacesError={undefined}
					isWorkspacesLoading={false}
					prefill={prefill}
					{...props}
				/>
			</AppProviders>
		</StrictMode>,
	);

afterEach(() => {
	vi.restoreAllMocks();
	localStorage.clear();
});

describe("AgentCreateForm prefill", () => {
	it("uploads the attachment once, then sends once with only that file and no workspace", async () => {
		mockFormQueries();
		localStorage.setItem(emptyInputStorageKey, "draft the user typed earlier");
		localStorage.setItem(persistedAttachmentsStorageKey, userDraftAttachments);
		localStorage.setItem(selectedWorkspaceIdStorageKey, MockWorkspace.id);
		const upload = createDeferred<TypesGen.UploadChatFileResponse>();
		const uploadChatFile = vi
			.spyOn(API.experimental, "uploadChatFile")
			.mockReturnValue(upload.promise);
		const onCreateChat = vi.fn().mockResolvedValue(undefined);

		renderForm(onCreateChat);

		await waitFor(() => expect(uploadChatFile).toHaveBeenCalledTimes(1));
		const [uploadedFile, uploadOrganizationId] = uploadChatFile.mock.calls[0];
		expect(uploadedFile.name).toBe("workspace-build-logs.txt");
		expect(uploadedFile.type).toBe("text/plain");
		expect(await readMockFileText(uploadedFile)).toBe("Error: exit status 1\n");
		expect(uploadOrganizationId).toBe(MockDefaultOrganization.id);
		expect(onCreateChat).not.toHaveBeenCalled();

		await act(async () => {
			upload.resolve({ id: "uploaded-logs" });
		});

		await waitFor(() => expect(onCreateChat).toHaveBeenCalledTimes(1));
		expect(onCreateChat).toHaveBeenCalledWith({
			message: "Why did this build fail?",
			fileIDs: ["uploaded-logs"],
			workspaceId: undefined,
			model: defaultModel.id,
			reasoningEffort: undefined,
			organizationId: MockDefaultOrganization.id,
			mcpServerIds: undefined,
			planMode: undefined,
		});
		expect(uploadChatFile).toHaveBeenCalledTimes(1);
		expect(localStorage.getItem(emptyInputStorageKey)).toBe(
			"draft the user typed earlier",
		);
		expect(localStorage.getItem(persistedAttachmentsStorageKey)).toBe(
			userDraftAttachments,
		);
		expect(localStorage.getItem(selectedWorkspaceIdStorageKey)).toBe(
			MockWorkspace.id,
		);
	});

	it("waits for the model catalog before sending", async () => {
		mockFormQueries();
		const catalog = createDeferred<TypesGen.OrganizationChatModelsResponse>();
		vi.spyOn(API.experimental, "getChatModels").mockReturnValue(
			catalog.promise,
		);
		const uploadChatFile = vi
			.spyOn(API.experimental, "uploadChatFile")
			.mockResolvedValue({ id: "uploaded-logs" });
		const onCreateChat = vi.fn().mockResolvedValue(undefined);

		renderForm(onCreateChat);

		await waitFor(() => expect(uploadChatFile).toHaveBeenCalledTimes(1));
		await act(async () => {
			await Promise.resolve();
		});
		expect(onCreateChat).not.toHaveBeenCalled();

		await act(async () => {
			catalog.resolve(modelCatalog);
		});

		await waitFor(() => expect(onCreateChat).toHaveBeenCalledTimes(1));
		expect(onCreateChat).toHaveBeenCalledWith(
			expect.objectContaining({ model: defaultModel.id }),
		);
	});

	it("neither uploads nor sends for a user who cannot create chats", async () => {
		mockFormQueries();
		const uploadChatFile = vi
			.spyOn(API.experimental, "uploadChatFile")
			.mockResolvedValue({ id: "uploaded-logs" });
		const onCreateChat = vi.fn().mockResolvedValue(undefined);

		renderForm(onCreateChat, { canCreateChat: false });

		await screen.findByText("Permission required");
		expect(uploadChatFile).not.toHaveBeenCalled();
		expect(onCreateChat).not.toHaveBeenCalled();
	});

	it("does not save the attachment as a draft when the send fails", async () => {
		mockFormQueries();
		vi.spyOn(API.experimental, "uploadChatFile").mockResolvedValue({
			id: "uploaded-logs",
		});
		const onCreateChat = vi.fn().mockRejectedValue(new Error("server error"));

		renderForm(onCreateChat);

		await waitFor(() => expect(onCreateChat).toHaveBeenCalledTimes(1));
		expect(localStorage.getItem(persistedAttachmentsStorageKey)).toBeNull();
	});

	it("does not send when the attachment upload fails", async () => {
		mockFormQueries();
		vi.spyOn(API.experimental, "uploadChatFile").mockRejectedValue(
			new Error("upload failed"),
		);
		const onCreateChat = vi.fn().mockResolvedValue(undefined);

		renderForm(onCreateChat);

		await screen.findByText("The build logs could not be attached");
		expect(onCreateChat).not.toHaveBeenCalled();
	});

	it("only prefills without auto-send, and sends the composer contents on Send", async () => {
		mockFormQueries();
		vi.spyOn(API.experimental, "uploadChatFile").mockResolvedValue({
			id: "uploaded-logs",
		});
		const onCreateChat = vi.fn().mockResolvedValue(undefined);
		const user = userEvent.setup();

		renderForm(onCreateChat, { prefill: { ...prefill, autoSend: false } });

		const sendButton = screen.getByRole("button", { name: "Send" });
		await waitFor(() => expect(sendButton).toBeEnabled());
		expect(onCreateChat).not.toHaveBeenCalled();

		await user.click(sendButton);

		await waitFor(() => expect(onCreateChat).toHaveBeenCalledTimes(1));
		expect(onCreateChat).toHaveBeenCalledWith(
			expect.objectContaining({
				message: "Why did this build fail?",
				fileIDs: ["uploaded-logs"],
				workspaceId: undefined,
			}),
		);
	});
});
