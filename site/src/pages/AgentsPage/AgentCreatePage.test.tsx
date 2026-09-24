import { screen, waitFor } from "@testing-library/react";
import { HttpResponse, http } from "msw";
import { useLocation } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import {
	MockChatModel,
	MockChatModelProviderDescriptor,
	MockUserChatPersonalModelOverrides,
} from "#/testHelpers/chatModels";
import {
	MockDefaultOrganization,
	MockFailedWorkspaceBuild,
	MockUserPreferenceSettings,
	MockWorkspaceBuildLogs,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import AgentCreatePage from "./AgentCreatePage";
import {
	debugWorkspaceBuildLogsFileName,
	debugWorkspaceBuildPrompt,
	debugWorkspaceBuildSearchParam,
	formatWorkspaceBuildLogsForDebug,
} from "./utils/workspaceBuildDebug";

// The header reads the sidebar outlet context, which this test does not
// exercise.
vi.mock("./components/AgentPageHeader", () => ({
	AgentPageHeader: () => null,
}));

const defaultModel: TypesGen.ChatModel = {
	...MockChatModel,
	id: "model-config-1",
	organization_id: MockDefaultOrganization.id,
	is_default: true,
};

const failedBuild = MockFailedWorkspaceBuild("start");

const LocationProbe = () => {
	const location = useLocation();
	return (
		<output>
			{location.pathname}
			{location.search}
		</output>
	);
};

// jsdom's File does not implement Blob.text().
const readFileText = (file: File) =>
	new Promise<string>((resolve, reject) => {
		const reader = new FileReader();
		reader.onload = () => resolve(String(reader.result));
		reader.onerror = () => reject(reader.error);
		reader.readAsText(file);
	});

afterEach(() => {
	vi.restoreAllMocks();
	localStorage.clear();
});

describe("AgentCreatePage debug deep link", () => {
	it("attaches the build logs, creates the chat, and drops the param from the chat URL", async () => {
		server.use(
			http.get("/api/v2/experiments", () =>
				HttpResponse.json(["enable-ai-workspace-debug"]),
			),
		);
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
		).mockResolvedValue(MockUserChatPersonalModelOverrides);
		vi.spyOn(API.experimental, "getMCPServerConfigs").mockResolvedValue([]);
		vi.spyOn(API, "getUserPreferenceSettings").mockResolvedValue(
			MockUserPreferenceSettings,
		);
		const uploadChatFile = vi
			.spyOn(API.experimental, "uploadChatFile")
			.mockResolvedValue({ id: "uploaded-logs" });
		const createChat = vi
			.spyOn(API.experimental, "createChat")
			.mockResolvedValue({ ...MockChat, id: "new-chat-id" });

		renderWithAuth(<AgentCreatePage />, {
			path: "/agents",
			route: `/agents?${debugWorkspaceBuildSearchParam}=${failedBuild.id}&archived=archived`,
			extraRoutes: [{ path: "/agents/:agentId", element: <LocationProbe /> }],
		});

		await waitFor(() => expect(createChat).toHaveBeenCalledTimes(1));
		const [uploadedFile] = uploadChatFile.mock.calls[0];
		expect(uploadedFile.name).toBe(
			debugWorkspaceBuildLogsFileName(failedBuild),
		);
		expect(await readFileText(uploadedFile)).toBe(
			formatWorkspaceBuildLogsForDebug(failedBuild, MockWorkspaceBuildLogs),
		);
		expect(createChat).toHaveBeenCalledWith(
			expect.objectContaining({
				organization_id: MockDefaultOrganization.id,
				model_config_id: defaultModel.id,
				client_type: "ui",
				content: [
					{ type: "text", text: debugWorkspaceBuildPrompt("start") },
					{ type: "file", file_id: "uploaded-logs" },
				],
			}),
		);
		await screen.findByText("/agents/new-chat-id?archived=archived");
	});
});
