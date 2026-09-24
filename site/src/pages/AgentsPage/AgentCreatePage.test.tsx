import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import {
	buildDebugWorkspaceBuildPath,
	debugWorkspaceBuildIntentStorageKey,
	storeDebugWorkspaceBuildIntent,
} from "#/modules/workspaces/workspaceBuildDebugLink";
import { MockChat } from "#/testHelpers/chatEntities";
import {
	MockChatModelProviderDescriptor,
	MockDefaultChatModel,
	MockUnsetUserChatPersonalModelOverrides,
} from "#/testHelpers/chatModels";
import {
	MockFailedWorkspaceBuildWithUUID,
	MockUserPreferenceSettings,
	MockWorkspaceBuildLogs,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import AgentCreatePage from "./AgentCreatePage";
import { emptyInputStorageKey } from "./components/AgentCreateForm";
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

const failedBuild = MockFailedWorkspaceBuildWithUUID;

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
		models: [MockDefaultChatModel],
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

const findEnabledSendButton = async () => {
	const sendButton = await screen.findByRole("button", { name: "Send" });
	await waitFor(() => expect(sendButton).toBeEnabled());
	return sendButton;
};

afterEach(() => {
	vi.restoreAllMocks();
	localStorage.clear();
});

describe("AgentCreatePage debug deep link", () => {
	it("sends the build logs once after the button click", async () => {
		enableExperiment();
		const { uploadChatFile, createChat } = mockPageQueries();
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
				model_config_id: MockDefaultChatModel.id,
				client_type: "ui",
				content: [
					{ type: "text", text: debugWorkspaceBuildPrompt(failedBuild) },
					{ type: "file", file_id: "uploaded-logs" },
				],
			}),
		);
		// The chat's URL does not carry the build ID.
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

		renderPage();

		const sendButton = await findEnabledSendButton();
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

	it("gives New chat a plain composer even though the link's param is forwarded", async () => {
		enableExperiment();
		const { uploadChatFile, createChat } = mockPageQueries();
		localStorage.setItem(emptyInputStorageKey, "draft the user typed earlier");

		const { router } = renderPage();

		await findEnabledSendButton();
		// Stands in for the layout's New chat link, which forwards location.search.
		await router.navigate({
			pathname: "/agents",
			search: router.state.location.search,
		});

		await waitFor(() =>
			expect(
				screen.getByRole("textbox", { name: "Chat message" }),
			).toHaveTextContent("draft the user typed earlier"),
		);
		expect(uploadChatFile).toHaveBeenCalledTimes(1);
		expect(createChat).not.toHaveBeenCalled();
	});

	it("reports a build that fails to load and sends nothing", async () => {
		enableExperiment();
		const { uploadChatFile, createChat } = mockPageQueries();
		vi.spyOn(API, "getWorkspaceBuild").mockRejectedValue(new Error("boom"));
		storeDebugWorkspaceBuildIntent(failedBuild.id);

		renderPage();

		await screen.findByText(
			"Could not load the workspace build or its logs. Nothing was sent.",
		);
		await screen.findByText("boom");
		await findChatMessage();
		expect(uploadChatFile).not.toHaveBeenCalled();
		expect(createChat).not.toHaveBeenCalled();
	});
});
