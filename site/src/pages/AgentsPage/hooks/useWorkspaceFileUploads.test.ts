import {
	configure,
	getConfig,
	renderHook as renderHookBase,
	waitFor,
} from "@testing-library/react";
import { act, createElement } from "react";
import { QueryClient, QueryClientProvider } from "react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { UploadChatWorkspaceFileResponse } from "#/api/typesGenerated";
import { mockApiError } from "#/testHelpers/entities";
import { useWorkspaceFileUploads } from "./useWorkspaceFileUploads";

vi.mock("#/api/api", () => ({
	API: {
		experimental: {
			uploadChatWorkspaceFile: vi.fn(),
		},
	},
}));

const { API } = await import("#/api/api");
const uploadMock = vi.mocked(API.experimental.uploadChatWorkspaceFile);

const makeFile = (name = "doc.bin", type = "application/x-tar"): File =>
	new File([new Uint8Array(16)], name, { type });

const okResponse = {
	path: "/home/coder/.coder/chats/chat-1/files/doc.bin",
	name: "doc.bin",
	size: 16,
	media_type: "application/x-tar",
	workspace_id: "ws-1",
};

const renderHook: typeof renderHookBase = (callback, options) => {
	const queryClient = new QueryClient({
		defaultOptions: { mutations: { retry: false } },
	});
	return renderHookBase(callback, {
		...options,
		wrapper: ({ children }: { children: React.ReactNode }) =>
			createElement(QueryClientProvider, { client: queryClient }, children),
	});
};

