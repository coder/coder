import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { act, StrictMode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
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
	MockOrganization2,
	MockUserPreferenceSettings,
	MockWorkspace,
} from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { persistedAttachmentsStorageKey } from "../hooks/useFileAttachments";
import { readAgentAttachmentText } from "../utils/fileAttachmentLimits";
import {
	AgentCreateForm,
	type AgentCreatePrefill,
	emptyInputStorageKey,
	selectedOrganizationIdStorageKey,
	selectedWorkspaceIdStorageKey,
} from "./AgentCreateForm";

const dashboard = vi.hoisted(() => ({
	current: {
		organizations: [] as TypesGen.Organization[],
		showOrganizations: false,
	},
}));

vi.mock("#/modules/dashboard/useDashboard", () => ({
	useDashboard: () => dashboard.current,
}));

const defaultModel: TypesGen.ChatModel = {
	...MockChatModel,
	id: "model-config-1",
	organization_id: MockDefaultOrganization.id,
	is_default: true,
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
	vi.spyOn(API.experimental, "getChatModels").mockImplementation(
		async (organizationId) => ({
			models: [{ ...defaultModel, organization_id: organizationId }],
			providers: [MockChatModelProviderDescriptor],
			unsupported_providers: [],
		}),
	);
	vi.spyOn(
		API.experimental,
		"getUserChatPersonalModelOverrides",
	).mockResolvedValue(MockUnsetUserChatPersonalModelOverrides);
	vi.spyOn(API.experimental, "getMCPServerConfigs").mockResolvedValue([]);
	vi.spyOn(API, "getUserPreferenceSettings").mockResolvedValue(
		MockUserPreferenceSettings,
	);
};

// The permitted-organizations query authorizes each organization separately.
const mockOrganizations = (permitted: Record<string, boolean>) => {
	dashboard.current = {
		organizations: [MockDefaultOrganization, MockOrganization2],
		showOrganizations: true,
	};
	vi.spyOn(API, "getOrganizations").mockResolvedValue([
		MockDefaultOrganization,
		MockOrganization2,
	]);
	vi.spyOn(API, "checkAuthorization").mockImplementation(async (request) =>
		Object.fromEntries(
			Object.keys(request.checks).map((key) => [key, permitted[key] ?? true]),
		),
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

const chatMessage = () => screen.getByRole("textbox", { name: "Chat message" });

beforeEach(() => {
	dashboard.current = {
		organizations: [MockDefaultOrganization],
		showOrganizations: false,
	};
});

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
		const getMCPServerConfigs = vi.spyOn(
			API.experimental,
			"getMCPServerConfigs",
		);
		const onCreateChat = vi.fn().mockResolvedValue(undefined);

		renderForm(onCreateChat);

		await waitFor(() => expect(uploadChatFile).toHaveBeenCalledTimes(1));
		const [uploadedFile, uploadOrganizationId] = uploadChatFile.mock.calls[0];
		expect(uploadedFile.name).toBe("workspace-build-logs.txt");
		expect(uploadedFile.type).toBe("text/plain");
		expect(await readAgentAttachmentText(uploadedFile)).toBe(
			"Error: exit status 1\n",
		);
		expect(uploadOrganizationId).toBe(MockDefaultOrganization.id);
		expect(onCreateChat).not.toHaveBeenCalled();
		// Typing during the pending send would be discarded.
		expect(chatMessage()).toHaveAttribute("aria-disabled", "true");

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
		expect(getMCPServerConfigs).not.toHaveBeenCalled();
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

	it("uses the build's organization, not the stored one", async () => {
		mockFormQueries();
		mockOrganizations({});
		localStorage.setItem(
			selectedOrganizationIdStorageKey,
			MockDefaultOrganization.id,
		);
		const uploadChatFile = vi
			.spyOn(API.experimental, "uploadChatFile")
			.mockResolvedValue({ id: "uploaded-logs" });
		const onCreateChat = vi.fn().mockResolvedValue(undefined);

		renderForm(onCreateChat, {
			prefill: { ...prefill, organizationId: MockOrganization2.id },
		});

		await waitFor(() => expect(onCreateChat).toHaveBeenCalledTimes(1));
		expect(uploadChatFile.mock.calls[0][1]).toBe(MockOrganization2.id);
		expect(onCreateChat).toHaveBeenCalledWith(
			expect.objectContaining({ organizationId: MockOrganization2.id }),
		);
		expect(localStorage.getItem(selectedOrganizationIdStorageKey)).toBe(
			MockDefaultOrganization.id,
		);
	});

	it("neither uploads nor sends when the user cannot create chats in the build's organization", async () => {
		mockFormQueries();
		mockOrganizations({ [MockOrganization2.id]: false });
		const uploadChatFile = vi
			.spyOn(API.experimental, "uploadChatFile")
			.mockResolvedValue({ id: "uploaded-logs" });
		const onCreateChat = vi.fn().mockResolvedValue(undefined);

		renderForm(onCreateChat, {
			prefill: { ...prefill, organizationId: MockOrganization2.id },
		});

		await screen.findByText(
			"You cannot create chats in this build's organization. Nothing was sent.",
		);
		await act(async () => {
			await Promise.resolve();
		});
		expect(uploadChatFile).not.toHaveBeenCalled();
		expect(onCreateChat).not.toHaveBeenCalled();
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
			catalog.resolve({
				models: [defaultModel],
				providers: [MockChatModelProviderDescriptor],
				unsupported_providers: [],
			});
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

		await screen.findByText(
			"The build logs could not be attached. Nothing was sent.",
		);
		expect(onCreateChat).not.toHaveBeenCalled();
	});

	it("keeps Send disabled after the upload fails without auto-send", async () => {
		mockFormQueries();
		vi.spyOn(API.experimental, "uploadChatFile").mockRejectedValue(
			new Error("upload failed"),
		);
		const onCreateChat = vi.fn().mockResolvedValue(undefined);

		renderForm(onCreateChat, { prefill: { ...prefill, autoSend: false } });

		await screen.findByText(
			"The build logs could not be attached. Nothing was sent.",
		);
		expect(screen.getByRole("button", { name: "Send" })).toBeDisabled();
	});

	it("hands the composer back instead of sending when the logs are removed", async () => {
		mockFormQueries();
		const upload = createDeferred<TypesGen.UploadChatFileResponse>();
		vi.spyOn(API.experimental, "uploadChatFile").mockReturnValue(
			upload.promise,
		);
		const onCreateChat = vi.fn().mockResolvedValue(undefined);
		const user = userEvent.setup();

		renderForm(onCreateChat);

		await user.click(
			await screen.findByRole("button", {
				name: "Remove workspace-build-logs.txt",
			}),
		);
		await act(async () => {
			upload.resolve({ id: "uploaded-logs" });
		});

		await waitFor(() =>
			expect(chatMessage()).toHaveAttribute("aria-disabled", "false"),
		);
		expect(onCreateChat).not.toHaveBeenCalled();
	});

	it("only prefills without auto-send, and sends on Send", async () => {
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
