import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AxiosError, AxiosHeaders } from "axios";
import { type FC, useState } from "react";
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
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import type FileEditor from "./FileEditor";
import { type FileDraft, FileTab } from "./FileTab";

vi.mock("./FileEditor", () => ({
	default: ({ value, onChange }: React.ComponentProps<typeof FileEditor>) => (
		<textarea
			aria-label="File contents"
			value={value}
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

const preconditionFailed = () =>
	new AxiosError(
		"Request failed with status code 412",
		AxiosError.ERR_BAD_REQUEST,
		undefined,
		undefined,
		{
			data: { message: "File changed." },
			status: 412,
			statusText: "Precondition Failed",
			headers: { etag: '"v2"' },
			config: { headers: new AxiosHeaders() },
		},
	);

const repository: WorkspaceAgentRepoChanges = {
	repo_root: "/repo",
	branch: "main",
};

const Harness: FC<{ repository?: WorkspaceAgentRepoChanges }> = ({
	repository,
}) => {
	const [draft, setDraft] = useState<FileDraft>();
	return (
		<FileTab
			agentId={agentId}
			path={path}
			reveal={undefined}
			repository={repository}
			draft={draft}
			onDraftChange={(_, update) => setDraft(update)}
		/>
	);
};

const renderFileTab = (props: { repository?: WorkspaceAgentRepoChanges }) => {
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
});
