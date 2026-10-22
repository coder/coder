import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AxiosError, AxiosHeaders } from "axios";
import { useEffect, useRef, useState } from "react";
import { QueryClientProvider } from "react-query";
import {
	afterEach,
	beforeEach,
	describe,
	expect,
	it,
	type MockInstance,
	vi,
} from "vitest";
import { API, type WorkspaceAgentFileContents } from "#/api/api";
import { workspaceAgentFileKey } from "#/api/queries/workspaceFiles";
import { createDeferred } from "#/testHelpers/deferred";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import type FileEditor from "./FileEditor";
import { type FileDraft, FileTab } from "./FileTab";

vi.mock("./FileEditor", () => ({
	default: function MockFileEditor({
		value,
		reveal,
		readOnly,
		onChange,
		onRevealed,
	}: React.ComponentProps<typeof FileEditor>) {
		const ref = useRef<HTMLTextAreaElement>(null);
		useEffect(() => {
			if (reveal) {
				if (reveal.focus) {
					ref.current?.focus();
				}
				onRevealed();
			}
		}, [reveal, onRevealed]);
		return (
			<textarea
				ref={ref}
				aria-label="File contents"
				value={value}
				readOnly={readOnly}
				onChange={(event) => onChange(event.target.value)}
			/>
		);
	},
}));

const agentId = "agent-1";
const path = "/repo/main.go";

const fileContents = (
	content: string,
	etag: string,
): WorkspaceAgentFileContents => ({
	data: new TextEncoder().encode(content).buffer,
	contentType: "text/plain; charset=utf-8",
	etag,
});

const responseError = (status: number, statusText: string) =>
	new AxiosError(
		`Request failed with status code ${status}`,
		AxiosError.ERR_BAD_REQUEST,
		undefined,
		undefined,
		{
			data: {},
			status,
			statusText,
			headers: { etag: '"v2"' },
			config: { headers: new AxiosHeaders() },
		},
	);

const preconditionFailed = () => responseError(412, "Precondition Failed");

type HarnessProps = {
	readOnly?: boolean;
	onClose?: (path: string, hasUnsavedEdits: boolean) => void;
};

const Harness: React.FC<HarnessProps> = ({
	readOnly = false,
	onClose = () => {},
}) => {
	const [draft, setDraft] = useState<FileDraft>();
	return (
		<FileTab
			chatId="chat-1"
			agentId={agentId}
			path={path}
			reveal={undefined}
			isActive
			draft={draft}
			onDraftChange={(_, update) => setDraft(update)}
			onClose={onClose}
			readOnly={readOnly}
		/>
	);
};

const renderFileTab = (props: HarnessProps) => {
	const queryClient = createTestQueryClient();
	const result = render(<Harness {...props} />, {
		wrapper: ({ children }) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		),
	});
	// What the main tabs do when the file may have changed.
	const refreshFile = () =>
		act(() =>
			queryClient.invalidateQueries({
				queryKey: workspaceAgentFileKey(agentId, path),
			}),
		);
	return { ...result, refreshFile };
};

const editFile = async (
	user: ReturnType<typeof userEvent.setup>,
	text: string,
) => {
	await user.type(await screen.findByLabelText("File contents"), text);
};

