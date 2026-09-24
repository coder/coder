import { render, screen, waitFor } from "@testing-library/react";
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

vi.mock("#/modules/dashboard/useDashboard", () => ({
	useDashboard: () => ({
		organizations: [MockDefaultOrganization],
		showOrganizations: false,
	}),
}));

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
};

const renderForm = (onCreateChat: () => Promise<void>) =>
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
					prefill={prefill}
				/>
			</AppProviders>
		</StrictMode>,
	);

afterEach(() => {
	vi.restoreAllMocks();
	localStorage.clear();
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

		renderForm(onCreateChat);

		await waitFor(() => expect(uploadChatFile).toHaveBeenCalledTimes(1));
		const [uploadedFile, uploadOrganizationId] = uploadChatFile.mock.calls[0];
		expect(uploadedFile.name).toBe("workspace-build-logs.txt");
		expect(await readAgentAttachmentText(uploadedFile)).toBe(
			"Error: exit status 1\n",
		);
		expect(uploadOrganizationId).toBe(MockDefaultOrganization.id);
		expect(onCreateChat).not.toHaveBeenCalled();

		await act(async () => {
			upload.resolve({ id: "uploaded-logs" });
		});

		await waitFor(() => expect(onCreateChat).toHaveBeenCalledTimes(1));
		expect(onCreateChat).toHaveBeenCalledWith(
			expect.objectContaining({
				message: "Why did this build fail?",
				fileIDs: ["uploaded-logs"],
				organizationId: MockDefaultOrganization.id,
			}),
		);
		expect(uploadChatFile).toHaveBeenCalledTimes(1);
		// The user's own draft is untouched.
		expect(localStorage.getItem(emptyInputStorageKey)).toBe(
			"draft the user typed earlier",
		);
		expect(localStorage.getItem(persistedAttachmentsStorageKey)).toBe(
			userDraftAttachments,
		);
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
		expect(screen.getByRole("button", { name: "Send" })).toBeDisabled();
		expect(onCreateChat).not.toHaveBeenCalled();
	});
});
