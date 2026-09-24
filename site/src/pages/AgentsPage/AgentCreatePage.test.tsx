import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import {
	MockChatModel,
	MockChatModelProviderDescriptor,
	MockUnsetUserChatPersonalModelOverrides,
} from "#/testHelpers/chatModels";
import {
	MockDefaultOrganization,
	MockFailedWorkspaceBuild,
	MockUserPreferenceSettings,
	MockWorkspace,
	MockWorkspaceBuildLogs,
} from "#/testHelpers/entities";
import { readMockFileText } from "#/testHelpers/files";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import AgentCreatePage from "./AgentCreatePage";
import { selectedWorkspaceIdStorageKey } from "./components/AgentCreateForm";
import {
	buildDebugWorkspaceBuildPath,
	debugWorkspaceBuildLogsFileName,
	debugWorkspaceBuildPrompt,
	formatWorkspaceBuildLogsForDebug,
	storeDebugWorkspaceBuildIntent,
} from "./utils/workspaceBuildDebug";

// AgentPageHeader needs the layout's outlet context.
vi.mock("./components/AgentPageHeader", () => ({
	AgentPageHeader: () => null,
}));

const defaultModel: TypesGen.ChatModel = {
	...MockChatModel,
	id: "model-config-1",
	organization_id: MockDefaultOrganization.id,
	is_default: true,
};

const failedBuild: TypesGen.WorkspaceBuild = {
	...MockFailedWorkspaceBuild("start"),
	id: "9f0e7d0e-4b2b-4ac9-8f1a-1a7a1f0c9d11",
};

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
		models: [defaultModel],
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

afterEach(() => {
	vi.restoreAllMocks();
	localStorage.clear();
});

describe("AgentCreatePage debug deep link", () => {
	it("sends the build logs once after the button click", async () => {
		enableExperiment();
		const { uploadChatFile, createChat } = mockPageQueries();
		localStorage.setItem(selectedWorkspaceIdStorageKey, MockWorkspace.id);
		storeDebugWorkspaceBuildIntent(failedBuild.id);

		const { router } = renderPage();

		await waitFor(() => expect(createChat).toHaveBeenCalledTimes(1));
		const [uploadedFile] = uploadChatFile.mock.calls[0];
		expect(uploadedFile.name).toBe(
			debugWorkspaceBuildLogsFileName(failedBuild),
		);
		expect(await readMockFileText(uploadedFile)).toBe(
			formatWorkspaceBuildLogsForDebug(failedBuild, MockWorkspaceBuildLogs),
		);
		expect(createChat).toHaveBeenCalledWith(
			expect.objectContaining({
				organization_id: failedBuild.job.organization_id,
				workspace_id: undefined,
				model_config_id: defaultModel.id,
				client_type: "ui",
				content: [
					{ type: "text", text: debugWorkspaceBuildPrompt(failedBuild) },
					{ type: "file", file_id: "uploaded-logs" },
				],
			}),
		);
		// Back must not land on the deep link again.
		await waitFor(() =>
			expect(router.state.location).toMatchObject({
				pathname: "/agents/new-chat-id",
				search: "?archived=archived",
			}),
		);
		expect(router.state.historyAction).toBe("REPLACE");
		expect(uploadChatFile).toHaveBeenCalledTimes(1);
	});

	it("only prefills a link opened without the button click", async () => {
		enableExperiment();
		const { uploadChatFile, createChat } = mockPageQueries();
		const user = userEvent.setup();

		renderPage();

		const sendButton = await screen.findByRole("button", { name: "Send" });
		await waitFor(() => expect(sendButton).toBeEnabled());
		expect(uploadChatFile).toHaveBeenCalledTimes(1);
		expect(createChat).not.toHaveBeenCalled();

		await user.click(sendButton);

		await waitFor(() => expect(createChat).toHaveBeenCalledTimes(1));
		expect(createChat).toHaveBeenCalledWith(
			expect.objectContaining({
				content: [
					{ type: "text", text: debugWorkspaceBuildPrompt(failedBuild) },
					{ type: "file", file_id: "uploaded-logs" },
				],
			}),
		);
	});

	it("sends at most once when creating the chat fails", async () => {
		enableExperiment();
		const { createChat } = mockPageQueries();
		createChat.mockRejectedValue(new Error("server error"));
		storeDebugWorkspaceBuildIntent(failedBuild.id);

		renderPage();

		await waitFor(() => expect(createChat).toHaveBeenCalledTimes(1));
		await screen.findByText("server error");
		expect(createChat).toHaveBeenCalledTimes(1);
	});

	it("ignores the deep link without the experiment", async () => {
		const { uploadChatFile } = mockPageQueries();
		const getWorkspaceBuild = vi.spyOn(API, "getWorkspaceBuild");
		storeDebugWorkspaceBuildIntent(failedBuild.id);

		renderPage();

		await screen.findByRole("textbox", { name: "Chat message" });
		expect(getWorkspaceBuild).not.toHaveBeenCalled();
		expect(uploadChatFile).not.toHaveBeenCalled();
	});

	it("ignores a build ID that is not a UUID", async () => {
		enableExperiment();
		mockPageQueries();
		const getWorkspaceBuild = vi.spyOn(API, "getWorkspaceBuild");

		renderPage(buildDebugWorkspaceBuildPath("../templateversions/abc"));

		await screen.findByRole("textbox", { name: "Chat message" });
		expect(getWorkspaceBuild).not.toHaveBeenCalled();
	});

	it("does not attach logs for a build that did not fail", async () => {
		enableExperiment();
		const { uploadChatFile } = mockPageQueries();
		vi.spyOn(API, "getWorkspaceBuild").mockResolvedValue({
			...failedBuild,
			status: "running",
			job: { ...failedBuild.job, status: "succeeded" },
		});
		storeDebugWorkspaceBuildIntent(failedBuild.id);

		renderPage();

		await screen.findByText("Nothing to debug");
		await screen.findByRole("textbox", { name: "Chat message" });
		expect(uploadChatFile).not.toHaveBeenCalled();
	});
});