describe("useWorkspaceFileUploads", () => {
	beforeEach(() => {
		uploadMock.mockReset();
		localStorage.clear();
	});

	it("transitions an upload from uploading to uploaded", async () => {
		uploadMock.mockResolvedValueOnce(okResponse);
		const { result } = renderHook(() =>
			useWorkspaceFileUploads("chat-1", "ws-1"),
		);

		act(() => {
			result.current.attach([makeFile()]);
		});

		expect(result.current.uploads).toHaveLength(1);
		expect(result.current.uploads[0].status).toBe("uploading");

		await waitFor(() => {
			expect(result.current.uploads[0].status).toBe("uploaded");
		});
		expect(result.current.uploads[0].response).toEqual(okResponse);
		expect(uploadMock).toHaveBeenCalledWith(
			"chat-1",
			expect.any(File),
			expect.any(AbortSignal),
		);
	});

	it("records the server message and detail when the upload fails", async () => {
		uploadMock.mockRejectedValueOnce(
			Object.assign(
				new Error("Request failed with status code 502"),
				mockApiError({
					message: "Failed to upload file to workspace agent.",
					detail: "The workspace agent could not be reached.",
				}),
			),
		);
		const { result } = renderHook(() =>
			useWorkspaceFileUploads("chat-1", "ws-1"),
		);

		act(() => {
			result.current.attach([makeFile()]);
		});

		await waitFor(() => {
			expect(result.current.uploads[0].status).toBe("error");
		});
		expect(result.current.uploads[0].error).toBe(
			"Failed to upload file to workspace agent. The workspace agent could not be reached.",
		);
	});

	it("marks files beyond the concurrency limit as queued", () => {
		uploadMock.mockImplementation(() => new Promise(() => {}));
		const { result } = renderHook(() =>
			useWorkspaceFileUploads("chat-1", "ws-1"),
		);

		act(() => {
			result.current.attach([
				makeFile("a.bin"),
				makeFile("b.bin"),
				makeFile("c.bin"),
				makeFile("d.bin"),
			]);
		});

		expect(result.current.uploads.map((upload) => upload.status)).toEqual([
			"uploading",
			"uploading",
			"uploading",
			"queued",
		]);
	});

	it("queues entries without a chat id until uploadQueued runs them", async () => {
		uploadMock.mockResolvedValue(okResponse);
		const { result } = renderHook(() =>
			useWorkspaceFileUploads(undefined, undefined),
		);

		act(() => {
			result.current.attach([makeFile("a.tar"), makeFile("b.tar")]);
		});

		expect(result.current.uploads).toHaveLength(2);
		expect(result.current.uploads.every((u) => u.status === "deferred")).toBe(
			true,
		);
		expect(uploadMock).not.toHaveBeenCalled();

		let settled: readonly { status: string }[] = [];
		await act(async () => {
			settled = await result.current.uploadQueued("chat-9");
		});

		expect(settled).toHaveLength(2);
		expect(settled.every((u) => u.status === "uploaded")).toBe(true);
		expect(result.current.uploads.every((u) => u.status === "uploaded")).toBe(
			true,
		);
		expect(uploadMock).toHaveBeenCalledWith(
			"chat-9",
			expect.any(File),
			expect.any(AbortSignal),
		);
	});

	it("uploadQueued proceeds after a StrictMode simulated unmount", async () => {
		// StrictMode only replays mount effects for the root or a newly
		// placed subtree inside it, so a StrictMode wrapper component
		// around the hook would not exercise the remount.
		const previousStrictMode = getConfig().reactStrictMode;
		configure({ reactStrictMode: true });
		try {
			uploadMock.mockResolvedValue(okResponse);
			const { result } = renderHook(() =>
				useWorkspaceFileUploads(undefined, undefined),
			);

			act(() => {
				result.current.attach([makeFile("a.tar")]);
			});

			let settled: readonly { status: string }[] = [];
			await act(async () => {
				settled = await result.current.uploadQueued("chat-9");
			});

			expect(settled).toHaveLength(1);
			expect(settled[0].status).toBe("uploaded");
		} finally {
			configure({ reactStrictMode: previousStrictMode });
		}
	});

	it("does not upload an entry removed before a deferred callback runs", async () => {
		uploadMock.mockResolvedValue(okResponse);
		const { result } = renderHook(() =>
			useWorkspaceFileUploads(undefined, undefined),
		);

		act(() => {
			result.current.attach([makeFile()]);
		});
		const uploadQueued = result.current.uploadQueued;
		const [entry] = result.current.uploads;

		act(() => {
			result.current.remove(entry.id);
		});

		let settled: readonly { status: string }[] = [];
		await act(async () => {
			settled = await uploadQueued("chat-9");
		});

		expect(settled).toHaveLength(0);
		expect(uploadMock).not.toHaveBeenCalled();
	});

	it("rejects a deferred callback invalidated before it runs", async () => {
		uploadMock.mockResolvedValue(okResponse);
		const { result } = renderHook(() =>
			useWorkspaceFileUploads(undefined, undefined),
		);

		act(() => {
			result.current.attach([makeFile()]);
		});
		const uploadQueued = result.current.uploadQueued;

		act(() => {
			result.current.reset();
		});

		await expect(uploadQueued("chat-9")).rejects.toMatchObject({
			name: "AbortError",
		});
		expect(uploadMock).not.toHaveBeenCalled();
	});

	it("rejects uploadQueued when reset cancels an in-flight submission", async () => {
		let capturedSignal: AbortSignal | undefined;
		uploadMock.mockImplementationOnce(
			(_chatId: string, _file: File, signal?: AbortSignal) => {
				capturedSignal = signal;
				return new Promise((_resolve, reject) => {
					signal?.addEventListener(
						"abort",
						() => reject(new DOMException("aborted", "AbortError")),
						{ once: true },
					);
				});
			},
		);
		const { result } = renderHook(() =>
			useWorkspaceFileUploads(undefined, undefined),
		);

		act(() => {
			result.current.attach([makeFile()]);
		});
		const settledPromise = result.current.uploadQueued("chat-9");
		await waitFor(() => {
			expect(uploadMock).toHaveBeenCalledOnce();
		});

		let settledError: unknown;
		await act(async () => {
			result.current.reset();
			try {
				await settledPromise;
			} catch (error) {
				settledError = error;
			}
		});

		expect(settledError).toMatchObject({ name: "AbortError" });
		expect(capturedSignal?.aborted).toBe(true);
	});

	it("uploadQueued reports per-file failures and keeps chips in error state", async () => {
		uploadMock
			.mockResolvedValueOnce(okResponse)
			.mockRejectedValueOnce(new Error("boom"));
		const { result } = renderHook(() =>
			useWorkspaceFileUploads(undefined, undefined),
		);

		act(() => {
			result.current.attach([makeFile("a.tar"), makeFile("b.tar")]);
		});

		let settled: readonly { status: string }[] = [];
		await act(async () => {
			settled = await result.current.uploadQueued("chat-9");
		});

		expect(settled).toHaveLength(2);
		expect(settled.filter((u) => u.status === "uploaded")).toHaveLength(1);
		expect(settled.filter((u) => u.status === "error")).toHaveLength(1);
		expect(
			result.current.uploads.filter((u) => u.status === "error"),
		).toHaveLength(1);
	});

	it("uploadQueued re-uploads every entry on retry", async () => {
		uploadMock
			.mockRejectedValueOnce(new Error("boom"))
			.mockResolvedValue(okResponse);
		const { result } = renderHook(() =>
			useWorkspaceFileUploads(undefined, undefined),
		);

		act(() => {
			result.current.attach([makeFile()]);
		});

		await act(async () => {
			await result.current.uploadQueued("chat-1");
		});
		expect(result.current.uploads[0].status).toBe("error");

		// The retry targets a fresh chat, so the failed entry uploads
		// again rather than being skipped.
		let settled: readonly { status: string }[] = [];
		await act(async () => {
			settled = await result.current.uploadQueued("chat-2");
		});

		expect(settled).toHaveLength(1);
		expect(settled[0].status).toBe("uploaded");
		expect(uploadMock).toHaveBeenLastCalledWith(
			"chat-2",
			expect.any(File),
			expect.any(AbortSignal),
		);
	});

	it("uploadQueued excludes entries removed mid-upload", async () => {
		let resolveUpload:
			| ((value: UploadChatWorkspaceFileResponse) => void)
			| undefined;
		uploadMock.mockImplementationOnce(
			() =>
				new Promise((resolve) => {
					resolveUpload = resolve;
				}),
		);
		uploadMock.mockResolvedValueOnce(okResponse);
		const { result } = renderHook(() =>
			useWorkspaceFileUploads(undefined, undefined),
		);

		act(() => {
			result.current.attach([makeFile("a.tar"), makeFile("b.tar")]);
		});
		const [first] = result.current.uploads;

		let settledPromise: Promise<readonly { status: string }[]> | undefined;
		act(() => {
			settledPromise = result.current.uploadQueued("chat-1");
		});
		act(() => {
			result.current.remove(first.id);
		});
		resolveUpload?.(okResponse);

		const settled = await settledPromise;
		expect(settled).toHaveLength(1);
		expect(settled?.[0].status).toBe("uploaded");
		expect(result.current.uploads).toHaveLength(1);
	});

	it("uploadQueued excludes entries removed after their upload completed", async () => {
		let resolveSecond:
			| ((value: UploadChatWorkspaceFileResponse) => void)
			| undefined;
		uploadMock.mockResolvedValueOnce(okResponse);
		uploadMock.mockImplementationOnce(
			() =>
				new Promise((resolve) => {
					resolveSecond = resolve;
				}),
		);
		const { result } = renderHook(() =>
			useWorkspaceFileUploads(undefined, undefined),
		);

		act(() => {
			result.current.attach([makeFile("a.tar"), makeFile("b.tar")]);
		});
		const [first] = result.current.uploads;

		let settledPromise:
			| Promise<readonly { id: string; status: string }[]>
			| undefined;
		act(() => {
			settledPromise = result.current.uploadQueued("chat-1");
		});
		// The first file settles as uploaded while the second is still
		// in flight; removing its chip now must drop it from the final
		// results even though its settle promise already resolved.
		await waitFor(() => {
			expect(result.current.uploads[0]?.status).toBe("uploaded");
		});
		act(() => {
			result.current.remove(first.id);
		});
		resolveSecond?.(okResponse);

		const settled = await settledPromise;
		expect(settled).toHaveLength(1);
		expect(settled?.[0].id).not.toBe(first.id);
		expect(result.current.uploads).toHaveLength(1);
	});

	it("remove aborts an in-flight upload and drops the entry", async () => {
		let capturedSignal: AbortSignal | undefined;
		uploadMock.mockImplementationOnce(
			(_chatId: string, _file: File, signal?: AbortSignal) => {
				capturedSignal = signal;
				return new Promise(() => {
					// Never resolves; the abort is what ends it.
				});
			},
		);
		const { result } = renderHook(() =>
			useWorkspaceFileUploads("chat-1", "ws-1"),
		);

		act(() => {
			result.current.attach([makeFile()]);
		});
		const [entry] = result.current.uploads;

		act(() => {
			result.current.remove(entry.id);
		});

		expect(result.current.uploads).toHaveLength(0);
		await waitFor(() => {
			expect(capturedSignal?.aborted).toBe(true);
		});
	});

	it("keeps uploaded entries when another entry is removed", async () => {
		uploadMock.mockResolvedValue(okResponse);
		const { result } = renderHook(() =>
			useWorkspaceFileUploads("chat-1", "ws-1"),
		);

		act(() => {
			result.current.attach([makeFile("a.tar"), makeFile("b.tar")]);
		});

		await waitFor(() => {
			expect(result.current.uploads.every((u) => u.status === "uploaded")).toBe(
				true,
			);
		});

		act(() => {
			result.current.remove(result.current.uploads[0].id);
		});

		expect(result.current.uploads).toHaveLength(1);
		expect(result.current.uploads[0].file.name).toBe("b.tar");
	});

	it.each([
		{ scope: "chat", next: { chatId: "chat-2", workspaceId: "ws-1" } },
		{ scope: "workspace", next: { chatId: "chat-1", workspaceId: "ws-2" } },
	])(
		"restores completed uploads when returning to a $scope",
		async ({ next }) => {
			uploadMock.mockResolvedValue(okResponse);
			const initialProps = { chatId: "chat-1", workspaceId: "ws-1" };
			const { result, rerender } = renderHook(
				({ chatId, workspaceId }: typeof initialProps) =>
					useWorkspaceFileUploads(chatId, workspaceId, { persist: true }),
				{ initialProps },
			);

			act(() => {
				result.current.attach([makeFile()]);
			});
			await waitFor(() => {
				expect(result.current.uploads[0].status).toBe("uploaded");
			});

			rerender(next);
			expect(result.current.uploads).toEqual([]);

			rerender(initialProps);
			expect(result.current.uploads).toEqual([
				expect.objectContaining({ status: "uploaded", response: okResponse }),
			]);
			expect(uploadMock).toHaveBeenCalledOnce();
		},
	);

	it("ignores malformed and mismatched workspace references in a saved draft", () => {
		localStorage.setItem(
			"agents.workspace-upload-draft.chat-1.ws-1",
			JSON.stringify([
				null,
				{ ...okResponse, name: 42 },
				{ ...okResponse, size: -1 },
				{ ...okResponse, workspace_id: "ws-2" },
				okResponse,
			]),
		);

		const { result } = renderHook(() =>
			useWorkspaceFileUploads("chat-1", "ws-1", { persist: true }),
		);

		expect(result.current.uploads).toEqual([
			expect.objectContaining({ status: "uploaded", response: okResponse }),
		]);
		expect(uploadMock).not.toHaveBeenCalled();
	});

	it("uploads to the new chat while stale uploads are still settling", async () => {
		// Saturate the worker slots so the next chat must start uploads
		// before the previous chat's requests settle.
		const resolveOldUploads: ((
			value: UploadChatWorkspaceFileResponse,
		) => void)[] = [];
		const mockNewChatResponse = {
			...okResponse,
			path: "/home/coder/.coder/chats/chat-2/files/new.tar",
			name: "new.tar",
		};
		uploadMock.mockImplementation((chatId: string) => {
			if (chatId === "chat-1") {
				return new Promise((resolve) => resolveOldUploads.push(resolve));
			}
			return Promise.resolve(mockNewChatResponse);
		});
		const { result, rerender } = renderHook(
			({ chatId }: { chatId: string }) =>
				useWorkspaceFileUploads(chatId, "ws-1", { persist: true }),
			{ initialProps: { chatId: "chat-1" } },
		);

		act(() => {
			result.current.attach([
				makeFile("a.tar"),
				makeFile("b.tar"),
				makeFile("c.tar"),
			]);
		});
		await waitFor(() => {
			expect(uploadMock).toHaveBeenCalledTimes(3);
		});

		rerender({ chatId: "chat-2" });
		await waitFor(() => {
			expect(result.current.uploads).toHaveLength(0);
		});

		act(() => {
			result.current.attach([makeFile("new.tar")]);
		});

		await waitFor(() => {
			expect(result.current.uploads[0]?.status).toBe("uploaded");
		});
		expect(uploadMock).toHaveBeenLastCalledWith(
			"chat-2",
			expect.any(File),
			expect.any(AbortSignal),
		);

		await act(async () => {
			for (const resolve of resolveOldUploads) {
				resolve(okResponse);
			}
		});

		rerender({ chatId: "chat-1" });
		expect(result.current.uploads).toEqual([]);
		rerender({ chatId: "chat-2" });
		expect(result.current.uploads).toEqual([
			expect.objectContaining({
				status: "uploaded",
				response: mockNewChatResponse,
			}),
		]);
	});

	it("aborts in-flight uploads on unmount without restoring unfinished files", async () => {
		let capturedSignal: AbortSignal | undefined;
		uploadMock.mockResolvedValueOnce(okResponse);
		uploadMock.mockImplementationOnce(
			(_chatId: string, _file: File, signal?: AbortSignal) => {
				capturedSignal = signal;
				return new Promise(() => {
					// Never resolves; the abort is what ends it.
				});
			},
		);
		const { result, unmount } = renderHook(() =>
			useWorkspaceFileUploads("chat-1", "ws-1", { persist: true }),
		);

		act(() => {
			result.current.attach([makeFile(), makeFile("pending.tar")]);
		});
		await waitFor(() => {
			expect(result.current.uploads.map((upload) => upload.status)).toEqual([
				"uploaded",
				"uploading",
			]);
		});

		unmount();
		expect(capturedSignal?.aborted).toBe(true);

		const restored = renderHook(() =>
			useWorkspaceFileUploads("chat-1", "ws-1", { persist: true }),
		);
		expect(restored.result.current.uploads).toEqual([
			expect.objectContaining({ status: "uploaded", response: okResponse }),
		]);
		expect(uploadMock).toHaveBeenCalledTimes(2);
	});
});
