import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { act, StrictMode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AppProviders } from "#/App";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import {
	MockChatModelProviderDescriptor,
	MockDefaultChatModel,
	MockUnsetUserChatPersonalModelOverrides,
} from "#/testHelpers/chatModels";
import { createDeferred } from "#/testHelpers/deferred";
import {
	MockDefaultOrganization,
	MockOrganization2,
	MockUserPreferenceSettings,
} from "#/testHelpers/entities";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { persistedAttachmentsStorageKey } from "../hooks/useFileAttachments";
import { readAgentAttachmentText } from "../utils/fileAttachmentLimits";
import {
	AgentCreateForm,
	type AgentCreatePrefill,
	emptyInputStorageKey,
} from "./AgentCreateForm";

const dashboard: {
	organizations: TypesGen.Organization[];
	showOrganizations: boolean;
} = {
	organizations: [MockDefaultOrganization],
	showOrganizations: false,
};

vi.mock("#/modules/dashboard/useDashboard", () => ({
	useDashboard: () => dashboard,
}));

const modelCatalog: TypesGen.OrganizationChatModelsResponse = {
	models: [MockDefaultChatModel],
	providers: [MockChatModelProviderDescriptor],
	unsupported_providers: [],
};

const prefill: AgentCreatePrefill = {
	message: "Why did this build fail?",
	attachment: {
		name: "workspace-build-logs.txt",
		text: "Error: exit status 1\n",
	},
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
	queryClient = createTestQueryClient(),
) => {
	const element = (
		overrides: Partial<Parameters<typeof AgentCreateForm>[0]>,
	) => (
		<StrictMode>
			<AppProviders queryClient={queryClient}>
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
					prefill={prefill}
					{...props}
					{...overrides}
				/>
			</AppProviders>
		</StrictMode>
	);
	const result = render(element({}));
	return {
		rerender: (overrides: Partial<Parameters<typeof AgentCreateForm>[0]>) =>
			result.rerender(element(overrides)),
	};
};

afterEach(() => {
	vi.restoreAllMocks();
	localStorage.clear();
	dashboard.organizations = [MockDefaultOrganization];
	dashboard.showOrganizations = false;
});

