import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useEffect } from "react";
import { QueryClientProvider } from "react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import { WorkspaceFilesContext } from "../../context/WorkspaceFilesContext";
import { getPersistedMainTabs } from "../../utils/mainTabStorage";
import type FileEditor from "../FileTab/FileEditor";
import { MainTabs } from "./MainTabs";
import { useMainTabs } from "./useMainTabs";

vi.mock("../FileTab/FileEditor", () => ({
	default: ({ value, onChange }: React.ComponentProps<typeof FileEditor>) => (
		<textarea
			aria-label="File contents"
			value={value}
			onChange={(event) => onChange(event.target.value)}
		/>
	),
}));

const chatId = "chat-main-tabs";
const chatMounted = vi.fn();

const ChatStub: React.FC = () => {
	useEffect(() => {
		chatMounted();
	}, []);
	return <textarea aria-label="Message" />;
};

const Harness: React.FC<{ paths: readonly string[] }> = ({ paths }) => {
	const mainTabs = useMainTabs(chatId);
	return (
		<WorkspaceFilesContext
			value={{
				agentId: "agent-1",
				defaultDirectory: "/home/coder",
				repoRoots: [],
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
			<MainTabs mainTabs={mainTabs}>
				<ChatStub />
			</MainTabs>
		</WorkspaceFilesContext>
	);
};

const renderHarness = (paths: readonly string[] = []) => {
	const queryClient = createTestQueryClient();
	return render(<Harness paths={paths} />, {
		wrapper: ({ children }) => (
			<QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
		),
	});
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

	it("cycles tabs with Alt+[ and Alt+], also from the editor", async () => {
		const user = userEvent.setup();
		renderHarness(["/repo/a.go", "/repo/b.go"]);

		await user.click(screen.getByRole("button", { name: "Open /repo/a.go" }));
		await user.click(screen.getByRole("button", { name: "Open /repo/b.go" }));

		await user.keyboard("{Alt>}[BracketRight]{/Alt}");
		expect(getTab("Chat")).toHaveAttribute("aria-selected", "true");

		await user.keyboard("{Alt>}[BracketRight]{/Alt}");
		expect(getTab("a.go")).toHaveAttribute("aria-selected", "true");

		const panel = screen.getByRole("tabpanel", { name: "a.go" });
		await user.click(await within(panel).findByLabelText("File contents"));
		await user.keyboard("{Alt>}[BracketLeft]{/Alt}");
		expect(getTab("Chat")).toHaveAttribute("aria-selected", "true");
		expect(within(panel).getByLabelText("File contents")).toHaveValue(
			"package main\n",
		);
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
		expect(getPersistedMainTabs(chatId).paths).toEqual(["/repo/main.go"]);

		await user.click(screen.getByRole("button", { name: "Close main.go" }));
		await user.click(
			within(await screen.findByRole("dialog")).getByRole("button", {
				name: "Discard",
			}),
		);
		await waitFor(() => {
			expect(getPersistedMainTabs(chatId).paths).toEqual([]);
		});
	});
});
