import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { focusManager, QueryClientProvider } from "react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
	API,
	type WorkspaceAgentFile,
	type WorkspaceAgentListDirectoryResponse,
} from "#/api/api";
import type { WorkspaceAgentRepoChanges } from "#/api/typesGenerated";
import { mockApiError } from "#/testHelpers/entities";
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
			{ ...entry("/home/coder/repo/debug.log", false), ignored: true },
		],
	},
	"/home/coder/repo/src": {
		absolute_path: ["home", "coder", "repo", "src"],
		absolute_path_string: "/home/coder/repo/src",
		contents: [
			entry("/home/coder/repo/src/main.go", false),
			entry("/home/coder/repo/src/lib", true),
		],
	},
	"/home/coder/repo/src/lib": {
		absolute_path: ["home", "coder", "repo", "src", "lib"],
		absolute_path_string: "/home/coder/repo/src/lib",
		contents: [entry("/home/coder/repo/src/lib/util.go", false)],
	},
	"~": {
		absolute_path: ["home", "coder"],
		absolute_path_string: "/home/coder",
		contents: [
			entry("/home/coder/.bashrc", false),
			entry("/home/coder/notes.md", false),
		],
	},
};

const apiError = (status: number) =>
	Object.assign(mockApiError({ message: "failed" }), { status });

