import { renderHook as renderHookBase, waitFor } from "@testing-library/react";
import { act, createElement, type ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";
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

	it("errors immediately without a chat id", () => {
		const { result } = renderHook(() =>
			useWorkspaceFileUploads(undefined, "ws-1"),
		);

		act(() => {
			result.current.attach([makeFile()]);
		});

		expect(result.current.uploads[0].status).toBe("error");
		expect(uploadMock).not.toHaveBeenCalled();
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
