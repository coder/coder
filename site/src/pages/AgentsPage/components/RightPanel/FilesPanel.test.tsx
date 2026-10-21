import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
	API,
	type WorkspaceAgentFile,
	type WorkspaceAgentListDirectoryResponse,
} from "#/api/api";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import {
	WorkspaceFilesContext,
	type WorkspaceFilesContextValue,
} from "../../context/WorkspaceFilesContext";
import { FilesPanel, filesPanelRootStorageKeyPrefix } from "./FilesPanel";

const entry = (absolutePath: string, isDir: boolean): WorkspaceAgentFile => ({
	name: absolutePath.slice(absolutePath.lastIndexOf("/") + 1),
	absolute_path_string: absolutePath,
	is_dir: isDir,
	is_symlink: false,
	size: 0,
	mod_time: "2026-01-01T00:00:00Z",
});

const listings: Record<string, WorkspaceAgentListDirectoryResponse> = {
	"/home/coder/repo": {
		absolute_path: ["home", "coder", "repo"],
		absolute_path_string: "/home/coder/repo",
		contents: [
			entry("/home/coder/repo/src", true),
			entry("/home/coder/repo/README.md", false),
		],
	},
	"/home/coder/repo/src": {
		absolute_path: ["home", "coder", "repo", "src"],
		absolute_path_string: "/home/coder/repo/src",
		contents: [entry("/home/coder/repo/src/main.go", false)],
	},
	"~": {
		absolute_path: ["home", "coder"],
		absolute_path_string: "/home/coder",
		contents: [entry("/home/coder/.bashrc", false)],
	},
};

const renderPanel = (value: WorkspaceFilesContextValue | undefined) =>
	render(
		<QueryClientProvider client={createTestQueryClient()}>
			<WorkspaceFilesContext value={value}>
				<FilesPanel chatId="chat-1" />
			</WorkspaceFilesContext>
		</QueryClientProvider>,
	);

const contextValue = (
	overrides: Partial<WorkspaceFilesContextValue> = {},
): WorkspaceFilesContextValue => ({
	agentId: "agent-1",
	defaultDirectory: "/home/coder",
	repoRoots: ["/home/coder/repo"],
	repositories: new Map(),
	openFile: vi.fn(),
	...overrides,
});

describe("FilesPanel", () => {
	beforeEach(() => {
		localStorage.clear();
		vi.spyOn(API, "listWorkspaceAgentDirectory").mockImplementation(
			async (_agentId, path) => {
				const listing = listings[path];
				if (!listing) {
					throw new Error(`unexpected path ${path}`);
				}
				return listing;
			},
		);
	});

	it("opens a file from an expanded directory by its absolute path", async () => {
		const user = userEvent.setup();
		const value = contextValue();
		renderPanel(value);

		await user.click(await screen.findByRole("treeitem", { name: "src" }));
		await user.click(await screen.findByRole("treeitem", { name: "main.go" }));

		expect(API.listWorkspaceAgentDirectory).toHaveBeenCalledWith(
			"agent-1",
			"/home/coder/repo/src",
		);
		expect(value.openFile).toHaveBeenCalledWith("/home/coder/repo/src/main.go");
	});

	it("expands and opens with the keyboard", async () => {
		const user = userEvent.setup();
		const value = contextValue();
		renderPanel(value);

		await screen.findByRole("treeitem", { name: "src" });
		await user.tab();
		await user.tab();
		await user.tab();
		await user.keyboard("{ArrowRight}");
		await screen.findByRole("treeitem", { name: "main.go" });
		await user.keyboard("{ArrowRight}{Enter}");

		expect(value.openFile).toHaveBeenCalledWith("/home/coder/repo/src/main.go");
	});

	it("switches the root and remembers it for the chat", async () => {
		const user = userEvent.setup();
		const { unmount } = renderPanel(contextValue());

		await screen.findByRole("treeitem", { name: "README.md" });
		await user.click(screen.getByRole("combobox", { name: "Root directory" }));
		await user.click(screen.getByRole("option", { name: "Home" }));
		await screen.findByRole("treeitem", { name: ".bashrc" });

		expect(API.listWorkspaceAgentDirectory).toHaveBeenLastCalledWith(
			"agent-1",
			"~",
		);
		expect(
			localStorage.getItem(`${filesPanelRootStorageKeyPrefix}chat-1`),
		).toBe("~");

		unmount();
		vi.mocked(API.listWorkspaceAgentDirectory).mockClear();
		renderPanel(contextValue());
		await screen.findByRole("treeitem", { name: ".bashrc" });
		expect(API.listWorkspaceAgentDirectory).toHaveBeenCalledWith(
			"agent-1",
			"~",
		);
		expect(API.listWorkspaceAgentDirectory).not.toHaveBeenCalledWith(
			"agent-1",
			"/home/coder/repo",
		);
	});

	it("opens a typed path as the root", async () => {
		const user = userEvent.setup();
		renderPanel(contextValue({ repoRoots: [] }));

		await waitFor(() =>
			expect(API.listWorkspaceAgentDirectory).toHaveBeenCalledWith(
				"agent-1",
				"",
			),
		);
		await user.click(screen.getByRole("combobox", { name: "Root directory" }));
		await user.click(screen.getByRole("option", { name: "Open path..." }));
		await user.type(screen.getByRole("textbox", { name: "Path" }), "~{Enter}");

		await screen.findByRole("treeitem", { name: ".bashrc" });
		expect(
			localStorage.getItem(`${filesPanelRootStorageKeyPrefix}chat-1`),
		).toBe("~");
	});

	it("does not list files without a workspace agent", () => {
		renderPanel(undefined);
		expect(API.listWorkspaceAgentDirectory).not.toHaveBeenCalled();
	});
});
