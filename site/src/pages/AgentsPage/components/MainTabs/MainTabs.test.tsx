import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useEffect, useRef, useSyncExternalStore } from "react";
import { focusManager, QueryClientProvider } from "react-query";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { API, type WorkspaceAgentFileContents } from "#/api/api";
import type {
	ChatMessage,
	WorkspaceAgentRepoChanges,
} from "#/api/typesGenerated";
import { createDeferred } from "#/testHelpers/deferred";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { WorkspaceFilesContext } from "../../context/WorkspaceFilesContext";
import {
	getPersistedMainTabs,
	savePersistedMainTabs,
} from "../../utils/mainTabStorage";
import { type ChatStore, createChatStore } from "../ChatConversation/chatStore";
import type { StreamState } from "../ChatConversation/types";
import type FileEditor from "../FileTab/FileEditor";
import { MainTabs } from "./MainTabs";
import { useMainTabs } from "./useMainTabs";

vi.mock("../FileTab/FileEditor", () => ({
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

const chatId = "chat-main-tabs";
const chatMounted = vi.fn();

const ChatStub: React.FC<{ ref: React.Ref<HTMLTextAreaElement> }> = ({
	ref,
}) => {
	useEffect(() => {
		chatMounted();
	}, []);
	return <textarea ref={ref} aria-label="Message" />;
};

type Repositories = ReadonlyMap<string, WorkspaceAgentRepoChanges>;

/** Stands in for the git watcher, whose state the tests set. */
const createRepositoriesStore = () => {
	let repositories: Repositories = new Map();
	const listeners = new Set<() => void>();
	return {
		get: () => repositories,
		set: (next: Repositories) => {
			repositories = next;
			for (const listener of listeners) {
				listener();
			}
		},
		subscribe: (listener: () => void) => {
			listeners.add(listener);
			return () => {
				listeners.delete(listener);
			};
		},
	};
};

type HarnessProps = {
	paths: readonly string[];
	isAgentConnected?: boolean;
	store: ChatStore;
	repositoriesStore: ReturnType<typeof createRepositoriesStore>;
};

const Harness: React.FC<HarnessProps> = ({
	paths,
	isAgentConnected = true,
	store,
	repositoriesStore,
}) => {
	const mainTabs = useMainTabs(chatId);
	const chatInputRef = useRef<HTMLTextAreaElement>(null);
	const repositories = useSyncExternalStore(
		repositoriesStore.subscribe,
		repositoriesStore.get,
	);
	return (
		<WorkspaceFilesContext
			value={
				isAgentConnected
					? {
							agentId: "agent-1",
							defaultDirectory: "/home/coder",
							repoRoots: ["/repo"],
							repositories,
							openFile: mainTabs.openFile,
						}
					: undefined
			}
		>
			{paths.map((path) => (
				<button
					key={path}
					type="button"
					onClick={() => mainTabs.openFile(path)}
				>
					{`Open ${path}`}
				</button>
			))}
			{paths.map((path) => (
				<button
					key={path}
					type="button"
					onClick={() => mainTabs.openFile(path, { focus: false })}
				>
					{`Preview ${path}`}
				</button>
			))}
			<textarea aria-label="Files filter" />
			<div className="xterm">
				<textarea aria-label="Terminal" />
			</div>
			<MainTabs
				mainTabs={mainTabs}
				store={store}
				readOnly={false}
				filesEnabled
				focusChatInput={() => chatInputRef.current?.focus()}
			>
				<ChatStub ref={chatInputRef} />
			</MainTabs>
		</WorkspaceFilesContext>
	);
};

const renderHarness = (
	paths: readonly string[] = [],
	{
		isAgentConnected,
		streamState = null,
	}: { isAgentConnected?: boolean; streamState?: StreamState | null } = {},
) => {
	const queryClient = createTestQueryClient();
	const store = createChatStore();
	store.setStreamState(streamState);
	const repositoriesStore = createRepositoriesStore();
	const router = createMemoryRouter([
		{
			path: "/",
			element: (
				<Harness
					paths={paths}
					isAgentConnected={isAgentConnected}
					store={store}
					repositoriesStore={repositoriesStore}
				/>
			),
		},
		{ path: "/other", element: <p>Other page</p> },
	]);
	const result = render(
		<QueryClientProvider client={queryClient}>
			<RouterProvider router={router} />
		</QueryClientProvider>,
	);
	return {
		...result,
		router,
		setStreamState: (state: StreamState) =>
			act(() => store.setStreamState(state)),
		upsertMessages: (messages: readonly ChatMessage[]) =>
			act(() => store.upsertDurableMessages(messages)),
		setRepositories: (repositories: Repositories) =>
			act(() => repositoriesStore.set(repositories)),
	};
};

const repoWithDiff = (unifiedDiff: string, branch = "main"): Repositories =>
	new Map([
		["/repo", { repo_root: "/repo", branch, unified_diff: unifiedDiff }],
	]);

const fileDiff = (path: string, line: string) =>
	`diff --git a/${path} b/${path}\n--- a/${path}\n+++ b/${path}\n@@ -1 +1 @@\n-package main\n+${line}\n`;

const fileToolStream = (
	id: string,
	name: string,
	args: unknown,
): StreamState => ({
	blocks: [],
	toolCalls: { [id]: { id, name, args } },
	toolResults: { [id]: { id, name, isError: false } },
	sources: [],
});

/** The durable messages of a completed write_file call. */
const writeFileMessages = (
	firstMessageID: number,
	id: string,
	path: string,
): ChatMessage[] => [
	{
		id: firstMessageID,
		chat_id: chatId,
		created_at: "2026-01-01T00:00:00Z",
		role: "assistant",
		content: [
			{
				type: "tool-call",
				tool_call_id: id,
				tool_name: "write_file",
				args: { path },
			},
		],
	},
	{
		id: firstMessageID + 1,
		chat_id: chatId,
		created_at: "2026-01-01T00:00:01Z",
		role: "tool",
		content: [
			{
				type: "tool-result",
				tool_call_id: id,
				tool_name: "write_file",
				result: {},
			},
		],
	},
];

const getTab = (name: string | RegExp) => screen.getByRole("tab", { name });

describe("MainTabs", () => {
	beforeEach(() => {
		localStorage.clear();
		chatMounted.mockClear();
		vi.spyOn(API, "readWorkspaceAgentFile").mockResolvedValue({
			data: new TextEncoder().encode("package main\n").buffer,
			contentType: "text/plain; charset=utf-8",
			etag: '"v1"',
		});
	});

	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("opens a file tab once and activates it when opened again", async () => {
		const user = userEvent.setup();
		renderHarness(["/repo/main.go"]);

		await user.click(
			screen.getByRole("button", { name: "Open /repo/main.go" }),
		);
		expect(getTab("main.go")).toHaveAttribute("aria-selected", "true");

		await user.click(getTab("Chat"));
		expect(getTab("Chat")).toHaveAttribute("aria-selected", "true");

		await user.click(
			screen.getByRole("button", { name: "Open /repo/main.go" }),
		);
		expect(screen.getAllByRole("tab", { name: /main\.go/ })).toHaveLength(1);
		expect(getTab("main.go")).toHaveAttribute("aria-selected", "true");
		expect(getPersistedMainTabs(chatId)).toEqual({
			paths: ["/repo/main.go"],
			activePath: "/repo/main.go",
		});
	});

	it("names tabs that share a basename by their parent directory", async () => {
		const user = userEvent.setup();
		renderHarness(["/repo/a/index.ts", "/repo/b/index.ts"]);

		await user.click(
			screen.getByRole("button", { name: "Open /repo/a/index.ts" }),
		);
		await user.click(
			screen.getByRole("button", { name: "Open /repo/b/index.ts" }),
		);

		expect(getTab("index.ts b")).toHaveAttribute("aria-selected", "true");
		await user.click(getTab("index.ts a"));
		expect(getPersistedMainTabs(chatId).activePath).toBe("/repo/a/index.ts");
	});

	it("closes tabs with the close button and with a middle click", async () => {
		const user = userEvent.setup();
		renderHarness(["/repo/a.go", "/repo/b.go"]);

		await user.click(screen.getByRole("button", { name: "Open /repo/a.go" }));
		await user.click(screen.getByRole("button", { name: "Open /repo/b.go" }));

		await user.click(screen.getByRole("button", { name: "Close b.go" }));
		expect(getPersistedMainTabs(chatId)).toEqual({
			paths: ["/repo/a.go"],
			activePath: "/repo/a.go",
		});

		await user.pointer({ keys: "[MouseMiddle]", target: getTab("a.go") });
		expect(getPersistedMainTabs(chatId)).toEqual({
			paths: [],
			activePath: null,
		});
	});

	it("restores open tabs and the active tab for the chat", async () => {
		const user = userEvent.setup();
		const { unmount } = renderHarness(["/repo/a.go", "/repo/b.go"]);

		await user.click(screen.getByRole("button", { name: "Open /repo/a.go" }));
		await user.click(screen.getByRole("button", { name: "Open /repo/b.go" }));
		await user.click(getTab("a.go"));
		unmount();

		renderHarness();
		expect(getTab("a.go")).toHaveAttribute("aria-selected", "true");
		expect(getTab("b.go")).toHaveAttribute("aria-selected", "false");
	});

	it("shows restored tabs before the workspace agent connects", async () => {
		savePersistedMainTabs(chatId, {
			paths: ["/repo/a.go"],
			activePath: "/repo/a.go",
		});
		renderHarness([], { isAgentConnected: false });

		const tab = await screen.findByRole("tab", { name: "a.go" });
		expect(tab).toHaveAttribute("aria-selected", "true");
		const panel = screen.getByRole("tabpanel", { name: "a.go" });
		expect(within(panel).getByRole("status")).toHaveAccessibleName(
			"Waiting for the workspace agent",
		);
	});

	it("cycles tabs with Alt+[ and Alt+] and focuses their content", async () => {
		const user = userEvent.setup();
		renderHarness(["/repo/a.go", "/repo/b.go"]);

		await user.click(screen.getByRole("button", { name: "Open /repo/a.go" }));
		await user.click(screen.getByRole("button", { name: "Open /repo/b.go" }));
		await user.click(getTab("b.go"));

		await user.keyboard("{Alt>}[BracketRight]{/Alt}");
		expect(getTab("Chat")).toHaveAttribute("aria-selected", "true");
		expect(screen.getByLabelText("Message")).toHaveFocus();

		await user.keyboard("{Alt>}[BracketRight]{/Alt}");
		expect(getTab("a.go")).toHaveAttribute("aria-selected", "true");
		const panel = screen.getByRole("tabpanel", { name: "a.go" });
		await waitFor(() => {
			expect(within(panel).getByLabelText("File contents")).toHaveFocus();
		});

		await user.keyboard("{Alt>}[BracketLeft]{/Alt}");
		expect(getTab("Chat")).toHaveAttribute("aria-selected", "true");
		expect(screen.getByLabelText("Message")).toHaveFocus();
		expect(within(panel).getByLabelText("File contents")).toHaveValue(
			"package main\n",
		);
	});

	it("cycles tabs from anywhere but a terminal", async () => {
		const user = userEvent.setup();
		renderHarness(["/repo/a.go"]);

		await user.click(screen.getByRole("button", { name: "Open /repo/a.go" }));
		await user.click(screen.getByLabelText("Terminal"));
		await user.keyboard("{Alt>}[BracketRight]{/Alt}");
		expect(getTab("a.go")).toHaveAttribute("aria-selected", "true");
		expect(screen.getByLabelText("Terminal")).toHaveFocus();

		await user.click(screen.getByLabelText("Files filter"));
		await user.keyboard("{Alt>}[BracketRight]{/Alt}");
		expect(getTab("Chat")).toHaveAttribute("aria-selected", "true");
		expect(screen.getByLabelText("Message")).toHaveFocus();
	});

	it("focuses a file once it opens, unless asked not to", async () => {
		const user = userEvent.setup();
		renderHarness(["/repo/a.go", "/repo/b.go"]);

		await user.click(screen.getByRole("button", { name: "Open /repo/a.go" }));
		await waitFor(() => {
			expect(screen.getByLabelText("File contents")).toHaveFocus();
		});

		const preview = screen.getByRole("button", { name: "Preview /repo/b.go" });
		await user.click(preview);
		const panel = screen.getByRole("tabpanel", { name: "b.go" });
		await within(panel).findByLabelText("File contents");
		expect(getTab("b.go")).toHaveAttribute("aria-selected", "true");
		expect(preview).toHaveFocus();
	});

	it("focuses the next tab's content after closing the active tab", async () => {
		const user = userEvent.setup();
		renderHarness(["/repo/a.go", "/repo/b.go"]);

		await user.click(screen.getByRole("button", { name: "Open /repo/a.go" }));
		await user.click(screen.getByRole("button", { name: "Open /repo/b.go" }));
		await user.click(getTab("b.go"));
		await user.keyboard("{Delete}");
		const panel = screen.getByRole("tabpanel", { name: "a.go" });
		await waitFor(() => {
			expect(within(panel).getByLabelText("File contents")).toHaveFocus();
		});

		await user.click(screen.getByRole("button", { name: "Close a.go" }));
		expect(screen.getByLabelText("Message")).toHaveFocus();
	});

	it("closes the focused tab with Delete", async () => {
		const user = userEvent.setup();
		renderHarness(["/repo/a.go", "/repo/b.go"]);

		await user.click(screen.getByRole("button", { name: "Open /repo/a.go" }));
		await user.click(screen.getByRole("button", { name: "Open /repo/b.go" }));
		await user.click(getTab("a.go"));
		await user.keyboard("{Delete}");

		expect(getPersistedMainTabs(chatId)).toEqual({
			paths: ["/repo/b.go"],
			activePath: "/repo/b.go",
		});
	});

	it("closes other tabs and tabs to the right from a tab's menu", async () => {
		const user = userEvent.setup();
		renderHarness(["/repo/a.go", "/repo/b.go", "/repo/c.go"]);

		await user.click(screen.getByRole("button", { name: "Open /repo/a.go" }));
		await user.click(screen.getByRole("button", { name: "Open /repo/b.go" }));
		await user.click(screen.getByRole("button", { name: "Open /repo/c.go" }));

		await user.pointer({ keys: "[MouseRight]", target: getTab("b.go") });
		await user.click(
			await screen.findByRole("menuitem", { name: "Close to the right" }),
		);
		expect(getPersistedMainTabs(chatId)).toEqual({
			paths: ["/repo/a.go", "/repo/b.go"],
			activePath: "/repo/b.go",
		});

		await user.pointer({ keys: "[MouseRight]", target: getTab("a.go") });
		await user.click(
			await screen.findByRole("menuitem", { name: "Close others" }),
		);
		expect(getPersistedMainTabs(chatId)).toEqual({
			paths: ["/repo/a.go"],
			activePath: "/repo/a.go",
		});
	});

	it("copies a tab's path and its path in the repository", async () => {
		const user = userEvent.setup();
		const writeText = vi
			.spyOn(navigator.clipboard, "writeText")
			.mockResolvedValue(undefined);
		renderHarness(["/repo/src/a.go"]);

		await user.click(
			screen.getByRole("button", { name: "Open /repo/src/a.go" }),
		);
		await user.pointer({ keys: "[MouseRight]", target: getTab("a.go") });
		await user.click(
			await screen.findByRole("menuitem", { name: "Copy relative path" }),
		);
		expect(writeText).toHaveBeenLastCalledWith("src/a.go");

		await user.pointer({ keys: "[MouseRight]", target: getTab("a.go") });
		await user.click(
			await screen.findByRole("menuitem", { name: "Copy path" }),
		);
		expect(writeText).toHaveBeenLastCalledWith("/repo/src/a.go");
	});

	it("moves between tabs with the arrow keys", async () => {
		const user = userEvent.setup();
		renderHarness(["/repo/a.go", "/repo/b.go"]);

		await user.click(screen.getByRole("button", { name: "Open /repo/a.go" }));
		await user.click(screen.getByRole("button", { name: "Open /repo/b.go" }));
		await user.click(getTab("Chat"));

		await user.keyboard("{ArrowRight}");
		expect(getTab("a.go")).toHaveAttribute("aria-selected", "true");
		expect(getTab("a.go")).toHaveFocus();

		await user.keyboard("{End}");
		expect(getTab("b.go")).toHaveFocus();
		await user.keyboard("{ArrowRight}");
		expect(getTab("Chat")).toHaveAttribute("aria-selected", "true");
		expect(getTab("Chat")).toHaveFocus();
	});

	it("lists all tabs in a menu while they overflow the strip", async () => {
		vi.spyOn(Element.prototype, "scrollWidth", "get").mockReturnValue(1000);
		vi.spyOn(Element.prototype, "clientWidth", "get").mockReturnValue(100);
		const user = userEvent.setup();
		renderHarness(["/repo/a.go", "/repo/b.go"]);

		await user.click(screen.getByRole("button", { name: "Open /repo/a.go" }));
		await user.click(screen.getByRole("button", { name: "Open /repo/b.go" }));
		await user.click(screen.getByRole("button", { name: "All tabs" }));
		await user.click(
			await screen.findByRole("menuitemradio", { name: "a.go" }),
		);

		expect(getPersistedMainTabs(chatId).activePath).toBe("/repo/a.go");
	});

	it("keeps the chat mounted while a file tab is active", async () => {
		const user = userEvent.setup();
		renderHarness(["/repo/main.go"]);

		await user.type(screen.getByLabelText("Message"), "draft reply");
		await user.click(
			screen.getByRole("button", { name: "Open /repo/main.go" }),
		);
		await screen.findByLabelText("File contents");
		await user.click(getTab("Chat"));

		expect(chatMounted).toHaveBeenCalledTimes(1);
		expect(screen.getByLabelText("Message")).toHaveValue("draft reply");
	});

	it("asks before closing a tab with unsaved edits", async () => {
		const user = userEvent.setup();
		renderHarness(["/repo/main.go"]);

		await user.click(
			screen.getByRole("button", { name: "Open /repo/main.go" }),
		);
		await user.type(await screen.findByLabelText("File contents"), "// edit");
		await user.click(screen.getByRole("button", { name: "Close main.go" }));

		const dialog = await screen.findByRole("dialog");
		await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
		expect(getTab("main.go, unsaved")).toHaveAttribute("aria-selected", "true");
		await waitFor(() => {
			expect(getTab("main.go, unsaved")).toHaveFocus();
		});
		expect(getPersistedMainTabs(chatId).paths).toEqual(["/repo/main.go"]);

		await user.keyboard("{Delete}");
		await user.click(
			within(await screen.findByRole("dialog")).getByRole("button", {
				name: "Discard",
			}),
		);
		await waitFor(() => {
			expect(getPersistedMainTabs(chatId).paths).toEqual([]);
		});
		await waitFor(() => {
			expect(screen.getByLabelText("Message")).toHaveFocus();
		});
	});

	describe("refreshing files", () => {
		const reads = (path: string) =>
			vi
				.mocked(API.readWorkspaceAgentFile)
				.mock.calls.filter(([, readPath]) => readPath === path).length;
		const textFile = (content: string): WorkspaceAgentFileContents => ({
			data: new TextEncoder().encode(content).buffer,
			contentType: "text/plain; charset=utf-8",
			etag: `"${content}"`,
		});
		// Lets a refresh that is due start its read.
		const settle = () =>
			act(() => new Promise((resolve) => setTimeout(resolve, 0)));

		afterEach(() => {
			focusManager.setFocused(undefined);
		});

		it("rereads the active file when its diff changes, and others once activated", async () => {
			const user = userEvent.setup();
			const { setRepositories } = renderHarness(["/repo/a.go", "/repo/b.go"]);
			await user.click(screen.getByRole("button", { name: "Open /repo/a.go" }));
			await user.click(screen.getByRole("button", { name: "Open /repo/b.go" }));
			setRepositories(repoWithDiff(""));

			setRepositories(repoWithDiff(fileDiff("a.go", "// a")));
			setRepositories(
				repoWithDiff(fileDiff("a.go", "// a") + fileDiff("b.go", "// b")),
			);
			await waitFor(() => {
				expect(reads("/repo/b.go")).toBe(2);
			});
			expect(reads("/repo/a.go")).toBe(1);

			await user.click(getTab("a.go"));
			await waitFor(() => {
				expect(reads("/repo/a.go")).toBe(2);
			});
			await user.click(getTab("b.go"));
			expect(reads("/repo/b.go")).toBe(2);
		});

		it("rereads clean files when the branch changes", async () => {
			const user = userEvent.setup();
			const { setRepositories } = renderHarness(["/repo/a.go"]);
			await user.click(screen.getByRole("button", { name: "Open /repo/a.go" }));
			setRepositories(repoWithDiff(""));

			setRepositories(repoWithDiff("", "feature"));
			await waitFor(() => {
				expect(reads("/repo/a.go")).toBe(2);
			});
		});

		it("shows the newest content when the diff changes during a read", async () => {
			const user = userEvent.setup();
			const { setRepositories } = renderHarness(["/repo/a.go"]);
			await user.click(screen.getByRole("button", { name: "Open /repo/a.go" }));
			const editor = await screen.findByLabelText("File contents");
			setRepositories(repoWithDiff(""));

			const staleRead = createDeferred<WorkspaceAgentFileContents>();
			vi.mocked(API.readWorkspaceAgentFile).mockReturnValueOnce(
				staleRead.promise,
			);
			setRepositories(repoWithDiff(fileDiff("a.go", "// one")));
			vi.mocked(API.readWorkspaceAgentFile).mockResolvedValueOnce(
				textFile("latest"),
			);
			setRepositories(repoWithDiff(fileDiff("a.go", "// two")));
			await waitFor(() => {
				expect(editor).toHaveValue("latest");
			});

			await act(async () => {
				staleRead.resolve(textFile("stale"));
				await staleRead.promise;
			});
			expect(editor).toHaveValue("latest");
		});

		it("shows the newest content when the file changes during its first read", async () => {
			const user = userEvent.setup();
			const { setRepositories } = renderHarness(["/repo/a.go"]);
			setRepositories(repoWithDiff(""));
			const staleRead = createDeferred<WorkspaceAgentFileContents>();
			vi.mocked(API.readWorkspaceAgentFile).mockReturnValueOnce(
				staleRead.promise,
			);
			await user.click(screen.getByRole("button", { name: "Open /repo/a.go" }));

			vi.mocked(API.readWorkspaceAgentFile).mockResolvedValueOnce(
				textFile("latest"),
			);
			setRepositories(repoWithDiff(fileDiff("a.go", "// one")));
			await act(async () => {
				staleRead.resolve(textFile("stale"));
				await staleRead.promise;
			});

			await waitFor(() => {
				expect(screen.getByLabelText("File contents")).toHaveValue("latest");
			});
		});

		it("rereads a file that a tool call in the chat writes", async () => {
			savePersistedMainTabs(chatId, {
				paths: ["/repo/a.go"],
				activePath: "/repo/a.go",
			});
			// Completed before the page loaded, so the first read covers it.
			const { setStreamState } = renderHarness([], {
				streamState: fileToolStream("call-1", "write_file", {
					path: "/repo/a.go",
				}),
			});
			await screen.findByLabelText("File contents");
			expect(reads("/repo/a.go")).toBe(1);

			setStreamState(
				fileToolStream("call-2", "edit_files", {
					files: [{ path: "a.go", edits: [] }],
				}),
			);
			await waitFor(() => {
				expect(reads("/repo/a.go")).toBe(2);
			});
		});

		it("rereads a file whose tool result arrives only in a durable message", async () => {
			const user = userEvent.setup();
			const { upsertMessages } = renderHarness(["/repo/a.go"]);
			await user.click(screen.getByRole("button", { name: "Open /repo/a.go" }));
			await screen.findByLabelText("File contents");
			// History that loads after mount, and an older page loaded later,
			// precede the reads at mount.
			await upsertMessages(writeFileMessages(10, "call-1", "/repo/a.go"));
			await upsertMessages(writeFileMessages(1, "call-0", "/repo/a.go"));
			await settle();
			expect(reads("/repo/a.go")).toBe(1);

			await upsertMessages(writeFileMessages(20, "call-2", "/repo/a.go"));
			await waitFor(() => {
				expect(reads("/repo/a.go")).toBe(2);
			});
		});

		it("rereads once for a tool result in the stream and then in its message", async () => {
			const user = userEvent.setup();
			const { setStreamState, upsertMessages } = renderHarness(["/repo/a.go"]);
			await user.click(screen.getByRole("button", { name: "Open /repo/a.go" }));
			await screen.findByLabelText("File contents");
			await upsertMessages(writeFileMessages(1, "call-0", "/repo/b.go"));

			setStreamState(
				fileToolStream("call-1", "write_file", { path: "/repo/a.go" }),
			);
			await waitFor(() => {
				expect(reads("/repo/a.go")).toBe(2);
			});
			await upsertMessages(writeFileMessages(3, "call-1", "/repo/a.go"));
			await settle();
			expect(reads("/repo/a.go")).toBe(2);

			await upsertMessages(writeFileMessages(5, "call-2", "/repo/a.go"));
			await waitFor(() => {
				expect(reads("/repo/a.go")).toBe(3);
			});
		});

		it("rereads the active file when the window regains focus", async () => {
			const user = userEvent.setup();
			renderHarness(["/repo/a.go"]);
			await user.click(screen.getByRole("button", { name: "Open /repo/a.go" }));
			await screen.findByLabelText("File contents");

			act(() => {
				focusManager.setFocused(false);
				focusManager.setFocused(true);
			});
			await waitFor(() => {
				expect(reads("/repo/a.go")).toBe(2);
			});
		});
	});

	it("asks before leaving the chat with unsaved edits", async () => {
		const user = userEvent.setup();
		const { router } = renderHarness(["/repo/main.go"]);

		await user.click(
			screen.getByRole("button", { name: "Open /repo/main.go" }),
		);
		await user.type(await screen.findByLabelText("File contents"), "// edit");
		await act(() => router.navigate("/other"));

		const dialog = await screen.findByRole("dialog", {
			name: "Unsaved changes",
		});
		expect(router.state.location.pathname).toBe("/");
		await user.click(within(dialog).getByRole("button", { name: "Leave" }));
		await waitFor(() => {
			expect(router.state.location.pathname).toBe("/other");
		});
	});
});
