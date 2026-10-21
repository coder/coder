import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AxiosError, AxiosHeaders } from "axios";
import { useState } from "react";
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
import type { WorkspaceAgentRepoChanges } from "#/api/typesGenerated";
import { createDeferred } from "#/testHelpers/deferred";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import type FileEditor from "./FileEditor";
import { type FileDraft, FileTab } from "./FileTab";

vi.mock("./FileEditor", () => ({
	default: ({
		value,
		readOnly,
		onChange,
	}: React.ComponentProps<typeof FileEditor>) => (
		<textarea
			aria-label="File contents"
			value={value}
			readOnly={readOnly}
			onChange={(event) => onChange(event.target.value)}
		/>
	),
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

const repository: WorkspaceAgentRepoChanges = {
	repo_root: "/repo",
	branch: "main",
};

type HarnessProps = {
	repository?: WorkspaceAgentRepoChanges;
	readOnly?: boolean;
};

const Harness: React.FC<HarnessProps> = ({ repository, readOnly = false }) => {
	const [draft, setDraft] = useState<FileDraft>();
	return (
		<FileTab
			agentId={agentId}
			path={path}
			reveal={undefined}
			repository={repository}
			draft={draft}
			onDraftChange={(_, update) => setDraft(update)}
			readOnly={readOnly}
		/>
	);
};

const renderFileTab = (props: HarnessProps) => {
	const queryClient = createTestQueryClient();
	return render(<Harness {...props} />, {
		wrapper: ({ children }) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		),
	});
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
			);
		});
		await waitFor(() => {
			expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
		});
	});

	it("reloads from disk after a conflicting save", async () => {
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
			);
		});
		expect(screen.getByLabelText("File contents")).toHaveValue("hello world");
	});

	it("refetches when the repository changes and keeps local edits on request", async () => {
		const user = userEvent.setup();
		const { rerender } = renderFileTab({ repository });

		await editFile(user, " world");
		readFile.mockResolvedValue(fileContents("hello from disk", '"v2"'));
		rerender(<Harness repository={{ ...repository }} />);

		await user.click(await screen.findByRole("button", { name: "Keep mine" }));
		expect(screen.getByLabelText("File contents")).toHaveValue("hello world");

		await user.click(screen.getByRole("button", { name: "Save" }));
		await waitFor(() => {
			expect(writeFile).toHaveBeenCalledWith(
				agentId,
				path,
				"hello world",
				'"v2"',
			);
		});
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
			);
		});
	});

	it("keeps local edits when reloading fails", async () => {
		const user = userEvent.setup();
		writeFile.mockRejectedValueOnce(preconditionFailed());
		renderFileTab({});

		await editFile(user, " world");
		await user.click(screen.getByRole("button", { name: "Save" }));
		readFile.mockRejectedValueOnce(responseError(502, "Bad Gateway"));
		await user.click(await screen.findByRole("button", { name: "Reload" }));

		await waitFor(() => {
			expect(readFile).toHaveBeenCalledTimes(2);
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

	it("restarts a read in flight when the repository changes again", async () => {
		const { rerender } = renderFileTab({ repository });
		const editor = await screen.findByLabelText("File contents");

		const staleRead = createDeferred<WorkspaceAgentFileContents>();
		readFile.mockReturnValueOnce(staleRead.promise);
		rerender(<Harness repository={{ ...repository }} />);
		await waitFor(() => {
			expect(readFile).toHaveBeenCalledTimes(2);
		});

		readFile.mockResolvedValueOnce(fileContents("latest", '"v3"'));
		rerender(<Harness repository={{ ...repository }} />);
		await waitFor(() => {
			expect(editor).toHaveValue("latest");
		});

		await act(async () => {
			staleRead.resolve(fileContents("stale", '"v2"'));
			await staleRead.promise;
		});
		expect(readFile).toHaveBeenCalledTimes(3);
		expect(editor).toHaveValue("latest");
	});
});
