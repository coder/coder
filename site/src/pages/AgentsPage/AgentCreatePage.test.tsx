import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { toast } from "sonner";
import {
	afterEach,
	beforeAll,
	beforeEach,
	describe,
	expect,
	it,
	vi,
} from "vitest";
import { API } from "#/api/api";
import { buildDebugWorkspaceBuildPath } from "#/modules/workspaces/workspaceBuildDebugLink";
import { MockChat } from "#/testHelpers/chatEntities";
import {
	MockChatModelProviderDescriptor,
	MockDefaultChatModel,
	MockUnsetUserChatPersonalModelOverrides,
} from "#/testHelpers/chatModels";
import {
	MockDefaultOrganization,
	MockFailedWorkspaceBuild,
	MockUserPreferenceSettings,
	MockWorkspaceBuildLogs,
	mockApiError,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { server } from "#/testHelpers/server";
import AgentCreatePage from "./AgentCreatePage";
import type * as AgentCreateFormModule from "./components/AgentCreateForm";
import {
	type CreateChatOptions,
	emptyInputStorageKey,
} from "./components/AgentCreateForm";
import type { WorkspaceFileUpload } from "./hooks/useWorkspaceFileUploads";
import { readAgentAttachmentText } from "./utils/fileAttachmentLimits";
import {
	debugWorkspaceBuildPrompt,
	formatWorkspaceBuildLogsForDebug,
} from "./utils/workspaceBuildDebug";

const formProps = vi.hoisted(() => ({
	onCreateChat: undefined as
		| ((options: CreateChatOptions) => Promise<void>)
		| undefined,
}));

// Captures onCreateChat so upload tests can drive the page's submit
// path directly while other tests still use the real form.
vi.mock("./components/AgentCreateForm", async (importOriginal) => {
	const actual = await importOriginal<typeof AgentCreateFormModule>();
	return {
		...actual,
		AgentCreateForm: (
			props: React.ComponentProps<typeof actual.AgentCreateForm>,
		) => {
			formProps.onCreateChat = props.onCreateChat;
			return <actual.AgentCreateForm {...props} />;
		},
	};
});

// AgentPageHeader needs the layout's outlet context.
vi.mock("./components/AgentPageHeader", () => ({
	AgentPageHeader: () => null,
}));

const failedBuild = MockFailedWorkspaceBuild();

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

const findEnabledSendButton = async () => {
	const sendButton = await screen.findByRole("button", { name: "Send" });
	await waitFor(() => expect(sendButton).toBeEnabled());
	return sendButton;
};

// Lexical reads selection geometry when text is pasted; jsdom has none.
beforeAll(() => {
	Object.defineProperty(Range.prototype, "getBoundingClientRect", {
		configurable: true,
		value: () => new DOMRect(0, 0, 1, 16),
	});
});

afterEach(() => {
	vi.restoreAllMocks();
	localStorage.clear();
});

describe("AgentCreatePage debug deep link", () => {
	it("prefills the prompt and the build logs, and sends them on Send", async () => {
		enableExperiment();
		const { uploadChatFile, createChat } = mockPageQueries();
		const user = userEvent.setup();

		const { router } = renderPage();

		const sendButton = await findEnabledSendButton();
		expect(uploadChatFile).toHaveBeenCalledTimes(1);
		const [uploadedFile] = uploadChatFile.mock.calls[0];
		expect(uploadedFile.name).toBe(
			"workspace-build-logs-TestUser-test-workspace-1.txt",
		);
		expect(await readAgentAttachmentText(uploadedFile)).toBe(
			formatWorkspaceBuildLogsForDebug(failedBuild, MockWorkspaceBuildLogs),
		);
		expect(createChat).not.toHaveBeenCalled();

		await user.click(sendButton);

		await waitFor(() => expect(createChat).toHaveBeenCalledTimes(1));
		expect(createChat).toHaveBeenCalledWith(
			expect.objectContaining({
				model_config_id: MockDefaultChatModel.id,
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
	});

	it("moves the build ID out of the URL so New chat gets a plain composer", async () => {
		enableExperiment();
		const { uploadChatFile, createChat } = mockPageQueries();
		localStorage.setItem(emptyInputStorageKey, "draft the user typed earlier");
		const user = userEvent.setup();

		const { router } = renderPage();

		await findEnabledSendButton();
		expect(router.state.location).toMatchObject({
			search: "?archived=archived",
			state: { debugWorkspaceBuildId: failedBuild.id },
		});
		// The layout's links forward location.search to a new history entry.
		await router.navigate({
			pathname: "/agents",
			search: router.state.location.search,
		});
		await waitFor(() =>
			expect(
				screen.getByRole("textbox", { name: "Chat message" }),
			).toHaveTextContent("draft the user typed earlier"),
		);

		await user.click(await findEnabledSendButton());

		await waitFor(() => expect(createChat).toHaveBeenCalledTimes(1));
		expect(createChat.mock.calls[0][0].content).toEqual([
			{ type: "text", text: "draft the user typed earlier" },
		]);
		expect(uploadChatFile).toHaveBeenCalledTimes(1);
	});

	it("leaves a plain composer when the build fails to load", async () => {
		enableExperiment();
		const { uploadChatFile, createChat } = mockPageQueries();
		vi.spyOn(API, "getWorkspaceBuild").mockRejectedValue(new Error("boom"));
		const getWorkspaceBuildLogs = vi.spyOn(API, "getWorkspaceBuildLogs");
		const user = userEvent.setup();

		renderPage();

		await screen.findByText("Could not load the workspace build or its logs");
		await user.click(
			await screen.findByRole("textbox", { name: "Chat message" }),
		);
		await user.paste("What happened?");
		await user.click(await findEnabledSendButton());

		await waitFor(() => expect(createChat).toHaveBeenCalledTimes(1));
		expect(createChat.mock.calls[0][0].content).toEqual([
			{ type: "text", text: "What happened?" },
		]);
		expect(getWorkspaceBuildLogs).not.toHaveBeenCalled();
		expect(uploadChatFile).not.toHaveBeenCalled();
	});
});

const mockUploadedFile: WorkspaceFileUpload = {
	id: "upload-1",
	file: new File(["PK"], "bundle.zip", { type: "application/zip" }),
	status: "uploaded",
	response: {
		path: "/home/coder/bundle.zip",
		name: "bundle.zip",
		size: 2,
		media_type: "application/zip",
		workspace_id: "ws-1",
	},
};

const mockConflictError = {
	...mockApiError({ message: "Cannot archive an active chat." }),
	response: {
		status: 409,
		data: { message: "Cannot archive an active chat." },
	},
};

const chatPath = `/agents/${MockChat.id}`;

const renderUploadPage = async () => {
	const { router } = renderWithAuth(<AgentCreatePage />, {
		path: "/agents",
		route: "/agents",
		extraRoutes: [{ path: "/agents/:agentId", element: <div /> }],
	});
	await waitFor(() => expect(formProps.onCreateChat).toBeDefined());
	return router;
};

const submit = (options: Partial<CreateChatOptions>) => {
	const onCreateChat = formProps.onCreateChat;
	if (!onCreateChat) {
		throw new Error("AgentCreateForm was not rendered.");
	}
	return act(() =>
		onCreateChat({
			message: "inspect this archive",
			organizationId: MockDefaultOrganization.id,
			workspaceId: "ws-1",
			...options,
		}),
	);
};

describe("AgentCreatePage workspace uploads", () => {
	let events: string[];

	beforeEach(() => {
		formProps.onCreateChat = undefined;
		events = [];
		vi.spyOn(API.experimental, "createChat").mockImplementation(async () => {
			events.push("create");
			return MockChat;
		});
		vi.spyOn(API.experimental, "createChatMessage").mockImplementation(
			async () => {
				events.push("send");
				return { queued: false };
			},
		);
		vi.spyOn(API.experimental, "updateChat").mockImplementation(async () => {
			events.push("archive");
		});
		vi.spyOn(toast, "error");
	});

	it("creates an idle chat, uploads, then sends the first message", async () => {
		const router = await renderUploadPage();
		const uploadWorkspaceFiles = vi.fn(async (chatId: string) => {
			events.push(`upload:${chatId}`);
			return [mockUploadedFile];
		});

		await submit({ uploadWorkspaceFiles });

		expect(events).toEqual(["create", `upload:${MockChat.id}`, "send"]);
		expect(API.experimental.createChat).toHaveBeenCalledWith(
			expect.objectContaining({ content: [], workspace_id: "ws-1" }),
		);
		expect(API.experimental.createChatMessage).toHaveBeenCalledWith(
			MockChat.id,
			expect.objectContaining({
				content: [
					{ type: "text", text: "inspect this archive" },
					{
						type: "workspace-file-reference",
						workspace_file_path: "/home/coder/bundle.zip",
						workspace_file_name: "bundle.zip",
						workspace_file_size: 2,
						workspace_file_media_type: "application/zip",
						workspace_file_workspace_id: "ws-1",
					},
				],
			}),
		);
		expect(router.state.location.pathname).toBe(chatPath);
	});

	it("sends text-only submits with the create request", async () => {
		const router = await renderUploadPage();

		await submit({});

		expect(events).toEqual(["create"]);
		expect(API.experimental.createChat).toHaveBeenCalledWith(
			expect.objectContaining({
				content: [{ type: "text", text: "inspect this archive" }],
			}),
		);
		expect(router.state.location.pathname).toBe(chatPath);
	});

	it("archives the chat when the upload fails", async () => {
		const router = await renderUploadPage();
		const uploadWorkspaceFiles = vi
			.fn()
			.mockRejectedValue(mockApiError({ message: "Agent unreachable." }));

		await expect(submit({ uploadWorkspaceFiles })).rejects.toBeDefined();

		await waitFor(() => expect(events).toEqual(["create", "archive"]));
		expect(API.experimental.updateChat).toHaveBeenCalledWith(MockChat.id, {
			archived: true,
		});
		expect(toast.error).toHaveBeenCalledWith("Agent unreachable.");
		expect(router.state.location.pathname).toBe("/agents");
	});

	it("archives the chat without an upload toast when the upload is aborted", async () => {
		const router = await renderUploadPage();
		const abortError = new Error("The upload was aborted.");
		abortError.name = "AbortError";
		const uploadWorkspaceFiles = vi.fn().mockRejectedValue(abortError);

		await expect(submit({ uploadWorkspaceFiles })).rejects.toBe(abortError);

		await waitFor(() => expect(events).toEqual(["create", "archive"]));
		expect(toast.error).not.toHaveBeenCalled();
		expect(router.state.location.pathname).toBe("/agents");
	});

	it("archives the chat when an upload entry failed", async () => {
		const router = await renderUploadPage();
		const uploadWorkspaceFiles = vi.fn().mockResolvedValue([
			mockUploadedFile,
			{
				...mockUploadedFile,
				id: "upload-2",
				status: "error",
				response: undefined,
			},
		]);

		await expect(submit({ uploadWorkspaceFiles })).rejects.toBeDefined();

		await waitFor(() => expect(events).toEqual(["create", "archive"]));
		expect(toast.error).toHaveBeenCalledWith(
			"1 file failed to upload to the workspace. Remove or retry the failed files, then send again.",
		);
		expect(router.state.location.pathname).toBe("/agents");
	});

	it("reports a failed cleanup after an upload failure", async () => {
		await renderUploadPage();
		vi.mocked(API.experimental.updateChat).mockRejectedValue(
			mockApiError({ message: "Archive failed." }),
		);
		const uploadWorkspaceFiles = vi
			.fn()
			.mockRejectedValue(mockApiError({ message: "Agent unreachable." }));

		await expect(submit({ uploadWorkspaceFiles })).rejects.toBeDefined();

		await waitFor(() =>
			expect(toast.error).toHaveBeenCalledWith("Archive failed."),
		);
	});

	it("archives the chat when the first message fails", async () => {
		const router = await renderUploadPage();
		vi.mocked(API.experimental.createChatMessage).mockImplementation(
			async () => {
				events.push("send");
				throw mockApiError({ message: "Send failed." });
			},
		);
		const uploadWorkspaceFiles = vi.fn().mockResolvedValue([mockUploadedFile]);

		await expect(submit({ uploadWorkspaceFiles })).rejects.toBeDefined();

		expect(events).toEqual(["create", "send", "archive"]);
		expect(toast.error).toHaveBeenCalledTimes(1);
		expect(toast.error).toHaveBeenCalledWith("Send failed.");
		expect(router.state.location.pathname).toBe("/agents");
	});

	it("reports a failed cleanup after the first message fails", async () => {
		await renderUploadPage();
		vi.mocked(API.experimental.createChatMessage).mockRejectedValue(
			mockApiError({ message: "Send failed." }),
		);
		vi.mocked(API.experimental.updateChat).mockRejectedValue(
			mockApiError({ message: "Archive failed." }),
		);
		const uploadWorkspaceFiles = vi.fn().mockResolvedValue([mockUploadedFile]);

		await expect(submit({ uploadWorkspaceFiles })).rejects.toBeDefined();

		expect(toast.error).toHaveBeenCalledWith("Archive failed.");
		expect(toast.error).toHaveBeenCalledWith("Send failed.");
	});

	it("navigates to the chat when the failed send was committed", async () => {
		const router = await renderUploadPage();
		vi.mocked(API.experimental.createChatMessage).mockRejectedValue(
			mockApiError({ message: "Network Error" }),
		);
		vi.mocked(API.experimental.updateChat).mockRejectedValue(mockConflictError);
		const uploadWorkspaceFiles = vi.fn().mockResolvedValue([mockUploadedFile]);

		await submit({ uploadWorkspaceFiles });

		expect(toast.error).not.toHaveBeenCalled();
		expect(router.state.location.pathname).toBe(chatPath);
	});
});

describe("AgentCreatePage prompt link", () => {
	it("prefills the prompt from a prompt link and sends it only on Send", async () => {
		const { uploadChatFile, createChat } = mockPageQueries();
		const prompt = "Fix the flaky test\nin a & b = c";
		const user = userEvent.setup();

		renderPage(`/agents?prompt=${encodeURIComponent(prompt)}`);

		const sendButton = await findEnabledSendButton();
		expect(createChat).not.toHaveBeenCalled();
		expect(uploadChatFile).not.toHaveBeenCalled();

		await user.click(sendButton);

		await waitFor(() => expect(createChat).toHaveBeenCalledTimes(1));
		expect(createChat.mock.calls[0][0].content).toEqual([
			{ type: "text", text: prompt },
		]);
	});

	it("ignores the prompt when a debug link is also present", async () => {
		enableExperiment();
		const { createChat } = mockPageQueries();
		const user = userEvent.setup();

		const { router } = renderPage(`${deepLink}&prompt=hi`);

		const sendButton = await findEnabledSendButton();
		expect(router.state.location).toMatchObject({
			search: "?archived=archived",
			state: { debugWorkspaceBuildId: failedBuild.id },
		});
		await user.click(sendButton);

		await waitFor(() => expect(createChat).toHaveBeenCalledTimes(1));
		expect(createChat.mock.calls[0][0].content).toEqual([
			{ type: "text", text: debugWorkspaceBuildPrompt(failedBuild) },
			{ type: "file", file_id: "uploaded-logs" },
		]);
	});

	it("moves the prompt out of the URL so New chat gets a plain composer", async () => {
		const { createChat } = mockPageQueries();
		localStorage.setItem(emptyInputStorageKey, "draft the user typed earlier");
		const user = userEvent.setup();

		const { router } = renderPage("/agents?archived=archived&prompt=hi");

		await findEnabledSendButton();
		expect(router.state.location).toMatchObject({
			search: "?archived=archived",
			state: { prompt: "hi" },
		});
		// The layout's links forward location.search to a new history entry.
		await router.navigate({
			pathname: "/agents",
			search: router.state.location.search,
		});
		await waitFor(() =>
			expect(
				screen.getByRole("textbox", { name: "Chat message" }),
			).toHaveTextContent("draft the user typed earlier"),
		);

		await user.click(await findEnabledSendButton());

		await waitFor(() => expect(createChat).toHaveBeenCalledTimes(1));
		expect(createChat.mock.calls[0][0].content).toEqual([
			{ type: "text", text: "draft the user typed earlier" },
		]);
	});
});
