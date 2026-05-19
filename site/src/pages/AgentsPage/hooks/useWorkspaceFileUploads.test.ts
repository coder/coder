import { renderHook, waitFor } from "@testing-library/react";
import { act } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MaxWorkspaceFileSizeBytes } from "#/api/typesGenerated";
import { useWorkspaceFileUploads } from "./useWorkspaceFileUploads";

vi.mock("#/api/api", () => ({
	API: {
		experimental: {
			uploadChatWorkspaceFile: vi.fn(),
		},
	},
}));

const { API } = await import("#/api/api");
const uploadMock = API.experimental.uploadChatWorkspaceFile as ReturnType<
	typeof vi.fn
>;

const makeFile = (name = "doc.txt", size = 16, type = "text/plain"): File => {
	const file = new File([new Uint8Array(size)], name, { type });
	Object.defineProperty(file, "size", { value: size });
	return file;
};

const okResponse = {
	path: "/home/coder/.coder/chats/chat-1/files/doc.txt",
	name: "doc.txt",
	size: 16,
	media_type: "text/plain",
};

describe("useWorkspaceFileUploads", () => {
	beforeEach(() => {
		uploadMock.mockReset();
	});

	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("transitions a file from uploading to uploaded", async () => {
		uploadMock.mockResolvedValueOnce(okResponse);
		const { result } = renderHook(() => useWorkspaceFileUploads("chat-1"));

		act(() => {
			result.current.handleAttach([makeFile()]);
		});

		const [attached] = result.current.files;
		expect(result.current.uploadStates.get(attached)?.status).toBe("uploading");

		await waitFor(() => {
			expect(result.current.uploadStates.get(attached)?.status).toBe(
				"uploaded",
			);
		});
		const state = result.current.uploadStates.get(attached);
		expect(state).toMatchObject({
			status: "uploaded",
			path: okResponse.path,
			name: okResponse.name,
			size: okResponse.size,
			mediaType: okResponse.media_type,
		});
	});

	it("records an error state when the API rejects", async () => {
		uploadMock.mockRejectedValueOnce(new Error("boom"));
		const { result } = renderHook(() => useWorkspaceFileUploads("chat-1"));

		act(() => {
			result.current.handleAttach([makeFile()]);
		});

		await waitFor(() => {
			const [attached] = result.current.files;
			expect(result.current.uploadStates.get(attached)?.status).toBe("error");
		});
	});

	it("rejects oversized files without calling the API", async () => {
		const { result } = renderHook(() => useWorkspaceFileUploads("chat-1"));
		const big = makeFile("big.bin", MaxWorkspaceFileSizeBytes + 1);

		act(() => {
			result.current.handleAttach([big]);
		});

		const [attached] = result.current.files;
		const state = result.current.uploadStates.get(attached);
		expect(state?.status).toBe("error");
		expect(uploadMock).not.toHaveBeenCalled();
	});

	it("records an error state when no chat id is supplied", async () => {
		const { result } = renderHook(() => useWorkspaceFileUploads(undefined));

		act(() => {
			result.current.handleAttach([makeFile()]);
		});

		const [attached] = result.current.files;
		const state = result.current.uploadStates.get(attached);
		expect(state?.status).toBe("error");
		expect(uploadMock).not.toHaveBeenCalled();
	});

	it("removes a file by reference and clears its state", async () => {
		uploadMock.mockResolvedValueOnce(okResponse);
		const { result } = renderHook(() => useWorkspaceFileUploads("chat-1"));

		act(() => {
			result.current.handleAttach([makeFile()]);
		});
		const [attached] = result.current.files;
		await waitFor(() => {
			expect(result.current.uploadStates.get(attached)?.status).toBe(
				"uploaded",
			);
		});

		act(() => {
			result.current.handleRemove(attached);
		});

		expect(result.current.files).toHaveLength(0);
		expect(result.current.uploadStates.get(attached)).toBeUndefined();
	});

	it("removes a file by index", async () => {
		uploadMock.mockResolvedValue(okResponse);
		const { result } = renderHook(() => useWorkspaceFileUploads("chat-1"));

		act(() => {
			result.current.handleAttach([makeFile("a.txt"), makeFile("b.txt")]);
		});
		await waitFor(() => {
			expect(result.current.files).toHaveLength(2);
		});

		act(() => {
			result.current.handleRemove(0);
		});

		expect(result.current.files).toHaveLength(1);
		expect(result.current.files[0].name).toBe("b.txt");
	});

	it("aborts pending requests on reset", async () => {
		let abortedDuringUpload = false;
		uploadMock.mockImplementation(
			(_chatId: string, _file: File, signal?: AbortSignal) =>
				new Promise((_resolve, reject) => {
					signal?.addEventListener("abort", () => {
						abortedDuringUpload = true;
						reject(new DOMException("aborted", "AbortError"));
					});
				}),
		);
		const { result } = renderHook(() => useWorkspaceFileUploads("chat-1"));

		act(() => {
			result.current.handleAttach([makeFile()]);
		});

		act(() => {
			result.current.reset();
		});

		await waitFor(() => {
			expect(abortedDuringUpload).toBe(true);
		});
		expect(result.current.files).toHaveLength(0);
		expect(result.current.uploadStates.size).toBe(0);
	});
});
