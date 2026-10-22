import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useEffect, useRef } from "react";
import { QueryClientProvider } from "react-query";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { WorkspaceFilesContext } from "../../context/WorkspaceFilesContext";
import { getPersistedMainTabs } from "../../utils/mainTabStorage";
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

const Harness: React.FC<{ paths: readonly string[] }> = ({ paths }) => {
	const mainTabs = useMainTabs(chatId);
	const chatInputRef = useRef<HTMLTextAreaElement>(null);
	return (
		<WorkspaceFilesContext
			value={{
				agentId: "agent-1",
				defaultDirectory: "/home/coder",
				repoRoots: ["/repo"],
				repositories: new Map(),
				openFile: mainTabs.openFile,
			}}
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
				readOnly={false}
				focusChatInput={() => chatInputRef.current?.focus()}
			>
				<ChatStub ref={chatInputRef} />
			</MainTabs>
		</WorkspaceFilesContext>
	);
};

const renderHarness = (paths: readonly string[] = []) => {
	const queryClient = createTestQueryClient();
	const router = createMemoryRouter([
		{ path: "/", element: <Harness paths={paths} /> },
		{ path: "/other", element: <p>Other page</p> },
	]);
	const result = render(
		<QueryClientProvider client={queryClient}>
			<RouterProvider router={router} />
		</QueryClientProvider>,
	);
	return { ...result, router };
};

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
