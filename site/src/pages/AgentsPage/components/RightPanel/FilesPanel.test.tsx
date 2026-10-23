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
import {
	FilesPanel,
	filesPanelRootStorageKeyPrefix,
	type RevealRequest,
} from "./FilesPanel";

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

listings["/home/coder"] = listings["~"];

const renderRevealed = (
	request: RevealRequest | undefined,
	value: WorkspaceFilesContextValue = contextValue(),
) => {
	const queryClient = createTestQueryClient();
	const onRevealHandled = vi.fn();
	const panel = (
		revealRequest: RevealRequest | undefined,
		panelValue: WorkspaceFilesContextValue,
	) => (
		<QueryClientProvider client={queryClient}>
			<WorkspaceFilesContext value={panelValue}>
				<FilesPanel
					chatId="chat-1"
					revealRequest={revealRequest}
					onRevealHandled={onRevealHandled}
				/>
			</WorkspaceFilesContext>
		</QueryClientProvider>
	);
	const result = render(panel(request, value));
	return {
		onRevealHandled,
		// Passing the same request again only updates the context value.
		reveal: (next: RevealRequest, nextValue = value) =>
			result.rerender(panel(next, nextValue)),
	};
};

const apiError = (status: number) =>
	Object.assign(mockApiError({ message: "failed" }), { status });

const listedPaths = () =>
	vi.mocked(API.listWorkspaceAgentDirectory).mock.calls.map(([, path]) => path);

// Holds list-directory calls for one path until the test releases them.
const holdListings = (heldPath: string) => {
	const listDirectory = vi.mocked(API.listWorkspaceAgentDirectory);
	const fallback = listDirectory.getMockImplementation();
	const release: (() => void)[] = [];
	listDirectory.mockImplementation((agentId, path, signal) => {
		if (path !== heldPath || !fallback) {
			return fallback ? fallback(agentId, path, signal) : Promise.reject();
		}
		return new Promise((resolve) => {
			release.push(() => resolve(fallback(agentId, path, signal)));
		});
	});
	return () => {
		for (const resolve of release.splice(0)) {
			resolve();
		}
	};
};

// Never answers the first list-directory call for one path, recording the
// signal of every call for it.
const stallFirstListing = (stalledPath: string) => {
	const listDirectory = vi.mocked(API.listWorkspaceAgentDirectory);
	const fallback = listDirectory.getMockImplementation();
	const signals: (AbortSignal | undefined)[] = [];
	listDirectory.mockImplementation((agentId, path, signal) => {
		if (path === stalledPath) {
			signals.push(signal);
			if (signals.length === 1) {
				return new Promise(() => {});
			}
		}
		return fallback ? fallback(agentId, path, signal) : Promise.reject();
	});
	return signals;
};

