import {
	act,
	fireEvent,
	render,
	screen,
	waitFor,
} from "@testing-library/react";
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
import {
	WorkspaceFilesContext,
	type WorkspaceFilesContextValue,
} from "../../context/WorkspaceFilesContext";
import { createWatcherMessages } from "../../hooks/useGitWatcher";
import type FileEditor from "./FileEditor";
import { type FileDraft, FileTab } from "./FileTab";

// What the mock editor reports, as Monaco would once its chunk loads.
const mockEditor = vi.hoisted(() => ({ reportsStatus: true, language: "Go" }));

vi.mock("./FileEditor", () => ({
	default: function MockFileEditor({
		value,
		reveal,
		readOnly,
		onChange,
		onRevealed,
		onStatusChange,
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
		// Stands in for Monaco's cursor: the textarea's selection.
		const reportStatus = (textarea: HTMLTextAreaElement) => {
			if (!mockEditor.reportsStatus) {
				return;
			}
			const before = textarea.value.slice(0, textarea.selectionStart);
			onStatusChange?.({
				language: mockEditor.language,
				line: before.split("\n").length,
				column: before.length - before.lastIndexOf("\n"),
				selectedCharacters: textarea.selectionEnd - textarea.selectionStart,
			});
		};
		useEffect(() => {
			if (!mockEditor.reportsStatus) {
				return;
			}
			onStatusChange?.({
				language: mockEditor.language,
				line: 1,
				column: 1,
				selectedCharacters: 0,
			});
		}, [onStatusChange]);
		return (
			<textarea
				ref={ref}
				aria-label="File contents"
				value={value}
				readOnly={readOnly}
				onChange={(event) => onChange(event.target.value)}
				onSelect={(event) => reportStatus(event.currentTarget)}
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
	path?: string;
	readOnly?: boolean;
	onClose?: (path: string, hasUnsavedEdits: boolean) => void;
	workspaceFiles?: WorkspaceFilesContextValue;
};

const Harness: React.FC<HarnessProps> = ({
	path: filePath = path,
	readOnly = false,
	onClose = () => {},
	workspaceFiles,
}) => {
	const [draft, setDraft] = useState<FileDraft>();
	return (
		<WorkspaceFilesContext value={workspaceFiles}>
			<FileTab
				chatId="chat-1"
				agentId={agentId}
				path={filePath}
				reveal={undefined}
				isActive
				draft={draft}
				onDraftChange={(_, update) => setDraft(update)}
				onClose={onClose}
				readOnly={readOnly}
			/>
		</WorkspaceFilesContext>
	);
};

const renderFileTab = (props: HarnessProps) => {
	const queryClient = createTestQueryClient();
	const watcherMessages = createWatcherMessages();
	const withWatcherMessages = (
		workspaceFiles: WorkspaceFilesContextValue | undefined,
	) => workspaceFiles && { ...workspaceFiles, watcherMessages };
	const result = render(
		<Harness
			{...props}
			workspaceFiles={withWatcherMessages(props.workspaceFiles)}
		/>,
		{
			wrapper: ({ children }) => (
				<QueryClientProvider client={queryClient}>
					{children}
				</QueryClientProvider>
			),
		},
	);
	// What the main tabs do when the file may have changed.
	const refreshFile = () =>
		act(() =>
			queryClient.invalidateQueries({
				queryKey: workspaceAgentFileKey(agentId, path),
			}),
		);
	// What the git watcher reports after a scan that followed the tab's
	// last read or save of the file.
	const receiveWatcherMessage = (
		workspaceFiles: WorkspaceFilesContextValue,
	) => {
		const read = queryClient.getQueryState(
			workspaceAgentFileKey(agentId, props.path ?? path),
		);
		result.rerender(
			<Harness
				{...props}
				workspaceFiles={withWatcherMessages(workspaceFiles)}
			/>,
		);
		act(() => watcherMessages.receive((read?.dataUpdatedAt ?? 0) + 1));
	};
	return { ...result, queryClient, refreshFile, receiveWatcherMessage };
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
		mockEditor.reportsStatus = true;
		mockEditor.language = "Go";
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
		// The notice takes the place of the file info.
		expect(screen.queryByText("Go · 11 B · 1 line")).toBeNull();
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

	describe("file info", () => {
		it("describes the text in the editor and where its cursor is", async () => {
			const user = userEvent.setup();
			readFile.mockResolvedValue(fileContents("a\nb\n", '"v1"'));
			renderFileTab({});

			await screen.findByText("Go · 4 B · 2 lines · LF");
			await screen.findByText("Ln 1, Col 1");
			await editFile(user, "\nné");
			await screen.findByText("Go · 8 B · 4 lines · LF");

			const editor =
				screen.getByLabelText<HTMLTextAreaElement>("File contents");
			act(() => editor.setSelectionRange(2, 3));
			fireEvent.select(editor);
			await screen.findByText("Ln 2, Col 1 (1 selected)");
		});

		it("names the language from the file name before the editor loads", async () => {
			mockEditor.reportsStatus = false;
			readFile.mockResolvedValue(fileContents("a\nb\n", '"v1"'));
			renderFileTab({});

			await screen.findByText("Go · 4 B · 2 lines · LF");
			expect(screen.queryByText(/^Ln /)).toBeNull();
		});

		it("names the language from the editor for a file name it does not know", async () => {
			mockEditor.language = "Plain Text";
			readFile.mockResolvedValue(fileContents("module x\n", '"v1"'));
			renderFileTab({ path: "/repo/go.mod" });

			await screen.findByText("Plain Text · 9 B · 1 line · LF");
		});

		// The mock editor's textarea turns CRLF into LF on edit.
		it.each([
			["a\r\nb\r\n", "Go · 6 B · 2 lines · CRLF"],
			["a\r\nb\n", "Go · 5 B · 2 lines · Mixed"],
			["one line", "Go · 8 B · 1 line"],
			["", "Go · 0 B · 0 lines"],
		])("describes %j as %s", async (content, info) => {
			readFile.mockResolvedValue(fileContents(content, '"v1"'));
			renderFileTab({});

			await screen.findByText(info);
		});

		it("describes an image by its type, size and dimensions", async () => {
			vi.spyOn(URL, "createObjectURL").mockReturnValue("blob:image");
			vi.spyOn(URL, "revokeObjectURL").mockReturnValue(undefined);
			readFile.mockResolvedValue({
				data: new Uint8Array(2_900).buffer,
				contentType: "image/svg+xml",
				etag: '"v1"',
			});
			renderFileTab({});

			const image = await screen.findByRole("img", { name: "main.go" });
			Object.defineProperties(image, {
				naturalWidth: { value: 640 },
				naturalHeight: { value: 480 },
			});
			fireEvent.load(image);
			await screen.findByText("SVG · 2.9 kB · 640 × 480");
		});

		it("gives the size of a file it does not show", async () => {
			// The agent's ETag ends in the file's size in hex.
			readFile.mockResolvedValue({
				data: new Uint8Array(8 << 10).buffer,
				contentType: "application/octet-stream",
				etag: '"18e0e55e9817afe6-325aa0"',
			});
			renderFileTab({});

			await screen.findByText("Binary file");
			await screen.findByText("3.3 MB");
		});
	});

	describe("file actions", () => {
		const headCommit = "0123456789abcdef0123456789abcdef01234567";
		const workspaceFiles = (
			diffStatuses: WorkspaceFilesContextValue["diffStatuses"] = [],
			remoteOrigin = "git@github.com:coder/coder.git",
			unifiedDiff = "",
		): WorkspaceFilesContextValue => ({
			agentId,
			defaultDirectory: "/home/coder",
			repoRoots: ["/repo"],
			repoRootsSettled: true,
			repositories: new Map([
				[
					"/repo",
					{
						repo_root: "/repo",
						branch: "feat",
						remote_origin: remoteOrigin,
						head_commit: headCommit,
						unified_diff: unifiedDiff,
					},
				],
			]),
			openFile: vi.fn(),
			revealInFiles: vi.fn(),
			diffStatuses,
		});
		const pullRequestStatus = {
			chat_id: "chat-1",
			remote_origin: "https://github.com/coder/coder.git",
			git_branch: "feat",
			url: "https://github.com/coder/coder/pull/12",
			pr_number: 12,
			pull_request_title: "Add a feature",
			pull_request_draft: false,
			changes_requested: false,
			additions: 1,
			deletions: 0,
			changed_files: 1,
		};
		const openMenu = async (user: ReturnType<typeof userEvent.setup>) =>
			user.click(await screen.findByRole("button", { name: "File actions" }));
		const menuItemNames = () =>
			screen.getAllByRole("menuitem").map((item) => item.textContent);

		it("copies the file's paths and reveals it in the Files panel", async () => {
			const user = userEvent.setup();
			const writeText = vi
				.spyOn(navigator.clipboard, "writeText")
				.mockResolvedValue(undefined);
			const files = workspaceFiles();
			renderFileTab({ workspaceFiles: files });

			await openMenu(user);
			await user.click(screen.getByRole("menuitem", { name: "Copy path" }));
			expect(writeText).toHaveBeenLastCalledWith(path);
			await openMenu(user);
			await user.click(
				screen.getByRole("menuitem", { name: "Copy relative path" }),
			);
			expect(writeText).toHaveBeenLastCalledWith("main.go");
			await openMenu(user);
			await user.click(
				screen.getByRole("menuitem", { name: "Reveal in Files" }),
			);
			expect(files.revealInFiles).toHaveBeenLastCalledWith(path);
		});

		it("links the pushed branch, without the cursor's line, and its pull request", async () => {
			const user = userEvent.setup();
			readFile.mockResolvedValue(fileContents("a\nb\n", '"v1"'));
			renderFileTab({
				workspaceFiles: workspaceFiles([pullRequestStatus]),
			});
			const editor =
				await screen.findByLabelText<HTMLTextAreaElement>("File contents");
			act(() => editor.setSelectionRange(2, 2));
			fireEvent.select(editor);

			await openMenu(user);
			expect(
				screen.getByRole("menuitem", { name: "Open on GitHub" }),
			).toHaveAttribute(
				"href",
				"https://github.com/coder/coder/blob/feat/main.go",
			);
			const pullRequest = screen.getByRole("menuitem", {
				name: "Open in PR #12",
			});
			await waitFor(() => {
				expect(pullRequest).toHaveAttribute(
					"href",
					"https://github.com/coder/coder/pull/12/files#diff-2873f79a86c0d8b3335cd7731b0ecf7dd4301eb19a82ef7a1cba7589b5252261",
				);
			});
		});

		const insertDiff =
			"diff --git a/main.go b/main.go\nindex 1..2 100644\n--- a/main.go\n+++ b/main.go\n@@ -1,3 +1,4 @@\n a\n+new\n b\n c\n";
		const blob = `https://github.com/coder/coder/blob/${headCommit}/main.go`;
		/** The header menu's link at HEAD, with the cursor at `offset`. */
		const headLink = async (
			user: ReturnType<typeof userEvent.setup>,
			offset: number,
		) => {
			const editor =
				await screen.findByLabelText<HTMLTextAreaElement>("File contents");
			act(() => editor.setSelectionRange(offset, offset));
			fireEvent.select(editor);
			await openMenu(user);
			const href = screen
				.getByRole("menuitem", { name: "Open on GitHub" })
				.getAttribute("href");
			await user.keyboard("{Escape}");
			return href;
		};

		it("links the checked-out commit at the cursor's line in it", async () => {
			const user = userEvent.setup();
			readFile.mockResolvedValue(fileContents("a\nnew\nb\nc\n", '"v1"'));
			const files = workspaceFiles([], undefined, insertDiff);
			const { receiveWatcherMessage } = renderFileTab({
				workspaceFiles: files,
			});
			const editor =
				await screen.findByLabelText<HTMLTextAreaElement>("File contents");
			receiveWatcherMessage(files);

			// Line 3 on disk, "b", is line 2 at HEAD.
			expect(await headLink(user, 6)).toBe(`${blob}#L2`);
			// HEAD lacks line 2, "new".
			expect(await headLink(user, 2)).toBe(blob);
			// Unsaved edits move the editor's lines away from the disk's: line 4
			// in the editor, "b", is line 3 on disk.
			await user.type(editor, "x{Enter}", {
				initialSelectionStart: 0,
				initialSelectionEnd: 0,
			});
			expect(await headLink(user, 8)).toBe(blob);
		});

		it("links a line only once a scan follows the read or save", async () => {
			const user = userEvent.setup();
			readFile.mockResolvedValue(fileContents("a\nb\nc\n", '"v1"'));
			const files = workspaceFiles();
			const { receiveWatcherMessage } = renderFileTab({
				workspaceFiles: files,
			});
			const editor =
				await screen.findByLabelText<HTMLTextAreaElement>("File contents");

			expect(await headLink(user, 2)).toBe(blob);
			receiveWatcherMessage(files);
			expect(await headLink(user, 2)).toBe(`${blob}#L2`);

			await user.type(editor, "new{Enter}", {
				initialSelectionStart: 2,
				initialSelectionEnd: 2,
			});
			await user.click(screen.getByRole("button", { name: "Save" }));
			await waitFor(() => {
				expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
			});
			// The clean diff from before the save would put "b", now line 3 on
			// disk, on line 3 at HEAD.
			expect(await headLink(user, 6)).toBe(blob);
			receiveWatcherMessage(workspaceFiles([], undefined, insertDiff));
			expect(await headLink(user, 6)).toBe(`${blob}#L2`);
		});

		it("links no line while a reread of the file is pending", async () => {
			const user = userEvent.setup();
			readFile.mockResolvedValue(fileContents("a\nb\nc\n", '"v1"'));
			const files = workspaceFiles();
			const { queryClient, receiveWatcherMessage } = renderFileTab({
				workspaceFiles: files,
			});
			await screen.findByLabelText("File contents");
			receiveWatcherMessage(files);
			expect(await headLink(user, 2)).toBe(`${blob}#L2`);

			// A scan finds a line inserted at the top, and the main tabs reread
			// the file, which still shows its old content meanwhile: "b" is
			// still line 2 in the editor, not the diff's line 3.
			readFile.mockReturnValue(new Promise(() => {}));
			act(() => {
				void queryClient.invalidateQueries({
					queryKey: workspaceAgentFileKey(agentId, path),
				});
			});
			receiveWatcherMessage(
				workspaceFiles(
					[],
					undefined,
					"diff --git a/main.go b/main.go\nindex 1..2 100644\n--- a/main.go\n+++ b/main.go\n@@ -1,3 +1,4 @@\n+top\n a\n b\n c\n",
				),
			);
			expect(await headLink(user, 2)).toBe(blob);
		});

		it("links the checked-out commit of a branch not known to be pushed", async () => {
			const user = userEvent.setup();
			renderFileTab({ workspaceFiles: workspaceFiles() });

			await openMenu(user);
			const link = screen.getByRole("menuitem", { name: "Open on GitHub" });
			expect(link).toHaveAttribute(
				"href",
				`https://github.com/coder/coder/blob/${headCommit}/main.go`,
			);
			expect(link).toHaveAttribute(
				"title",
				"Opens commit 0123456, the checked-out HEAD. GitHub shows a 404 if the commit is not pushed.",
			);
			expect(screen.queryByRole("menuitem", { name: /^Open in/ })).toBeNull();
		});

		it("offers no link for a host it does not know", async () => {
			const user = userEvent.setup();
			renderFileTab({
				workspaceFiles: workspaceFiles([], "https://bitbucket.org/coder/coder"),
			});

			await openMenu(user);
			expect(menuItemNames()).toEqual([
				"Copy path",
				"Copy relative path",
				"Reveal in Files",
			]);
		});

		const otherFileDiff =
			"diff --git a/other.go b/other.go\nnew file mode 100644\nindex 0000000..e69de29\n";

		it.each([
			[
				"untracked or staged",
				"diff --git a/main.go b/main.go\nnew file mode 100644\nindex 0000000..587be6b\n--- /dev/null\n+++ b/main.go\n@@ -0,0 +1 @@\n+x\n",
			],
			[
				"empty and untracked",
				"diff --git a/main.go b/main.go\nnew file mode 100644\nindex 0000000..e69de29\n",
			],
			[
				"renamed",
				"diff --git a/old.go b/main.go\nsimilarity index 100%\nrename from old.go\nrename to main.go\n",
			],
		])("offers no host links for a file HEAD lacks: %s", async (_, diff) => {
			const user = userEvent.setup();
			renderFileTab({
				workspaceFiles: workspaceFiles(
					[pullRequestStatus],
					undefined,
					otherFileDiff + diff,
				),
			});

			await openMenu(user);
			expect(menuItemNames()).toEqual([
				"Copy path",
				"Copy relative path",
				"Reveal in Files",
			]);
		});

		it("links a modified file", async () => {
			const user = userEvent.setup();
			renderFileTab({
				workspaceFiles: workspaceFiles(
					[pullRequestStatus],
					undefined,
					`${otherFileDiff}diff --git a/main.go b/main.go\nindex 7898192..6178079 100644\n--- a/main.go\n+++ b/main.go\n@@ -1 +1 @@\n-new file mode 100644\n+b\n`,
				),
			});

			await openMenu(user);
			expect(menuItemNames()).toEqual([
				"Copy path",
				"Copy relative path",
				"Reveal in Files",
				"Open on GitHub",
				"Open in PR #12",
			]);
		});
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
