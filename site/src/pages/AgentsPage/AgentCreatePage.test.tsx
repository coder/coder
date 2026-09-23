import { act, waitFor } from "@testing-library/react";
import { toast } from "sonner";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { MockChat } from "#/testHelpers/chatEntities";
import { MockDefaultOrganization, mockApiError } from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import AgentCreatePage from "./AgentCreatePage";
import type { CreateChatOptions } from "./components/AgentCreateForm";
import type { WorkspaceFileUpload } from "./hooks/useWorkspaceFileUploads";

const formProps = vi.hoisted(() => ({
	onCreateChat: undefined as
		| ((options: CreateChatOptions) => Promise<void>)
		| undefined,
}));

vi.mock("./components/AgentCreateForm", () => ({
	AgentCreateForm: (props: {
		onCreateChat: (options: CreateChatOptions) => Promise<void>;
	}) => {
		formProps.onCreateChat = props.onCreateChat;
		return null;
	},
}));

vi.mock("./components/AgentPageHeader", () => ({
	AgentPageHeader: () => null,
}));

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

const renderPage = async () => {
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

describe("AgentCreatePage", () => {
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

	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("creates an idle chat, uploads, then sends the first message", async () => {
		const router = await renderPage();
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
		const router = await renderPage();

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
		const router = await renderPage();
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
		const router = await renderPage();
		const abortError = new Error("The upload was aborted.");
		abortError.name = "AbortError";
		const uploadWorkspaceFiles = vi.fn().mockRejectedValue(abortError);

		await expect(submit({ uploadWorkspaceFiles })).rejects.toBe(abortError);

		await waitFor(() => expect(events).toEqual(["create", "archive"]));
		expect(toast.error).not.toHaveBeenCalled();
		expect(router.state.location.pathname).toBe("/agents");
	});

	it("archives the chat when an upload entry failed", async () => {
		const router = await renderPage();
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
		await renderPage();
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
		const router = await renderPage();
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
		await renderPage();
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
		const router = await renderPage();
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