// Waits until every list-directory call has settled, releasing held calls
// as they arrive when given a release function.
const settled = (release?: () => void) =>
	waitFor(() => {
		release?.();
		expect(
			vi.mocked(API.listWorkspaceAgentDirectory).mock.settledResults,
		).not.toContainEqual({ type: "incomplete", value: undefined });
	});

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
	repoRootsSettled: true,
	repositories: new Map(),
	openFile: vi.fn(),
	revealInFiles: vi.fn(),
	diffStatuses: [],
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

		expect(listedPaths()).toContain("/home/coder/repo/src");
		expect(value.openFile).toHaveBeenCalledWith(
			"/home/coder/repo/src/main.go",
			{
				focus: false,
			},
		);
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

		expect(value.openFile).toHaveBeenCalledWith(
			"/home/coder/repo/src/main.go",
			{
				focus: false,
			},
		);
	});

	it("switches the root and remembers it for the chat", async () => {
		const user = userEvent.setup();
		const { unmount } = renderPanel(contextValue());

		await screen.findByRole("treeitem", { name: "README.md" });
		await user.click(screen.getByRole("button", { name: /^Root directory/ }));
		await user.click(screen.getByRole("menuitemradio", { name: /^Home/ }));
		await screen.findByRole("treeitem", { name: ".bashrc" });

		expect(listedPaths().at(-1)).toBe("~");
		expect(
			localStorage.getItem(`${filesPanelRootStorageKeyPrefix}chat-1`),
		).toBe("~");

		unmount();
		vi.mocked(API.listWorkspaceAgentDirectory).mockClear();
		renderPanel(contextValue());
		await screen.findByRole("treeitem", { name: ".bashrc" });
		expect(listedPaths()).toContain("~");
		expect(listedPaths()).not.toContain("/home/coder/repo");
	});

	it("opens a typed path as the root", async () => {
		const user = userEvent.setup();
		renderPanel(contextValue({ repoRoots: [] }));

		await waitFor(() => expect(listedPaths()).toContain(""));
		await user.click(screen.getByRole("button", { name: /^Root directory/ }));
		await user.click(screen.getByRole("menuitem", { name: "Open path..." }));
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

		await waitFor(() => expect(listedPaths()).toContain("/home/coder/repo"));
		expect(API.listWorkspaceAgentDirectory).toHaveBeenCalledTimes(1);

		await user.click(src);
		await screen.findByRole("treeitem", { name: "util.go" });
	});

	it("focuses the path input when Open path is chosen, and the picker on Escape", async () => {
		const user = userEvent.setup();
		renderPanel(contextValue());

		await screen.findByRole("treeitem", { name: "src" });
		const trigger = screen.getByRole("button", { name: /^Root directory/ });
		await user.click(trigger);
		await user.click(screen.getByRole("menuitem", { name: "Open path..." }));

		await waitFor(() =>
			expect(screen.getByRole("textbox", { name: "Path" })).toHaveFocus(),
		);
		await user.keyboard("{Escape}");
		expect(trigger).toHaveFocus();
	});

	it("keeps Refresh busy for a full spinner turn after a fast refresh", async () => {
		const user = userEvent.setup();
		renderPanel(contextValue());
		await screen.findByRole("treeitem", { name: "src" });
		const refresh = screen.getByRole("button", { name: "Refresh files" });

		await user.click(refresh);
		await settled();

		expect(refresh).toHaveAttribute("aria-busy", "true");
		await waitFor(() => expect(refresh).toHaveAttribute("aria-busy", "false"), {
			timeout: 2000,
		});
	});

	it("moves focus to a remaining row when a refresh drops the focused one", async () => {
		const user = userEvent.setup();
		const listDirectory = vi.mocked(API.listWorkspaceAgentDirectory);
		const fallback = listDirectory.getMockImplementation();
		let mainDeleted = false;
		listDirectory.mockImplementation(async (agentId, path, signal) => {
			if (mainDeleted && path === "/home/coder/repo/src") {
				return {
					...listings[path],
					contents: [entry("/home/coder/repo/src/lib", true)],
				};
			}
			return fallback ? fallback(agentId, path, signal) : Promise.reject();
		});
		renderPanel(contextValue());

		await user.click(await screen.findByRole("treeitem", { name: "src" }));
		const main = await screen.findByRole("treeitem", { name: "main.go" });
		act(() => main.focus());
		mainDeleted = true;
		act(() => {
			focusManager.setFocused(false);
			focusManager.setFocused(true);
		});

		await waitFor(() =>
			expect(screen.getByRole("treeitem", { name: "lib" })).toHaveFocus(),
		);
		focusManager.setFocused(undefined);
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
			await user.click(screen.getByRole("button", { name: /^Root directory/ }));
			await user.click(screen.getByRole("menuitem", { name: "Open path..." }));
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
		vi.mocked(API.listWorkspaceAgentDirectory).mockRejectedValue(apiError(403));
		localStorage.setItem(`${filesPanelRootStorageKeyPrefix}chat-1`, "/root");
		renderPanel(contextValue());

		const alert = await screen.findByRole("alert");
		expect(alert).toHaveTextContent("Permission denied: /root");
		await user.click(screen.getByRole("button", { name: /^Root directory/ }));
		await user.click(screen.getByRole("menuitem", { name: "Open path..." }));
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
			expect(listedPaths()).toContain("/home/coder/repo/src"),
		);
		expect(listedPaths()).toContain("/home/coder/repo");
	});

	it("refetches the tree when the window regains focus", async () => {
		renderPanel(contextValue());
		await screen.findByRole("treeitem", { name: "src" });
		vi.mocked(API.listWorkspaceAgentDirectory).mockClear();

		act(() => {
			focusManager.setFocused(false);
			focusManager.setFocused(true);
		});

		await waitFor(() => expect(listedPaths()).toContain("/home/coder/repo"));
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
		await user.keyboard("{Escape}{ArrowDown}");
		expect(screen.getByRole("treeitem", { name: "debug.log" })).toHaveFocus();
	});

	it("drops an Open path result after another root is picked", async () => {
		const user = userEvent.setup();
		const release = holdListings("/home/coder/repo/src");
		renderPanel(contextValue());

		await screen.findByRole("treeitem", { name: "src" });
		await user.click(screen.getByRole("button", { name: /^Root directory/ }));
		await user.click(screen.getByRole("menuitem", { name: "Open path..." }));
		await user.type(
			screen.getByRole("textbox", { name: "Path" }),
			"/home/coder/repo/src{Enter}",
		);
		await user.click(screen.getByRole("button", { name: /^Root directory/ }));
		await user.click(screen.getByRole("menuitemradio", { name: /^Home/ }));
		await screen.findByRole("treeitem", { name: ".bashrc" });
		release();
		await settled();

		expect(
			localStorage.getItem(`${filesPanelRootStorageKeyPrefix}chat-1`),
		).toBe("~");
		screen.getByRole("treeitem", { name: ".bashrc" });
	});

	it("returns focus to the picker after Open path only if the form kept it", async () => {
		const user = userEvent.setup();
		const release = holdListings("/home/coder/repo/src");
		renderPanel(contextValue());
		render(<input aria-label="Composer" />);
		const trigger = await screen.findByRole("button", {
			name: /^Root directory/,
		});
		const openPath = async () => {
			await user.click(trigger);
			await user.click(screen.getByRole("menuitem", { name: "Open path..." }));
			await user.type(
				screen.getByRole("textbox", { name: "Path" }),
				"/home/coder/repo/src{Enter}",
			);
		};

		await openPath();
		const composer = screen.getByRole("textbox", { name: "Composer" });
		await user.click(composer);
		await user.keyboard("hi");
		const releaseAll = () =>
			waitFor(() => {
				release();
				expect(
					vi.mocked(API.listWorkspaceAgentDirectory).mock.settledResults,
				).not.toContainEqual({ type: "incomplete", value: undefined });
			});
		await releaseAll();
		await screen.findByRole("treeitem", { name: "main.go" });
		expect(composer).toHaveFocus();

		await openPath();
		await releaseAll();
		await waitFor(() => expect(trigger).toHaveFocus());
	});

	it("coalesces repository refreshes into one request and one trailing request", async () => {
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
		const release = holdListings("/home/coder/repo/src");
		vi.mocked(API.listWorkspaceAgentDirectory).mockClear();
		const srcRequests = () =>
			listedPaths().filter((path) => path === "/home/coder/repo/src");

		for (const diff of ["a", "b", "c"]) {
			rerenderPanel({
				...value,
				repositories: new Map([
					[repository.repo_root, { ...repository, unified_diff: diff }],
				]),
			});
		}
		await waitFor(() => expect(srcRequests()).toHaveLength(1));
		expect(
			vi.mocked(API.listWorkspaceAgentDirectory).mock.calls[0][2],
		).toBeInstanceOf(AbortSignal);

		release();
		await waitFor(() => expect(srcRequests()).toHaveLength(2));
		release();
		await settled();
		expect(srcRequests()).toHaveLength(2);
	});

	it("stops querying an expanded directory once it is deleted", async () => {
		const user = userEvent.setup();
		const listDirectory = vi.mocked(API.listWorkspaceAgentDirectory);
		const fallback = listDirectory.getMockImplementation();
		let libDeleted = false;
		listDirectory.mockImplementation(async (agentId, path, signal) => {
			if (libDeleted && path === "/home/coder/repo/src") {
				return {
					...listings[path],
					contents: [entry("/home/coder/repo/src/main.go", false)],
				};
			}
			return fallback ? fallback(agentId, path, signal) : Promise.reject();
		});
		renderPanel(contextValue());

		await user.click(await screen.findByRole("treeitem", { name: "src" }));
		await user.click(await screen.findByRole("treeitem", { name: "lib" }));
		await screen.findByRole("treeitem", { name: "util.go" });
		libDeleted = true;
		const refresh = screen.getByRole("button", { name: "Refresh files" });
		await user.click(refresh);
		await waitFor(() =>
			expect(screen.queryByRole("treeitem", { name: "lib" })).toBeNull(),
		);
		await settled();
		listDirectory.mockClear();
		await user.click(refresh);

		await waitFor(() =>
			expect(listedPaths()).toContain("/home/coder/repo/src"),
		);
		expect(listedPaths()).not.toContain("/home/coder/repo/src/lib");
	});

	it("groups roots under Filesystem and Repositories headings", async () => {
		const user = userEvent.setup();
		renderPanel(contextValue());

		await screen.findByRole("treeitem", { name: "src" });
		const trigger = screen.getByRole("button", { name: /^Root directory/ });
		await waitFor(() => expect(listedPaths()).toContain("~"));
		await user.click(trigger);

		expect(screen.getByRole("menu")).toHaveTextContent(
			/^FilesystemRoot\/Home~Repositoriesrepo~\/repoOpen path\.\.\.$/,
		);
		expect(
			screen.getByRole("menuitemradio", { name: /^repo/ }),
		).toHaveAttribute("aria-checked", "true");
		expect(trigger).toHaveTextContent(/^repo$/);
	});

	it("lists a default directory apart from home under Filesystem", async () => {
		const user = userEvent.setup();
		renderPanel(
			contextValue({ defaultDirectory: "/home/coder/work", repoRoots: [] }),
		);

		await waitFor(() => expect(listedPaths()).toContain("~"));
		await user.click(screen.getByRole("button", { name: /^Root directory/ }));

		expect(screen.getByRole("menu")).toHaveTextContent(
			/^FilesystemRoot\/Home~work~\/workOpen path\.\.\.$/,
		);
	});

	it("cancels a hidden directory's fetch and lets Refresh restart a stalled one", async () => {
		const user = userEvent.setup();
		const listDirectory = vi.mocked(API.listWorkspaceAgentDirectory);
		const fallback = listDirectory.getMockImplementation();
		const libSignals: (AbortSignal | undefined)[] = [];
		let stallLib = false;
		listDirectory.mockImplementation(async (agentId, path, signal) => {
			if (path === "/home/coder/repo/src/lib") {
				libSignals.push(signal);
				if (stallLib) {
					return new Promise(() => {});
				}
				return {
					...listings[path],
					contents: [entry("/home/coder/repo/src/lib/fresh.go", false)],
				};
			}
			return fallback ? fallback(agentId, path, signal) : Promise.reject();
		});
		stallLib = true;
		renderPanel(contextValue());
		const refresh = await screen.findByRole("button", {
			name: "Refresh files",
		});
		const src = await screen.findByRole("treeitem", { name: "src" });
		await user.click(src);
		await user.click(await screen.findByRole("treeitem", { name: "lib" }));
		await waitFor(() => expect(libSignals).toHaveLength(1));

		await user.click(src);
		await waitFor(() => expect(libSignals[0]?.aborted).toBe(true));

		await user.click(src);
		await waitFor(() => expect(libSignals).toHaveLength(2));
		stallLib = false;
		await user.click(refresh);

		await screen.findByRole("treeitem", { name: "fresh.go" });
		expect(libSignals[1]?.aborted).toBe(true);
	});

	it("reveals a nested file inside the root: expands, selects and focuses it", async () => {
		const { onRevealHandled } = renderRevealed({
			path: "/home/coder/repo/src/lib/util.go",
			isDirectory: false,
		});

		const util = await screen.findByRole("treeitem", { name: "util.go" });
		await waitFor(() => expect(util).toHaveFocus());
		expect(util).toHaveAttribute("aria-selected", "true");
		expect(screen.getByRole("treeitem", { name: "src" })).toHaveAttribute(
			"aria-expanded",
			"true",
		);
		expect(onRevealHandled).toHaveBeenCalledTimes(1);
		expect(
			localStorage.getItem(`${filesPanelRootStorageKeyPrefix}chat-1`),
		).toBeNull();
	});

	it("reveals and expands a folder", async () => {
		renderRevealed({ path: "/home/coder/repo/src", isDirectory: true });

		await screen.findByRole("treeitem", { name: "main.go" });
		const src = screen.getByRole("treeitem", { name: "src" });
		expect(src).toHaveAttribute("aria-expanded", "true");
		expect(src).toHaveAttribute("aria-selected", "true");
	});

	it("roots a reveal outside the root at its repository", async () => {
		localStorage.setItem(
			`${filesPanelRootStorageKeyPrefix}chat-1`,
			"/home/coder/repo/src",
		);
		renderRevealed({ path: "/home/coder/repo/README.md", isDirectory: false });

		const readme = await screen.findByRole("treeitem", { name: "README.md" });
		await waitFor(() => expect(readme).toHaveFocus());
		expect(
			localStorage.getItem(`${filesPanelRootStorageKeyPrefix}chat-1`),
		).toBe("/home/coder/repo");
	});

	it("roots a reveal outside every repository at its folder", async () => {
		renderRevealed({ path: "/home/coder/notes.md", isDirectory: false });

		const notes = await screen.findByRole("treeitem", { name: "notes.md" });
		await waitFor(() => expect(notes).toHaveFocus());
		expect(
			localStorage.getItem(`${filesPanelRootStorageKeyPrefix}chat-1`),
		).toBe("/home/coder");
	});

	it("keeps a root that contains the revealed path", async () => {
		localStorage.setItem(`${filesPanelRootStorageKeyPrefix}chat-1`, "~");
		renderRevealed({ path: "/home/coder/notes.md", isDirectory: false });

		const notes = await screen.findByRole("treeitem", { name: "notes.md" });
		await waitFor(() => expect(notes).toHaveFocus());
		expect(
			localStorage.getItem(`${filesPanelRootStorageKeyPrefix}chat-1`),
		).toBe("~");
	});

	it("refetches a cached listing to reveal a file created since", async () => {
		const { reveal } = renderRevealed(undefined);
		await screen.findByRole("treeitem", { name: "README.md" });
		const listDirectory = vi.mocked(API.listWorkspaceAgentDirectory);
		const fallback = listDirectory.getMockImplementation();
		const repo = listings["/home/coder/repo"];
		listDirectory.mockImplementation(async (agentId, path, signal) =>
			path === repo.absolute_path_string
				? {
						...repo,
						contents: [
							...repo.contents,
							entry("/home/coder/repo/new.txt", false),
						],
					}
				: fallback
					? fallback(agentId, path, signal)
					: Promise.reject(),
		);

		reveal({ path: "/home/coder/repo/new.txt", isDirectory: false });

		const created = await screen.findByRole("treeitem", { name: "new.txt" });
		await waitFor(() => expect(created).toHaveFocus());
	});

	it("drops a pending reveal after a path is opened", async () => {
		const user = userEvent.setup();
		const { reveal } = renderRevealed(undefined);
		await screen.findByRole("treeitem", { name: "src" });
		const release = holdListings("/home/coder/repo/src/lib");

		reveal({ path: "/home/coder/repo/src/lib/util.go", isDirectory: false });
		await waitFor(() =>
			expect(listedPaths()).toContain("/home/coder/repo/src/lib"),
		);
		await user.click(screen.getByRole("button", { name: /^Root directory/ }));
		await user.click(screen.getByRole("menuitem", { name: "Open path..." }));
		await user.type(
			screen.getByRole("textbox", { name: "Path" }),
			"/home/coder{Enter}",
		);
		await screen.findByRole("treeitem", { name: ".bashrc" });
		await settled(release);

		expect(
			localStorage.getItem(`${filesPanelRootStorageKeyPrefix}chat-1`),
		).toBe("/home/coder");
		screen.getByRole("treeitem", { name: ".bashrc" });
	});

	it("leaves focus where the user moved it while a reveal loaded", async () => {
		const user = userEvent.setup();
		const { reveal } = renderRevealed(undefined);
		render(<input aria-label="Composer" />);
		await screen.findByRole("treeitem", { name: "src" });
		const release = holdListings("/home/coder/repo/src/lib");

		reveal({ path: "/home/coder/repo/src/lib/util.go", isDirectory: false });
		await waitFor(() =>
			expect(listedPaths()).toContain("/home/coder/repo/src/lib"),
		);
		const composer = screen.getByRole("textbox", { name: "Composer" });
		await user.click(composer);
		await user.keyboard("hi");
		await waitFor(() => {
			release();
			expect(screen.getByRole("treeitem", { name: "util.go" })).toHaveAttribute(
				"aria-selected",
				"true",
			);
		});
		expect(composer).toHaveFocus();
	});

	it("restarts its own stalled lookups when a path is revealed again", async () => {
		const { reveal } = renderRevealed(undefined);
		await screen.findByRole("treeitem", { name: "src" });
		const signals = stallFirstListing("/home/coder/repo/src/lib");
		const request = {
			path: "/home/coder/repo/src/lib/util.go",
			isDirectory: false,
		};

		reveal({ ...request });
		await waitFor(() => expect(signals).toHaveLength(1));
		reveal({ ...request });

		const util = await screen.findByRole("treeitem", { name: "util.go" });
		await waitFor(() => expect(util).toHaveAttribute("aria-selected", "true"));
		expect(signals[0]?.aborted).toBe(true);
	});

	it("lets Refresh restart the lookups a pending reveal waits on", async () => {
		const user = userEvent.setup();
		const { reveal } = renderRevealed(undefined);
		await screen.findByRole("treeitem", { name: "src" });
		const signals = stallFirstListing("/home/coder/repo/src/lib");

		reveal({ path: "/home/coder/repo/src/lib/util.go", isDirectory: false });
		await waitFor(() => expect(signals).toHaveLength(1));
		await user.click(screen.getByRole("button", { name: "Refresh files" }));

		const util = await screen.findByRole("treeitem", { name: "util.go" });
		await waitFor(() => expect(util).toHaveAttribute("aria-selected", "true"));
		expect(signals[0]?.aborted).toBe(true);
	});

	it("waits for the git watcher before rooting at the chat's repo", async () => {
		const { rerenderPanel } = renderPanel(
			contextValue({ repoRoots: [], repoRootsSettled: false }),
		);
		expect(screen.getByRole("status", { name: "Loading" })).toBeInTheDocument();

		rerenderPanel(contextValue());

		await screen.findByRole("treeitem", { name: "src" });
		expect(listedPaths()).not.toContain("");
	});

	it("roots at the default directory once a watcher without repos settles", async () => {
		const { rerenderPanel } = renderPanel(
			contextValue({ repoRoots: [], repoRootsSettled: false }),
		);
		expect(listedPaths()).not.toContain("");

		rerenderPanel(contextValue({ repoRoots: [], repoRootsSettled: true }));

		await waitFor(() => expect(listedPaths()).toContain(""));
	});

	it("keeps the resolved root when the watcher reconnects or reports a repo later", async () => {
		const { rerenderPanel } = renderPanel(contextValue());
		await screen.findByRole("treeitem", { name: "src" });

		rerenderPanel(contextValue({ repoRoots: [] }));
		rerenderPanel(contextValue({ repoRoots: ["/home/coder/other"] }));

		expect(screen.queryByRole("status", { name: "Loading" })).toBeNull();
		expect(screen.getByRole("treeitem", { name: "src" })).toBeInTheDocument();
		expect(listedPaths()).not.toContain("");
		expect(listedPaths()).not.toContain("/home/coder/other");
	});

	it("moves a default-directory root to a repo that appears before the user navigates", async () => {
		const { rerenderPanel } = renderPanel(contextValue({ repoRoots: [] }));
		await waitFor(() => expect(listedPaths()).toContain(""));

		rerenderPanel(contextValue());

		await screen.findByRole("treeitem", { name: "src" });
		rerenderPanel(
			contextValue({ repoRoots: ["/home/coder", "/home/coder/repo"] }),
		);
		expect(screen.getByRole("treeitem", { name: "src" })).toBeInTheDocument();
		expect(listedPaths()).not.toContain("/home/coder");
	});

	it("keeps a default-directory root once the user expanded a folder in it", async () => {
		const user = userEvent.setup();
		const listDirectory = vi.mocked(API.listWorkspaceAgentDirectory);
		const fallback = listDirectory.getMockImplementation();
		listDirectory.mockImplementation((agentId, path, signal) =>
			path === ""
				? Promise.resolve({
						...listings["/home/coder"],
						contents: [entry("/home/coder/repo", true)],
					})
				: fallback
					? fallback(agentId, path, signal)
					: Promise.reject(),
		);
		const { rerenderPanel } = renderPanel(contextValue({ repoRoots: [] }));
		await user.click(await screen.findByRole("treeitem", { name: "repo" }));
		await screen.findByRole("treeitem", { name: "src" });

		rerenderPanel(contextValue());

		expect(screen.getByRole("treeitem", { name: "repo" })).toHaveAttribute(
			"aria-expanded",
			"true",
		);
		expect(
			localStorage.getItem(`${filesPanelRootStorageKeyPrefix}chat-1`),
		).toBeNull();
	});

	it("lets the user pick a root while the git watcher has not settled", async () => {
		const user = userEvent.setup();
		renderPanel(contextValue({ repoRoots: [], repoRootsSettled: false }));
		expect(screen.getByRole("status", { name: "Loading" })).toBeInTheDocument();

		await user.click(
			screen.getByRole("button", { name: "Root directory: Loading" }),
		);
		await user.click(screen.getByRole("menuitemradio", { name: /^Home/ }));

		await screen.findByRole("treeitem", { name: ".bashrc" });
		expect(
			localStorage.getItem(`${filesPanelRootStorageKeyPrefix}chat-1`),
		).toBe("~");
	});

	it("keeps a default-directory root when a repo appears during a reveal", async () => {
		const listDirectory = vi.mocked(API.listWorkspaceAgentDirectory);
		const fallback = listDirectory.getMockImplementation();
		listDirectory.mockImplementation((agentId, path, signal) =>
			path === ""
				? Promise.resolve(listings["/home/coder"])
				: fallback
					? fallback(agentId, path, signal)
					: Promise.reject(),
		);
		const value = contextValue({ repoRoots: [] });
		const { reveal } = renderRevealed(undefined, value);
		await screen.findByRole("treeitem", { name: "notes.md" });
		const release = holdListings("");
		const request = { path: "/home/coder/notes.md", isDirectory: false };

		reveal(request);
		await waitFor(() =>
			expect(listedPaths().filter((path) => path === "")).toHaveLength(2),
		);
		reveal(request, contextValue());

		expect(
			screen.getByRole("treeitem", { name: "notes.md" }),
		).toBeInTheDocument();
		await settled(release);
		await waitFor(() =>
			expect(
				screen.getByRole("treeitem", { name: "notes.md" }),
			).toHaveAttribute("aria-selected", "true"),
		);
		expect(
			localStorage.getItem(`${filesPanelRootStorageKeyPrefix}chat-1`),
		).toBeNull();
	});

	it("does not list files without a workspace agent", () => {
		renderPanel(undefined);
		expect(API.listWorkspaceAgentDirectory).not.toHaveBeenCalled();
	});
});