const renderPanel = (value: WorkspaceFilesContextValue | undefined) => {
	const queryClient = createTestQueryClient();
	const panel = (panelValue: WorkspaceFilesContextValue | undefined) => (
		<QueryClientProvider client={queryClient}>
			<WorkspaceFilesContext value={panelValue}>
				<FilesPanel chatId="chat-1" />
			</WorkspaceFilesContext>
		</QueryClientProvider>
	);
	const result = render(panel(value));
	return {
		...result,
		rerenderPanel: (next: WorkspaceFilesContextValue) =>
			result.rerender(panel(next)),
	};
};

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
		expect(value.openFile).toHaveBeenCalledWith("/home/coder/repo/src/main.go", {
			focus: false,
		});
	});

	it("opens a gitignored file", async () => {
		const user = userEvent.setup();
		const value = contextValue();
		renderPanel(value);

		await user.click(
			await screen.findByRole("treeitem", { name: "debug.log" }),
		);

		expect(value.openFile).toHaveBeenCalledWith("/home/coder/repo/debug.log", {
			focus: false,
		});
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

		expect(value.openFile).toHaveBeenCalledWith("/home/coder/repo/src/main.go", {
			focus: false,
		});
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

	it("keeps listed entries when a refresh fails", async () => {
		const user = userEvent.setup();
		renderPanel(contextValue());

		await user.click(await screen.findByRole("treeitem", { name: "src" }));
		await screen.findByRole("treeitem", { name: "main.go" });
		vi.mocked(API.listWorkspaceAgentDirectory).mockRejectedValue(
			new Error("agent unreachable"),
		);
		await user.click(screen.getByRole("button", { name: "Refresh files" }));

		await screen.findByRole("alert");
		await screen.findByRole("treeitem", { name: /^Refresh failed/ });
		screen.getByRole("treeitem", { name: "README.md" });
		screen.getByRole("treeitem", { name: "main.go" });
	});

	it("stops refreshing directories under a collapsed directory", async () => {
		const user = userEvent.setup();
		renderPanel(contextValue());

		const src = await screen.findByRole("treeitem", { name: "src" });
		await user.click(src);
		await user.click(await screen.findByRole("treeitem", { name: "lib" }));
		await screen.findByRole("treeitem", { name: "util.go" });
		await user.click(src);
		vi.mocked(API.listWorkspaceAgentDirectory).mockClear();
		await user.click(screen.getByRole("button", { name: "Refresh files" }));

		await waitFor(() =>
			expect(API.listWorkspaceAgentDirectory).toHaveBeenCalledWith(
				"agent-1",
				"/home/coder/repo",
			),
		);
		expect(API.listWorkspaceAgentDirectory).toHaveBeenCalledTimes(1);

		await user.click(src);
		await screen.findByRole("treeitem", { name: "util.go" });
	});

	it("focuses the path input when Open path is chosen", async () => {
		const user = userEvent.setup();
		renderPanel(contextValue());

		await screen.findByRole("treeitem", { name: "src" });
		await user.click(screen.getByRole("combobox", { name: "Root directory" }));
		await user.click(screen.getByRole("option", { name: "Open path..." }));

		await waitFor(() =>
			expect(screen.getByRole("textbox", { name: "Path" })).toHaveFocus(),
		);
	});

	it.each([400, 500])(
		"opens a typed file path and roots the tree at its directory (%i)",
		async (status) => {
			const user = userEvent.setup();
			const value = contextValue();
			const listDirectory = vi.mocked(API.listWorkspaceAgentDirectory);
			const listing = listDirectory.getMockImplementation();
			listDirectory.mockImplementation(async (agentId, path) => {
				if (path === "~/notes.md") {
					throw apiError(status);
				}
				return listing ? listing(agentId, path) : Promise.reject();
			});
			renderPanel(value);

			await screen.findByRole("treeitem", { name: "src" });
			await user.click(
				screen.getByRole("combobox", { name: "Root directory" }),
			);
			await user.click(screen.getByRole("option", { name: "Open path..." }));
			await user.type(
				screen.getByRole("textbox", { name: "Path" }),
				"~/notes.md{Enter}",
			);

			await waitFor(() =>
				expect(value.openFile).toHaveBeenCalledWith("/home/coder/notes.md"),
			);
			expect(
				localStorage.getItem(`${filesPanelRootStorageKeyPrefix}chat-1`),
			).toBe("~");
		},
	);

	it("explains a denied path inline", async () => {
		const user = userEvent.setup();
		vi.mocked(API.listWorkspaceAgentDirectory).mockRejectedValue(
			apiError(403),
		);
		localStorage.setItem(`${filesPanelRootStorageKeyPrefix}chat-1`, "/root");
		renderPanel(contextValue());

		const alert = await screen.findByRole("alert");
		expect(alert).toHaveTextContent("Permission denied: /root");
		await user.click(screen.getByRole("combobox", { name: "Root directory" }));
		await user.click(screen.getByRole("option", { name: "Open path..." }));
		await user.type(
			screen.getByRole("textbox", { name: "Path" }),
			"/secret{Enter}",
		);
		await screen.findByText("Permission denied: /secret");
	});

	it("refetches expanded directories when their repository changes", async () => {
		const user = userEvent.setup();
		const repository: WorkspaceAgentRepoChanges = {
			repo_root: "/home/coder/repo",
			branch: "main",
		};
		const value = contextValue({
			repositories: new Map([[repository.repo_root, repository]]),
		});
		const { rerenderPanel } = renderPanel(value);

		await user.click(await screen.findByRole("treeitem", { name: "src" }));
		await screen.findByRole("treeitem", { name: "main.go" });
		vi.mocked(API.listWorkspaceAgentDirectory).mockClear();
		rerenderPanel({
			...value,
			repositories: new Map([
				[repository.repo_root, { ...repository, unified_diff: "diff" }],
			]),
		});

		await waitFor(() =>
			expect(API.listWorkspaceAgentDirectory).toHaveBeenCalledWith(
				"agent-1",
				"/home/coder/repo/src",
			),
		);
		expect(API.listWorkspaceAgentDirectory).toHaveBeenCalledWith(
			"agent-1",
			"/home/coder/repo",
		);
	});

	it("refetches the tree when the window regains focus", async () => {
		renderPanel(contextValue());
		await screen.findByRole("treeitem", { name: "src" });
		vi.mocked(API.listWorkspaceAgentDirectory).mockClear();

		act(() => {
			focusManager.setFocused(false);
			focusManager.setFocused(true);
		});

		await waitFor(() =>
			expect(API.listWorkspaceAgentDirectory).toHaveBeenCalledWith(
				"agent-1",
				"/home/coder/repo",
			),
		);
		focusManager.setFocused(undefined);
	});

	it("jumps by typed prefix and opens with Space", async () => {
		const user = userEvent.setup();
		const value = contextValue();
		renderPanel(value);

		const src = await screen.findByRole("treeitem", { name: "src" });
		act(() => src.focus());
		await user.keyboard("re");
		expect(screen.getByRole("treeitem", { name: "README.md" })).toHaveFocus();
		await user.keyboard(" ");

		expect(value.openFile).toHaveBeenCalledWith("/home/coder/repo/README.md", {
			focus: false,
		});
		await user.keyboard("{Escape}");
		expect(screen.getByRole("tree", { name: "Files" })).toHaveFocus();
	});

	it("does not list files without a workspace agent", () => {
		renderPanel(undefined);
		expect(API.listWorkspaceAgentDirectory).not.toHaveBeenCalled();
	});
});
