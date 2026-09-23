import { render, waitFor } from "@testing-library/react";
import { act } from "react";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { AppProviders } from "#/App";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import {
	MockChatModel,
	MockChatModelProviderDescriptor,
} from "#/testHelpers/chatModels";
import { createDeferred } from "#/testHelpers/deferred";
import {
	MockDefaultOrganization,
	MockUserPreferenceSettings,
} from "#/testHelpers/entities";
import { AgentCreateForm } from "./AgentCreateForm";

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

const unsetOverride = (
	context: TypesGen.ChatPersonalModelOverride["context"],
): TypesGen.ChatPersonalModelOverride => ({
	context,
	mode: "deployment_default",
	model_config_id: "",
	is_set: false,
});

const personalModelOverrides: TypesGen.UserChatPersonalModelOverridesResponse =
	{
		enabled: true,
		root: unsetOverride("root"),
		general: unsetOverride("general"),
		explore: unsetOverride("explore"),
		deployment_defaults: {
			general: { context: "general", model_config_id: "" },
			explore: { context: "explore", model_config_id: "" },
		},
	};

// jsdom's File does not implement Blob.text().
const readFileText = (file: File) =>
	new Promise<string>((resolve, reject) => {
		const reader = new FileReader();
		reader.onload = () => resolve(String(reader.result));
		reader.onerror = () => reject(reader.error);
		reader.readAsText(file);
	});

beforeAll(() => {
	// Lexical measures selection rects, which jsdom does not implement.
	Object.defineProperty(Range.prototype, "getBoundingClientRect", {
		configurable: true,
		value: () => new DOMRect(0, 0, 1, 16),
	});
});

afterEach(() => {
	vi.restoreAllMocks();
	localStorage.clear();
});

describe("AgentCreateForm autoSubmit", () => {
	it("uploads the attachment, then creates the chat once with the message and file", async () => {
		vi.spyOn(API.experimental, "getChatModels").mockResolvedValue(modelCatalog);
		vi.spyOn(
			API.experimental,
			"getUserChatPersonalModelOverrides",
		).mockResolvedValue(personalModelOverrides);
		vi.spyOn(API.experimental, "getMCPServerConfigs").mockResolvedValue([]);
		vi.spyOn(API, "getUserPreferenceSettings").mockResolvedValue(
			MockUserPreferenceSettings,
		);
		const upload = createDeferred<TypesGen.UploadChatFileResponse>();
		const uploadChatFile = vi
			.spyOn(API.experimental, "uploadChatFile")
			.mockReturnValue(upload.promise);
		const onCreateChat = vi.fn().mockResolvedValue(undefined);

		render(
			<AppProviders>
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
					autoSubmit={{
						message: "Why did this build fail?",
						attachment: {
							name: "workspace-build-logs.txt",
							content: "Error: exit status 1\n",
						},
					}}
				/>
			</AppProviders>,
		);

		await waitFor(() => expect(uploadChatFile).toHaveBeenCalledTimes(1));
		const [uploadedFile, uploadOrganizationId] = uploadChatFile.mock.calls[0];
		expect(uploadedFile.name).toBe("workspace-build-logs.txt");
		expect(uploadedFile.type).toBe("text/plain");
		expect(await readFileText(uploadedFile)).toBe("Error: exit status 1\n");
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
				model: defaultModel.id,
			}),
		);
	});
});
