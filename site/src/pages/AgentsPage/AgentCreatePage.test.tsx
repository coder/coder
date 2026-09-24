import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import type { FC } from "react";
import { onlineManager } from "react-query";
import { type InitialEntry, useLocation, useSearchParams } from "react-router";
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
	MockChatModelProviderDescriptor,
	MockDefaultChatModel,
	MockUnsetUserChatPersonalModelOverrides,
} from "#/testHelpers/chatModels";
import { createDeferred } from "#/testHelpers/deferred";
import {
	MockFailedWorkspaceBuildWithUUID,
	MockUserPreferenceSettings,
	MockWorkspaceBuildLogs,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import AgentCreatePage from "./AgentCreatePage";
import { emptyInputStorageKey } from "./components/AgentCreateForm";
import { getAgentSidebarFilters } from "./utils/agentSidebarFilters";
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

const renderPage = (route: InitialEntry = deepLink) =>
	renderWithAuth(<AgentCreatePage />, {
		path: "/agents",
		route,
		extraRoutes: [{ path: "/agents/:agentId", element: null }],
	});

// Writes a sidebar filter the way the layout does.
const SidebarFilterProbe: FC = () => {
	const [searchParams, setSearchParams] = useSearchParams();
	const location = useLocation();
	const [filters, setFilters] = getAgentSidebarFilters(
		searchParams,
		setSearchParams,
		location.state,
	);
	return (
		<button
			type="button"
			onClick={() => setFilters({ ...filters, groupBy: "chat_status" })}
		>
			Group by status
		</button>
	);
};

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
	onlineManager.setOnline(true);
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

		const sendButton = await findEnabledSendButton();
		expect(uploadChatFile).toHaveBeenCalledTimes(1);
		expect(createChat).not.toHaveBeenCalled();
		expect(router.state.location).toMatchObject({
			pathname: "/agents",
			search: "?archived=archived",
			state: { debugWorkspaceBuild: failedBuild.id },
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

	it("keeps the prefill across a reload through history state, without sending", async () => {
		enableExperiment();
		const { uploadChatFile, createChat } = mockPageQueries();

		renderPage({
			pathname: "/agents",
			search: "?archived=archived",
			state: { debugWorkspaceBuild: failedBuild.id },
		});

		await findEnabledSendButton();
		expect(uploadChatFile).toHaveBeenCalledTimes(1);
		expect(createChat).not.toHaveBeenCalled();
	});

	it("drops the prefill on New chat and does not send again on Back", async () => {
		enableExperiment();
		const { uploadChatFile, createChat } = mockPageQueries();
		localStorage.setItem(emptyInputStorageKey, "draft the user typed earlier");
		storeDebugWorkspaceBuildIntent(failedBuild.id);
		createChat.mockRejectedValue(new Error("server error"));

		const { router } = renderPage();

		await waitFor(() => expect(createChat).toHaveBeenCalledTimes(1));
		await screen.findByText("server error");

		// Stands in for the layout's New chat link.
		await router.navigate({
			pathname: "/agents",
			search: "?archived=archived",
		});
		await waitFor(() =>
			expect(
				screen.getByRole("textbox", { name: "Chat message" }),
			).toHaveTextContent("draft the user typed earlier"),
		);
		expect(localStorage.getItem(emptyInputStorageKey)).toContain(
			"draft the user typed earlier",
		);

		await router.navigate(-1);

		await waitFor(() => expect(uploadChatFile).toHaveBeenCalledTimes(2));
		await findEnabledSendButton();
		expect(createChat).toHaveBeenCalledTimes(1);
	});

	it("keeps the prefill and the pending send across a sidebar filter change", async () => {
		enableExperiment();
		const { uploadChatFile, createChat } = mockPageQueries();
		const upload = createDeferred<TypesGen.UploadChatFileResponse>();
		uploadChatFile.mockReturnValue(upload.promise);
		storeDebugWorkspaceBuildIntent(failedBuild.id);
		const user = userEvent.setup();

		const { router } = renderWithAuth(
			<>
				<SidebarFilterProbe />
				<AgentCreatePage />
			</>,
			{
				path: "/agents",
				route: deepLink,
				extraRoutes: [{ path: "/agents/:agentId", element: null }],
			},
		);

		await waitFor(() => expect(uploadChatFile).toHaveBeenCalledTimes(1));
		await user.click(screen.getByRole("button", { name: "Group by status" }));
		await waitFor(() =>
			expect(router.state.location).toMatchObject({
				search: "?archived=archived&group_by=chat_status",
				state: { debugWorkspaceBuild: failedBuild.id },
			}),
		);

		await act(async () => {
			upload.resolve({ id: "uploaded-logs" });
		});

		await waitFor(() => expect(createChat).toHaveBeenCalledTimes(1));
		expect(createChat).toHaveBeenCalledWith(
			expect.objectContaining({
				content: [
					{ type: "text", text: debugWorkspaceBuildPrompt(failedBuild) },
					{ type: "file", file_id: "uploaded-logs" },
				],
			}),
		);
		expect(uploadChatFile).toHaveBeenCalledTimes(1);
	});

	it("does not refetch the build on Back, and a failed refetch is not a load error", async () => {
		enableExperiment();
		const { uploadChatFile, createChat } = mockPageQueries();
		const getWorkspaceBuild = vi
			.spyOn(API, "getWorkspaceBuild")
			.mockResolvedValueOnce(failedBuild)
			.mockRejectedValue(new Error("boom"));
		localStorage.setItem(emptyInputStorageKey, "draft the user typed earlier");
		const user = userEvent.setup();

		const { router, queryClient } = renderPage();

		await findEnabledSendButton();
		// Stands in for the layout's New chat link.
		await router.navigate({
			pathname: "/agents",
			search: "?archived=archived",
		});
		await waitFor(() =>
			expect(
				screen.getByRole("textbox", { name: "Chat message" }),
			).toHaveTextContent("draft the user typed earlier"),
		);

		await router.navigate(-1);

		await waitFor(() => expect(uploadChatFile).toHaveBeenCalledTimes(2));
		const sendButton = await findEnabledSendButton();
		expect(getWorkspaceBuild).toHaveBeenCalledTimes(1);

		await act(async () => {
			await queryClient.invalidateQueries({
				queryKey: ["workspaceBuilds", failedBuild.id],
				exact: true,
			});
			// React Query delivers the result to the page on a timer.
			await new Promise((resolve) => setTimeout(resolve, 0));
		});

		expect(getWorkspaceBuild).toHaveBeenCalledTimes(2);
		expect(
			screen.queryByText(
				"Could not load the workspace build or its logs. Nothing was sent.",
			),
		).toBeNull();
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
		const sendButton = await findEnabledSendButton();
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
			"Could not load the workspace build or its logs. Nothing was sent.",
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
			"Could not load the workspace build or its logs. Nothing was sent.",
		);
		await screen.findByText("logs unavailable");
		await findChatMessage();
		expect(uploadChatFile).not.toHaveBeenCalled();
		expect(createChat).not.toHaveBeenCalled();
	});
});
