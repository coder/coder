import {
	configure,
	getConfig,
	renderHook as renderHookBase,
	waitFor,
} from "@testing-library/react";
import { act, createElement, type ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { UploadChatWorkspaceFileResponse } from "#/api/typesGenerated";
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
		wrapper: ({ children }: { children: ReactNode }) =>
			createElement(QueryClientProvider, { client: queryClient }, children),
	});
};

describe("useWorkspaceFileUploads", () => {
	beforeEach(() => {
		uploadMock.mockReset();
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

	it("records an error when the upload fails", async () => {
		uploadMock.mockRejectedValueOnce(new Error("boom"));
		const { result } = renderHook(() =>
			useWorkspaceFileUploads("chat-1", "ws-1"),
		);

		act(() => {
			result.current.attach([makeFile()]);
		});

		await waitFor(() => {
			expect(result.current.uploads[0].status).toBe("error");
		});
		expect(result.current.uploads[0].error).toBeTruthy();
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
		expect(result.current.uploads.every((u) => u.status === "queued")).toBe(
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

	it("resets pending uploads when the chat changes", async () => {
		uploadMock.mockResolvedValue(okResponse);
		const { result, rerender } = renderHook(
			({ chatId }: { chatId: string }) =>
				useWorkspaceFileUploads(chatId, "ws-1"),
			{ initialProps: { chatId: "chat-1" } },
		);

		act(() => {
			result.current.attach([makeFile()]);
		});
		await waitFor(() => {
			expect(result.current.uploads[0].status).toBe("uploaded");
		});

		rerender({ chatId: "chat-2" });

		await waitFor(() => {
			expect(result.current.uploads).toHaveLength(0);
		});
	});

	it("resets uploads when the bound workspace changes", async () => {
		uploadMock.mockResolvedValue(okResponse);
		const initialProps: { workspaceId: string | undefined } = {
			workspaceId: "ws-1",
		};
		const { result, rerender } = renderHook(
			({ workspaceId }: typeof initialProps) =>
				useWorkspaceFileUploads("chat-1", workspaceId),
			{ initialProps },
		);

		act(() => {
			result.current.attach([makeFile()]);
		});
		await waitFor(() => {
			expect(result.current.uploads[0].status).toBe("uploaded");
		});

		rerender({ workspaceId: "ws-2" });

		await waitFor(() => {
			expect(result.current.uploads).toHaveLength(0);
		});
	});

	it("uploads to the new chat while stale uploads are still settling", async () => {
		// Saturate all worker slots with chat-1 uploads that never
		// settle, so stale workers still hold slots when chat-2 files
		// arrive. Without generation-scoped slot accounting the chat-2
		// upload would never start (or a stale chat-1 worker would
		// steal it).
		uploadMock.mockImplementation((chatId: string) => {
			if (chatId === "chat-1") {
				return new Promise(() => {
					// Never resolves; the abort is what ends it.
				});
			}
			return Promise.resolve(okResponse);
		});
		const { result, rerender } = renderHook(
			({ chatId }: { chatId: string }) =>
				useWorkspaceFileUploads(chatId, "ws-1"),
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
	});

	it("aborts in-flight uploads on unmount", async () => {
		let capturedSignal: AbortSignal | undefined;
		uploadMock.mockImplementationOnce(
			(_chatId: string, _file: File, signal?: AbortSignal) => {
				capturedSignal = signal;
				return new Promise(() => {
					// Never resolves; the abort is what ends it.
				});
			},
		);
		const { result, unmount } = renderHook(() =>
			useWorkspaceFileUploads("chat-1", "ws-1"),
		);

		act(() => {
			result.current.attach([makeFile()]);
		});

		unmount();

		await waitFor(() => {
			expect(capturedSignal?.aborted).toBe(true);
		});
	});
});
