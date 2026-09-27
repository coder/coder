import { renderHook, waitFor } from "@testing-library/react";
import { act, createElement, type ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "react-query";
import { toast } from "sonner";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { mockApiError } from "#/testHelpers/entities";
import {
	getParkedWorkspaceUploads,
	parkWorkspaceUploads,
} from "../utils/parkedWorkspaceUploads";
import { useParkedWorkspaceUploads } from "./useParkedWorkspaceUploads";

vi.mock("#/api/api", () => ({
	API: {
		experimental: {
			uploadChatWorkspaceFile: vi.fn(),
			createChatMessage: vi.fn(),
		},
	},
}));

const { API } = await import("#/api/api");
const uploadMock = vi.mocked(API.experimental.uploadChatWorkspaceFile);
const sendMock = vi.mocked(API.experimental.createChatMessage);

const makeFile = (name: string): File =>
	new File([new Uint8Array(8)], name, { type: "application/gzip" });

const uploadResponse = (name: string) => ({
	path: `/home/coder/.coder/chats/chat-1/files/${name}`,
	name,
	size: 8,
	media_type: "application/gzip",
	workspace_id: "ws-1",
});

const renderParked = (canUpload: boolean) => {
	const queryClient = new QueryClient({
		defaultOptions: { mutations: { retry: false } },
	});
	return renderHook(
		({ canUpload }) => useParkedWorkspaceUploads("chat-1", canUpload),
		{
			initialProps: { canUpload },
			wrapper: ({ children }: { children: ReactNode }) =>
				createElement(QueryClientProvider, { client: queryClient }, children),
		},
	);
};

describe("useParkedWorkspaceUploads", () => {
	beforeEach(() => {
		uploadMock.mockReset();
		sendMock.mockReset();
		sendMock.mockResolvedValue({ queued: true });
		vi.spyOn(toast, "error");
	});

	afterEach(() => {
		vi.restoreAllMocks();
		parkWorkspaceUploads("chat-1", []);
	});

	it("uploads parked files once the workspace connects and sends them queued", async () => {
		parkWorkspaceUploads("chat-1", [makeFile("logs.tar.gz")]);
		uploadMock.mockResolvedValueOnce(uploadResponse("logs.tar.gz"));
		const { result, rerender } = renderParked(false);

		await waitFor(() =>
			expect(result.current.uploads.map((upload) => upload.status)).toEqual([
				"deferred",
			]),
		);
		expect(uploadMock).not.toHaveBeenCalled();

		rerender({ canUpload: true });

		await waitFor(() => expect(sendMock).toHaveBeenCalledTimes(1));
		expect(uploadMock).toHaveBeenCalledWith(
			"chat-1",
			expect.objectContaining({ name: "logs.tar.gz" }),
			expect.any(AbortSignal),
		);
		expect(sendMock).toHaveBeenCalledWith("chat-1", {
			busy_behavior: "queue",
			content: [
				{
					type: "workspace-file-reference",
					workspace_file_path:
						"/home/coder/.coder/chats/chat-1/files/logs.tar.gz",
					workspace_file_name: "logs.tar.gz",
					workspace_file_size: 8,
					workspace_file_media_type: "application/gzip",
					workspace_file_workspace_id: "ws-1",
				},
			],
		});
		await waitFor(() => expect(result.current.uploads).toEqual([]));
		expect(getParkedWorkspaceUploads("chat-1")).toEqual([]);
	});

	it("forgets a parked file removed before the workspace starts", async () => {
		parkWorkspaceUploads("chat-1", [
			makeFile("a.tar.gz"),
			makeFile("b.tar.gz"),
		]);
		const { result } = renderParked(false);
		await waitFor(() => expect(result.current.uploads).toHaveLength(2));

		act(() => {
			result.current.remove(result.current.uploads[0].id);
		});

		expect(result.current.uploads.map((upload) => upload.file.name)).toEqual([
			"b.tar.gz",
		]);
		expect(
			getParkedWorkspaceUploads("chat-1").map((file) => file.name),
		).toEqual(["b.tar.gz"]);
	});

	it("sends only the files that uploaded", async () => {
		parkWorkspaceUploads("chat-1", [
			makeFile("ok.tar.gz"),
			makeFile("bad.tar.gz"),
		]);
		uploadMock.mockImplementation(async (_chatId, file) => {
			if (file.name === "bad.tar.gz") {
				throw mockApiError({ message: "disk full" });
			}
			return uploadResponse(file.name);
		});
		renderParked(true);

		await waitFor(() => expect(sendMock).toHaveBeenCalledTimes(1));
		expect(
			sendMock.mock.calls[0][1].content.map((part) =>
				part.type === "workspace-file-reference"
					? part.workspace_file_name
					: "",
			),
		).toEqual(["ok.tar.gz"]);
		expect(toast.error).toHaveBeenCalledWith(
			"Failed to upload to the workspace: bad.tar.gz",
		);
	});
});