describe("FileTab", () => {
	let readFile: MockInstance<typeof API.readWorkspaceAgentFile>;
	let writeFile: MockInstance<typeof API.writeWorkspaceAgentFile>;

	beforeEach(() => {
		readFile = vi
			.spyOn(API, "readWorkspaceAgentFile")
			.mockResolvedValue(fileContents("hello", '"v1"'));
		writeFile = vi
			.spyOn(API, "writeWorkspaceAgentFile")
			.mockResolvedValue({ etag: '"v2"' });
	});

	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("saves with the etag the edits are based on", async () => {
		const user = userEvent.setup();
		renderFileTab({});

		await editFile(user, " world");
		await user.click(screen.getByRole("button", { name: "Save" }));

		await waitFor(() => {
			expect(writeFile).toHaveBeenCalledWith(
				agentId,
				path,
				"hello world",
				'"v1"',
				{ createOnly: false },
			);
		});
		await waitFor(() => {
			expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
		});
	});

	it("reloads from disk after a conflicting save and focuses the editor", async () => {
		const user = userEvent.setup();
		writeFile.mockRejectedValueOnce(preconditionFailed());
		renderFileTab({});

		await editFile(user, " world");
		await user.click(screen.getByRole("button", { name: "Save" }));
		readFile.mockResolvedValue(fileContents("changed on disk", '"v2"'));
		await user.click(await screen.findByRole("button", { name: "Reload" }));

		await waitFor(() => {
			expect(screen.getByLabelText("File contents")).toHaveValue(
				"changed on disk",
			);
		});
		expect(writeFile).toHaveBeenCalledTimes(1);
		await waitFor(() => {
			expect(screen.getByLabelText("File contents")).toHaveFocus();
		});
	});

	it("overwrites the file without a precondition after a conflict", async () => {
		const user = userEvent.setup();
		writeFile.mockRejectedValueOnce(preconditionFailed());
		renderFileTab({});

		await editFile(user, " world");
		await user.click(screen.getByRole("button", { name: "Save" }));
		await user.click(await screen.findByRole("button", { name: "Overwrite" }));

		await waitFor(() => {
			expect(writeFile).toHaveBeenLastCalledWith(
				agentId,
				path,
				"hello world",
				undefined,
				{ createOnly: false },
			);
		});
		expect(screen.getByLabelText("File contents")).toHaveValue("hello world");
	});

	it("keeps local edits over a change on disk on request", async () => {
		const user = userEvent.setup();
		const { refreshFile } = renderFileTab({});

		await editFile(user, " world");
		readFile.mockResolvedValue(fileContents("hello from disk", '"v2"'));
		await refreshFile();

		await user.click(await screen.findByRole("button", { name: "Keep mine" }));
		expect(screen.getByLabelText("File contents")).toHaveValue("hello world");

		await user.click(screen.getByRole("button", { name: "Save" }));
		await waitFor(() => {
			expect(writeFile).toHaveBeenCalledWith(
				agentId,
				path,
				"hello world",
				'"v2"',
				{ createOnly: false },
			);
		});
	});

	it("keeps the buffer of a file deleted on disk and recreates it", async () => {
		const user = userEvent.setup();
		// The agent rejects a conditional write to a missing file.
		writeFile.mockRejectedValueOnce(preconditionFailed());
		renderFileTab({});

		await editFile(user, " world");
		readFile.mockRejectedValue(responseError(404, "Not Found"));
		await user.click(screen.getByRole("button", { name: "Save" }));

		expect(await screen.findByRole("alert")).toHaveTextContent(
			/Deleted on disk/,
		);
		expect(screen.getByLabelText("File contents")).toHaveValue("hello world");
		await user.click(screen.getByRole("button", { name: "Recreate" }));
		await waitFor(() => {
			expect(writeFile).toHaveBeenLastCalledWith(
				agentId,
				path,
				"hello world",
				undefined,
				{ createOnly: true },
			);
		});
		await waitFor(() => {
			expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
		});
	});

	it.each(["Recreate", "Save"])(
		"shows a conflict when %s finds the deleted file created again",
		async (button) => {
			const user = userEvent.setup();
			writeFile.mockRejectedValueOnce(preconditionFailed());
			renderFileTab({});

			await editFile(user, " world");
			readFile.mockRejectedValue(responseError(404, "Not Found"));
			await user.click(screen.getByRole("button", { name: "Save" }));
			await screen.findByText("Deleted on disk");
			writeFile.mockRejectedValueOnce(preconditionFailed());
			readFile.mockResolvedValue(fileContents("created elsewhere", '"v3"'));
			await user.click(screen.getByRole("button", { name: button }));

			expect(await screen.findByRole("alert")).toHaveTextContent(
				/Changed on disk/,
			);
			expect(writeFile).toHaveBeenLastCalledWith(
				agentId,
				path,
				"hello world",
				undefined,
				{ createOnly: true },
			);
			expect(screen.getByLabelText("File contents")).toHaveValue("hello world");
		},
	);

	it("offers no Recreate for a deleted file without complete text", async () => {
		readFile.mockResolvedValue({
			data: new Uint8Array([0, 1, 2]).buffer,
			contentType: "application/octet-stream",
			etag: '"v1"',
		});
		const { refreshFile } = renderFileTab({});
		await screen.findByText("Binary file");

		readFile.mockRejectedValue(responseError(404, "Not Found"));
		await refreshFile();

		expect(await screen.findByRole("alert")).toHaveTextContent(
			/Deleted on disk/,
		);
		expect(screen.queryByRole("button", { name: "Recreate" })).toBeNull();
		expect(screen.getByRole("button", { name: "Close" })).toBeEnabled();
	});

	it("closes the tab of a file deleted on disk from its notice", async () => {
		const user = userEvent.setup();
		const onClose = vi.fn();
		writeFile.mockRejectedValueOnce(preconditionFailed());
		renderFileTab({ onClose });

		await editFile(user, " world");
		readFile.mockRejectedValue(responseError(404, "Not Found"));
		await user.click(screen.getByRole("button", { name: "Save" }));
		await user.click(await screen.findByRole("button", { name: "Close" }));

		expect(onClose).toHaveBeenCalledWith(path, true);
	});

	it("keeps edits made during a save dirty against the saved content", async () => {
		const user = userEvent.setup();
		const write = createDeferred<{ etag: string }>();
		writeFile.mockReturnValueOnce(write.promise);
		renderFileTab({});

		await editFile(user, " world");
		await user.click(screen.getByRole("button", { name: "Save" }));
		const editor = screen.getByLabelText("File contents");
		await user.clear(editor);
		await user.type(editor, "hello");
		await act(async () => {
			write.resolve({ etag: '"v2"' });
			await write.promise;
		});

		expect(editor).toHaveValue("hello");
		await user.click(screen.getByRole("button", { name: "Save" }));
		await waitFor(() => {
			expect(writeFile).toHaveBeenLastCalledWith(
				agentId,
				path,
				"hello",
				'"v2"',
				{ createOnly: false },
			);
		});
	});

	it("keeps local edits when reloading fails", async () => {
		const user = userEvent.setup();
		writeFile.mockRejectedValueOnce(preconditionFailed());
		renderFileTab({});

		await editFile(user, " world");
		await user.click(screen.getByRole("button", { name: "Save" }));
		const reload = await screen.findByRole("button", { name: "Reload" });
		// The rejected save rereads the file to tell a change from a deletion.
		await waitFor(() => {
			expect(reload).toBeEnabled();
		});
		readFile.mockRejectedValueOnce(responseError(502, "Bad Gateway"));
		await user.click(reload);

		await waitFor(() => {
			expect(readFile).toHaveBeenCalledTimes(3);
		});
		await waitFor(() => {
			expect(screen.getByRole("button", { name: "Reload" })).toBeEnabled();
		});
		expect(screen.getByLabelText("File contents")).toHaveValue("hello world");
		expect(screen.getByRole("button", { name: "Save" })).toBeEnabled();
	});

	it("opens files read-only without a way to save", async () => {
		const user = userEvent.setup();
		renderFileTab({ readOnly: true });

		await editFile(user, " world");

		expect(screen.getByLabelText("File contents")).toHaveValue("hello");
		expect(screen.queryByRole("button", { name: "Save" })).toBeNull();
	});

	it("treats invalid UTF-8 as binary instead of editable text", async () => {
		readFile.mockResolvedValue({
			data: new Uint8Array([0x68, 0x69, 0xff, 0x0a]).buffer,
			contentType: "text/plain",
			etag: '"v1"',
		});
		renderFileTab({});

		await screen.findByText("Binary file");
		expect(screen.queryByLabelText("File contents")).toBeNull();
		expect(screen.queryByRole("button", { name: "Save" })).toBeNull();
	});

	describe("reading large files", () => {
		const sniffBytes = 8 << 10;
		const bytes = (length: number, fill: number) => ({
			data: new Uint8Array(length).fill(fill).buffer,
			contentType: "application/octet-stream",
			etag: '"v1"',
		});
		const serveFile = (size: number | (() => number), fill: number) =>
			readFile.mockImplementation(async (_agent, _path, range) => {
				const offset = range?.offset ?? 0;
				const currentSize = typeof size === "function" ? size() : size;
				const available = Math.max(currentSize - offset, 0);
				return bytes(Math.min(available, range?.limit ?? available), fill);
			});

		it("reads only the first 8 KiB of a binary file", async () => {
			serveFile(3_300_000, 0);
			renderFileTab({});

			await screen.findByText("Binary file");
			expect(readFile.mock.calls).toEqual([
				[agentId, path, { limit: sniffBytes }],
			]);
		});

		it("refuses text over 10 MiB without downloading it", async () => {
			serveFile((10 << 20) + 1, 0x61);
			renderFileTab({});

			await screen.findByText("File too large");
			expect(readFile.mock.calls).toEqual([
				[agentId, path, { limit: sniffBytes }],
				[agentId, path, { offset: 10 << 20, limit: 1 }],
			]);
		});

		it("opens text over 8 KiB after checking its size", async () => {
			serveFile(20_000, 0x61);
			renderFileTab({});

			const editor = await screen.findByLabelText("File contents");
			expect(editor).toHaveValue("a".repeat(20_000));
			expect(readFile).toHaveBeenLastCalledWith(agentId, path, {
				limit: (10 << 20) + 1,
			});
		});

		it("refuses text that grows past 10 MiB after its size check", async () => {
			// The third read, after the size check, sees the grown file.
			serveFile(
				() => (readFile.mock.calls.length < 3 ? 20_000 : 20 << 20),
				0x61,
			);
			renderFileTab({});

			await screen.findByText("File too large");
			expect(readFile).toHaveBeenCalledTimes(3);
		});
	});
});