describe("AgentCreateForm prefill", () => {
	it("uploads the attachment once, then sends once with only that file", async () => {
		mockFormQueries();
		localStorage.setItem(emptyInputStorageKey, "draft the user typed earlier");
		localStorage.setItem(persistedAttachmentsStorageKey, userDraftAttachments);
		const upload = createDeferred<TypesGen.UploadChatFileResponse>();
		const uploadChatFile = vi
			.spyOn(API.experimental, "uploadChatFile")
			.mockReturnValue(upload.promise);
		const onCreateChat = vi.fn().mockResolvedValue(undefined);
		const user = userEvent.setup();

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
		// The composer is read-only while the automatic send is pending.
		const message = screen.getByRole("textbox", { name: "Chat message" });
		await user.click(message);
		await user.paste(" and how do I fix it?");
		expect(message).toHaveTextContent(/^Why did this build fail\?$/);

		await act(async () => {
			upload.resolve({ id: "uploaded-logs" });
		});

		await waitFor(() => expect(onCreateChat).toHaveBeenCalledTimes(1));
		expect(onCreateChat).toHaveBeenCalledWith({
			message: "Why did this build fail?",
			fileIDs: ["uploaded-logs"],
			workspaceId: undefined,
			model: MockDefaultChatModel.id,
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
	});

	it("sends once and stays read-only while the automatic send is in flight", async () => {
		mockFormQueries();
		vi.spyOn(API.experimental, "uploadChatFile").mockResolvedValue({
			id: "uploaded-logs",
		});
		const onCreateChat = vi.fn().mockReturnValue(new Promise(() => {}));
		const user = userEvent.setup();

		const { rerender } = renderForm(onCreateChat);

		await waitFor(() => expect(onCreateChat).toHaveBeenCalledTimes(1));
		// The page reports the mutation, then a gate flip that must not resend.
		rerender({ isCreating: true });
		rerender({ isCreating: false });
		const message = screen.getByRole("textbox", { name: "Chat message" });
		await user.click(message);
		await user.paste(" and how do I fix it?");

		expect(message).toHaveTextContent(/^Why did this build fail\?$/);
		expect(onCreateChat).toHaveBeenCalledTimes(1);
	});

	it("auto-sends a prefill whose file name needs sanitizing", async () => {
		mockFormQueries();
		const uploadChatFile = vi
			.spyOn(API.experimental, "uploadChatFile")
			.mockResolvedValue({ id: "uploaded-logs" });
		const onCreateChat = vi.fn().mockResolvedValue(undefined);

		renderForm(onCreateChat, {
			prefill: {
				...prefill,
				attachment: { ...prefill.attachment, name: "build logs (1).txt" },
			},
		});

		await waitFor(() => expect(onCreateChat).toHaveBeenCalledTimes(1));
		expect(uploadChatFile.mock.calls[0][0].name).not.toBe("build logs (1).txt");
		expect(onCreateChat).toHaveBeenCalledWith(
			expect.objectContaining({ fileIDs: ["uploaded-logs"] }),
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
			expect.objectContaining({ model: MockDefaultChatModel.id }),
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

		const sendButton = screen.getByRole("button", { name: "Send" });
		await waitFor(() => expect(sendButton).toBeEnabled());
		expect(onCreateChat).not.toHaveBeenCalled();

		await user.click(sendButton);

		await waitFor(() => expect(onCreateChat).toHaveBeenCalledTimes(1));
		expect(onCreateChat).toHaveBeenCalledWith(
			expect.objectContaining({
				message: "Why did this build fail?",
				fileIDs: undefined,
			}),
		);
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
			}),
		);
	});

	it("uploads the logs again and sends to the new organization when theirs is revoked", async () => {
		mockFormQueries();
		dashboard.showOrganizations = true;
		dashboard.organizations = [MockDefaultOrganization, MockOrganization2];
		vi.spyOn(API, "getOrganizations").mockResolvedValue(
			dashboard.organizations,
		);
		let permittedOrganization = MockOrganization2;
		vi.spyOn(API, "checkAuthorization").mockImplementation(async () => ({
			[MockDefaultOrganization.id]:
				permittedOrganization === MockDefaultOrganization,
			[MockOrganization2.id]: permittedOrganization === MockOrganization2,
		}));
		const uploadToOrg2 = createDeferred<TypesGen.UploadChatFileResponse>();
		const uploadChatFile = vi
			.spyOn(API.experimental, "uploadChatFile")
			.mockImplementation((_file, organizationId) =>
				organizationId === MockOrganization2.id
					? uploadToOrg2.promise
					: Promise.resolve({ id: "file-in-org1" }),
			);
		const onCreateChat = vi.fn().mockResolvedValue(undefined);
		const queryClient = createTestQueryClient();

		renderForm(onCreateChat, {}, queryClient);

		await waitFor(() => expect(uploadChatFile).toHaveBeenCalledTimes(1));
		expect(uploadChatFile.mock.calls[0][1]).toBe(MockOrganization2.id);

		// A permission refetch revokes the upload's organization before the
		// upload finishes.
		permittedOrganization = MockDefaultOrganization;
		await act(async () => {
			await queryClient.invalidateQueries({
				queryKey: ["organizations", "permitted"],
			});
		});
		await act(async () => {
			uploadToOrg2.resolve({ id: "file-in-org2" });
		});

		await waitFor(() => expect(onCreateChat).toHaveBeenCalledTimes(1));
		expect(onCreateChat).toHaveBeenCalledWith(
			expect.objectContaining({
				organizationId: MockDefaultOrganization.id,
				fileIDs: ["file-in-org1"],
			}),
		);
		expect(uploadChatFile).toHaveBeenCalledTimes(2);
		expect(uploadChatFile.mock.calls[1][1]).toBe(MockDefaultOrganization.id);
	});
});
