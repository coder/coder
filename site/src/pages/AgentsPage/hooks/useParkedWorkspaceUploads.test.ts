import {
	configure,
	getConfig,
	renderHook,
	waitFor,
} from "@testing-library/react";
import { act, createElement, type ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "react-query";
import { toast } from "sonner";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { UploadChatWorkspaceFileResponse } from "#/api/typesGenerated";
import { createDeferred } from "#/testHelpers/deferred";
import { mockApiError } from "#/testHelpers/entities";
import { createMockFile } from "#/testHelpers/files";
import {
	getParkedWorkspaceUploads,
	parkWorkspaceUploads,
	unparkWorkspaceUploads,
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

const mockUploadResponse: UploadChatWorkspaceFileResponse = {
	path: "/home/coder/.coder/chats/chat-1/files/logs.tar.gz",
	name: "logs.tar.gz",
	size: 8,
	media_type: "application/gzip",
	workspace_id: "ws-1",
};

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
		unparkWorkspaceUploads("chat-1", getParkedWorkspaceUploads("chat-1"));
	});

	it("uploads parked files once the workspace connects and sends them queued", async () => {
		parkWorkspaceUploads("chat-1", [
			createMockFile("logs.tar.gz", "application/gzip"),
		]);
		uploadMock.mockResolvedValueOnce(mockUploadResponse);
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
			expect.any(Function),
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
			createMockFile("a.tar.gz", "application/gzip"),
			createMockFile("b.tar.gz", "application/gzip"),
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

	it("sends only the files that uploaded and keeps the failed one", async () => {
		parkWorkspaceUploads("chat-1", [
			createMockFile("ok.tar.gz", "application/gzip"),
			createMockFile("bad.tar.gz", "application/gzip"),
		]);
		uploadMock.mockImplementation(async (_chatId, file) => {
			if (file.name === "bad.tar.gz") {
				throw mockApiError({ message: "disk full" });
			}
			return { ...mockUploadResponse, name: file.name };
		});
		const { result } = renderParked(true);

		await waitFor(() => expect(sendMock).toHaveBeenCalledTimes(1));
		expect(sentFileNames(0)).toEqual(["ok.tar.gz"]);
		expect(toast.error).toHaveBeenCalledWith(
			"Failed to upload to the workspace: bad.tar.gz",
		);
		await waitFor(() =>
			expect(
				result.current.uploads.map(({ file, status, error }) => ({
					name: file.name,
					status,
					error,
				})),
			).toEqual([{ name: "bad.tar.gz", status: "error", error: "disk full" }]),
		);
	});

	it("uploads a failed file again when retried", async () => {
		parkWorkspaceUploads("chat-1", [
			createMockFile("flaky.tar.gz", "application/gzip"),
		]);
		uploadMock
			.mockRejectedValueOnce(mockApiError({ message: "agent restarted" }))
			.mockResolvedValueOnce({ ...mockUploadResponse, name: "flaky.tar.gz" });
		const { result } = renderParked(true);
		await waitFor(() =>
			expect(result.current.uploads[0]?.status).toBe("error"),
		);
		expect(sendMock).not.toHaveBeenCalled();

		act(() => {
			result.current.retry(result.current.uploads[0].id);
		});

		await waitFor(() => expect(sendMock).toHaveBeenCalledTimes(1));
		expect(sentFileNames(0)).toEqual(["flaky.tar.gz"]);
		await waitFor(() => expect(result.current.uploads).toEqual([]));
	});

	it("uploads files attached after an earlier batch was delivered", async () => {
		parkWorkspaceUploads("chat-1", [
			createMockFile("first.tar.gz", "application/gzip"),
		]);
		uploadMock.mockImplementation(async (_chatId, file) => ({
			...mockUploadResponse,
			name: file.name,
		}));
		const { result, rerender } = renderParked(true);
		await waitFor(() => expect(sendMock).toHaveBeenCalledTimes(1));

		rerender({ canUpload: false });
		act(() => {
			result.current.attach([
				createMockFile("second.tar.gz", "application/gzip"),
			]);
		});
		expect(
			getParkedWorkspaceUploads("chat-1").map((file) => file.name),
		).toEqual(["second.tar.gz"]);
		rerender({ canUpload: true });

		await waitFor(() => expect(sendMock).toHaveBeenCalledTimes(2));
		expect(sentFileNames(1)).toEqual(["second.tar.gz"]);
	});

	it("keeps a file attached during an upload for the next batch", async () => {
		parkWorkspaceUploads("chat-1", [
			createMockFile("first.tar.gz", "application/gzip"),
		]);
		const firstUpload = createDeferred<UploadChatWorkspaceFileResponse>();
		uploadMock.mockImplementation((_chatId, file) =>
			file.name === "first.tar.gz"
				? firstUpload.promise
				: Promise.resolve({ ...mockUploadResponse, name: file.name }),
		);
		const { result } = renderParked(true);
		await waitFor(() => expect(uploadMock).toHaveBeenCalledTimes(1));

		act(() => {
			result.current.attach([
				createMockFile("late.tar.gz", "application/gzip"),
			]);
		});
		act(() => {
			firstUpload.resolve({ ...mockUploadResponse, name: "first.tar.gz" });
		});

		await waitFor(() => expect(sendMock).toHaveBeenCalledTimes(2));
		expect(sentFileNames(0)).toEqual(["first.tar.gz"]);
		expect(sentFileNames(1)).toEqual(["late.tar.gz"]);
	});

	it("uploads files parked before a StrictMode mount with the workspace connected", async () => {
		// StrictMode only replays mount effects for the root, so a wrapper
		// component would not exercise the remount.
		const previousStrictMode = getConfig().reactStrictMode;
		configure({ reactStrictMode: true });
		try {
			parkWorkspaceUploads("chat-1", [
				createMockFile("logs.tar.gz", "application/gzip"),
			]);
			uploadMock.mockResolvedValue(mockUploadResponse);
			renderParked(true);

			await waitFor(() => expect(sendMock).toHaveBeenCalledTimes(1));
			expect(uploadMock).toHaveBeenCalledTimes(1);
		} finally {
			configure({ reactStrictMode: previousStrictMode });
		}
	});

	it("warns before unloading the page while files are parked", async () => {
		const { result } = renderParked(false);
		act(() => {
			result.current.attach([
				createMockFile("logs.tar.gz", "application/gzip"),
			]);
		});
		expect(unloadIsBlocked()).toBe(true);

		act(() => {
			result.current.remove(result.current.uploads[0].id);
		});

		expect(unloadIsBlocked()).toBe(false);
	});
});

const sentFileNames = (call: number) =>
	sendMock.mock.calls[call][1].content.map((part) =>
		part.type === "workspace-file-reference" ? part.workspace_file_name : "",
	);

const unloadIsBlocked = () => {
	const event = new Event("beforeunload", { cancelable: true });
	window.dispatchEvent(event);
	return event.defaultPrevented;
};
