import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { onlineManager } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import {
	buildDebugWorkspaceBuildPath,
	debugWorkspaceBuildIntentStorageKey,
	storeDebugWorkspaceBuildIntent,
} from "#/modules/workspaces/workspaceBuildDebugLink";
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
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import AgentCreatePage from "./AgentCreatePage";
import { selectedWorkspaceIdStorageKey } from "./components/AgentCreateForm";
import { readAgentAttachmentText } from "./utils/fileAttachmentLimits";
import {
	debugWorkspaceBuildLogsFileName,
	debugWorkspaceBuildPrompt,
	formatWorkspaceBuildLogsForDebug,
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

const findChatMessage = () =>
	screen.findByRole("textbox", { name: "Chat message" });

afterEach(() => {
	vi.restoreAllMocks();
	localStorage.clear();
	onlineManager.setOnline(true);
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
		expect(await readAgentAttachmentText(uploadedFile)).toBe(
			formatWorkspaceBuildLogsForDebug(failedBuild, MockWorkspaceBuildLogs),
		);
		expect(createChat).toHaveBeenCalledWith(
			expect.objectContaining({
				organization_id: failedBuild.job.organization_id,
				workspace_id: undefined,
				mcp_server_ids: undefined,
				model_config_id: defaultModel.id,
				client_type: "ui",
				content: [
					{ type: "text", text: debugWorkspaceBuildPrompt(failedBuild) },
					{ type: "file", file_id: "uploaded-logs" },
				],
			}),
		);
		await waitFor(() =>
			expect(router.state.location).toMatchObject({
				pathname: "/agents/new-chat-id",
				search: "?archived=archived",
			}),
		);
		expect(uploadChatFile).toHaveBeenCalledTimes(1);
		// A reload or a second tab must not send again.
		expect(
			localStorage.getItem(debugWorkspaceBuildIntentStorageKey),
		).toBeNull();
	});

	it("only prefills a link opened without the button click", async () => {
		enableExperiment();
		const { uploadChatFile, createChat } = mockPageQueries();
		const user = userEvent.setup();

		const { router } = renderPage();

		const sendButton = await screen.findByRole("button", { name: "Send" });
		await waitFor(() => expect(sendButton).toBeEnabled());
		expect(uploadChatFile).toHaveBeenCalledTimes(1);
		expect(createChat).not.toHaveBeenCalled();
		// The layout forwards location.search to every link, so the param is
		// dropped from the URL once it has been read.
		expect(router.state.location).toMatchObject({
			pathname: "/agents",
			search: "?archived=archived",
		});
		expect(router.state.historyAction).toBe("REPLACE");

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

	it("only prefills when the build belongs to someone else", async () => {
		enableExperiment();
		const { createChat } = mockPageQueries();
		vi.spyOn(API, "getWorkspaceBuild").mockResolvedValue({
			...failedBuild,
			workspace_owner_id: "another-user-id",
		});
		storeDebugWorkspaceBuildIntent(failedBuild.id);

		renderPage();

		const sendButton = await screen.findByRole("button", { name: "Send" });
		await waitFor(() => expect(sendButton).toBeEnabled());
		expect(createChat).not.toHaveBeenCalled();
		expect(
			localStorage.getItem(debugWorkspaceBuildIntentStorageKey),
		).toBeNull();
	});

	it("does not retry a failed automatic send until the user presses Send", async () => {
		enableExperiment();
		const { createChat } = mockPageQueries();
		createChat
			.mockRejectedValueOnce(new Error("server error"))
			.mockResolvedValue({ ...MockChat, id: "new-chat-id" });
		storeDebugWorkspaceBuildIntent(failedBuild.id);
		const user = userEvent.setup();

		const { router } = renderPage();

		await waitFor(() => expect(createChat).toHaveBeenCalledTimes(1));
		await screen.findByText("server error");
		const sendButton = screen.getByRole("button", { name: "Send" });
		await waitFor(() => expect(sendButton).toBeEnabled());
		expect(createChat).toHaveBeenCalledTimes(1);

		await user.click(sendButton);

		await waitFor(() => expect(createChat).toHaveBeenCalledTimes(2));
		expect(createChat.mock.calls[1][0].content).toEqual(
			createChat.mock.calls[0][0].content,
		);
		await waitFor(() =>
			expect(router.state.location.pathname).toBe("/agents/new-chat-id"),
		);
	});

	it("explains a deep link opened without the experiment", async () => {
		const { uploadChatFile } = mockPageQueries();
		const getWorkspaceBuild = vi.spyOn(API, "getWorkspaceBuild");
		storeDebugWorkspaceBuildIntent(failedBuild.id);

		renderPage();

		await screen.findByText("This debug link is not enabled here");
		await findChatMessage();
		expect(getWorkspaceBuild).not.toHaveBeenCalled();
		expect(uploadChatFile).not.toHaveBeenCalled();
	});

	it("explains a build ID that is not a UUID", async () => {
		enableExperiment();
		mockPageQueries();
		const getWorkspaceBuild = vi.spyOn(API, "getWorkspaceBuild");

		renderPage(buildDebugWorkspaceBuildPath("../templateversions/abc"));

		await screen.findByText("This debug link is not valid");
		await findChatMessage();
		expect(getWorkspaceBuild).not.toHaveBeenCalled();
	});

	it("does not fetch or attach logs for a build that has not failed", async () => {
		enableExperiment();
		const { uploadChatFile } = mockPageQueries();
		vi.spyOn(API, "getWorkspaceBuild").mockResolvedValue({
			...failedBuild,
			status: "running",
			job: { ...failedBuild.job, status: "running" },
		});
		const getWorkspaceBuildLogs = vi.spyOn(API, "getWorkspaceBuildLogs");
		storeDebugWorkspaceBuildIntent(failedBuild.id);

		renderPage();

		await screen.findByText("Nothing to debug");
		await screen.findByText(/has not failed \(status: running\)/);
		await findChatMessage();
		expect(getWorkspaceBuildLogs).not.toHaveBeenCalled();
		expect(uploadChatFile).not.toHaveBeenCalled();
	});

	it("reports a build that fails to load and sends nothing, even after a reconnect", async () => {
		enableExperiment();
		const { uploadChatFile, createChat } = mockPageQueries();
		vi.spyOn(API, "getWorkspaceBuild")
			.mockRejectedValueOnce(new Error("boom"))
			.mockResolvedValue(failedBuild);
		storeDebugWorkspaceBuildIntent(failedBuild.id);

		renderPage();

		await screen.findByText(
			"Could not load the failed workspace build. Nothing was sent.",
		);
		await screen.findByText("boom");
		await findChatMessage();

		onlineManager.setOnline(false);
		onlineManager.setOnline(true);
		await findChatMessage();

		expect(API.getWorkspaceBuild).toHaveBeenCalledTimes(1);
		expect(uploadChatFile).not.toHaveBeenCalled();
		expect(createChat).not.toHaveBeenCalled();
	});

	it("reports logs that fail to load and sends nothing", async () => {
		enableExperiment();
		const { uploadChatFile, createChat } = mockPageQueries();
		vi.spyOn(API, "getWorkspaceBuildLogs").mockRejectedValue(
			new Error("logs unavailable"),
		);
		storeDebugWorkspaceBuildIntent(failedBuild.id);

		renderPage();

		await screen.findByText(
			"Could not load the failed workspace build. Nothing was sent.",
		);
		await screen.findByText("logs unavailable");
		await findChatMessage();
		expect(uploadChatFile).not.toHaveBeenCalled();
		expect(createChat).not.toHaveBeenCalled();
	});
});
