import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AxiosError, AxiosHeaders } from "axios";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
	API,
	type WorkspaceAgentFile,
	type WorkspaceAgentFindFilesResult,
} from "#/api/api";
import { workspaceAgentDirectoryKey } from "#/api/queries/workspaceFiles";
import { toast } from "#/components/Toaster/toast";
import { createDeferred } from "#/testHelpers/deferred";
import { render } from "#/testHelpers/renderHelpers";
import {
	WorkspaceFilesContext,
	type WorkspaceFilesContextValue,
} from "../../context/WorkspaceFilesContext";
import { Response } from "./Response";

// jsdom's stylesheet parser throws on the real FileViewer's shadow-DOM styles.
vi.mock("@pierre/diffs/react", () => ({
	File: ({ file }: { file: { contents: string } }) => (
		<pre>{file.contents}</pre>
	),
}));

const sampleFileCode = `package auth

import "errors"

func ValidateToken(token string) error {
	if token == "" {
		return errors.New("token is empty")
	}
	return nil
}`;

describe("Response", () => {
	it("copies a multi-line fenced code block's raw text, without the fence markers or trailing newline", async () => {
		const user = userEvent.setup();
		const writeText = vi
			.spyOn(navigator.clipboard, "writeText")
			.mockResolvedValue();

		render(<Response>{`\`\`\`go\n${sampleFileCode}\n\`\`\`\n`}</Response>);

		await user.click(await screen.findByRole("button", { name: "Copy code" }));

		expect(writeText).toHaveBeenCalledWith(sampleFileCode);
	});

	describe("file mentions", () => {
		let onScreen = true;
		const observers = new Set<
			(entries: Array<{ isIntersecting: boolean }>) => void
		>();
		const setOnScreen = (value: boolean) => {
			onScreen = value;
			act(() => {
				for (const callback of observers) {
					callback([{ isIntersecting: value }]);
				}
			});
		};
		beforeEach(() => {
			onScreen = true;
			observers.clear();
			vi.stubGlobal(
				"IntersectionObserver",
				class {
					callback: (entries: Array<{ isIntersecting: boolean }>) => void;
					constructor(
						callback: (entries: Array<{ isIntersecting: boolean }>) => void,
					) {
						this.callback = callback;
					}
					observe() {
						observers.add(this.callback);
						if (onScreen) {
							this.callback([{ isIntersecting: true }]);
						}
					}
					disconnect() {
						observers.delete(this.callback);
					}
				},
			);
			vi.spyOn(API, "listWorkspaceAgentDirectory").mockRejectedValue(
				new Error("no listing"),
			);
		});
		afterEach(() => {
			vi.restoreAllMocks();
			vi.unstubAllGlobals();
		});
		const makeFiles = (): WorkspaceFilesContextValue => ({
			agentId: "agent-1",
			defaultDirectory: "/home/coder",
			repoRoots: ["/home/coder/coder", "/home/coder/other"],
			repositories: new Map(),
			openFile: vi.fn(),
		});

		const renderWithFiles = (markdown: string) => {
			const files = makeFiles();
			const { unmount, queryClient } = render(
				<WorkspaceFilesContext.Provider value={files}>
					<Response>{markdown}</Response>
				</WorkspaceFilesContext.Provider>,
			);
			return { files, unmount, queryClient };
		};

		const findResult = (
			root: string,
			path: string,
		): WorkspaceAgentFindFilesResult => ({
			absolute_path: `${root}/${path}`,
			path,
			root,
			is_dir: false,
			score: 1,
		});

		const markdown =
			"Edit `site/src/api/api.ts:42` and /home/coder/coder/go.mod#L3. " +
			"See [main.go](https://example.com/main.go) and v1.2.3.\n\n" +
			"```go\n// cmd/main.go\n```\n";

		const buttonTexts = () =>
			screen.queryAllByRole("button").map((button) => button.textContent);

		it("renders mentions as buttons only when workspace files are available", async () => {
			render(<Response>{markdown}</Response>);
			await screen.findByRole("button", { name: "Copy code" });
			expect(buttonTexts()).toEqual([""]);
		});

		it("marks paths in prose and inline code, not in links or code blocks", async () => {
			renderWithFiles(markdown);
			await screen.findByRole("button", { name: "Copy code" });
			expect(buttonTexts()).toEqual([
				"site/src/api/api.ts:42",
				"/home/coder/coder/go.mod#L3",
				"",
			]);
		});

		it("opens an absolute path without searching", async () => {
			const user = userEvent.setup();
			const find = vi.spyOn(API, "findWorkspaceAgentFiles");
			const { files } = renderWithFiles("Open /home/coder/coder/go.mod#L3.");

			await user.click(
				await screen.findByRole("button", {
					name: "/home/coder/coder/go.mod#L3",
				}),
			);

			expect(files.openFile).toHaveBeenCalledWith("/home/coder/coder/go.mod", {
				line: 3,
			});
			expect(find).not.toHaveBeenCalled();
		});

		it("opens the single file in the chat's repositories that matches", async () => {
			const user = userEvent.setup();
			const find = vi.spyOn(API, "findWorkspaceAgentFiles").mockResolvedValue({
				results: [
					findResult("/home/coder/coder", "site/src/api/api.ts"),
					findResult("/home/coder/coder", "site/src/api/myapi.ts"),
				],
			});
			const { files } = renderWithFiles("Edit `api/api.ts:42`.");

			await user.dblClick(
				await screen.findByRole("button", { name: "api/api.ts:42" }),
			);

			await waitFor(() => expect(files.openFile).toHaveBeenCalledTimes(1));
			expect(files.openFile).toHaveBeenCalledWith(
				"/home/coder/coder/site/src/api/api.ts",
				{ line: 42 },
			);
			expect(find).toHaveBeenCalledTimes(1);
			expect(find).toHaveBeenCalledWith(
				"agent-1",
				["/home/coder/coder", "/home/coder/other"],
				"api/api.ts",
				200,
			);
		});

		it("prefers the file whose repository path equals the mention", async () => {
			const user = userEvent.setup();
			vi.spyOn(API, "findWorkspaceAgentFiles").mockResolvedValue({
				results: [
					findResult("/home/coder/coder", "enterprise/coderd/x/chatd/chatd.go"),
					findResult("/home/coder/coder", "coderd/x/chatd/chatd.go"),
				],
			});
			const { files } = renderWithFiles("See `coderd/x/chatd/chatd.go:603`.");

			await user.click(
				await screen.findByRole("button", {
					name: "coderd/x/chatd/chatd.go:603",
				}),
			);

			await waitFor(() =>
				expect(files.openFile).toHaveBeenCalledWith(
					"/home/coder/coder/coderd/x/chatd/chatd.go",
					{ line: 603 },
				),
			);
		});

		it("lets the user choose between several matching files with the keyboard", async () => {
			const user = userEvent.setup();
			vi.spyOn(API, "findWorkspaceAgentFiles").mockResolvedValue({
				results: [
					findResult("/home/coder/coder", "README.md"),
					findResult("/home/coder/other", "README.md"),
					findResult("/home/coder/other", "docs/README.md"),
				],
			});
			const { files } = renderWithFiles("See README.md for setup.");

			await user.click(
				await screen.findByRole("button", { name: "README.md" }),
			);
			const picker = await screen.findByRole("dialog", {
				name: "Files matching README.md",
			});
			expect(
				within(picker)
					.getAllByRole("option")
					.map((option) => option.getAttribute("aria-label")),
			).toEqual(["README.md in coder", "README.md in other"]);
			await user.keyboard("{ArrowDown}{Enter}");

			expect(files.openFile).toHaveBeenCalledWith(
				"/home/coder/other/README.md",
				{ line: undefined },
			);
		});

		it("closes the picker with Escape and notes truncated results", async () => {
			const user = userEvent.setup();
			vi.spyOn(API, "findWorkspaceAgentFiles").mockResolvedValue({
				results: Array.from({ length: 200 }, (_, index) =>
					findResult("/home/coder/coder", `pkg${index}/README.md`),
				),
			});
			const { files } = renderWithFiles("See README.md for setup.");

			await user.click(
				await screen.findByRole("button", { name: "README.md" }),
			);
			const picker = await screen.findByRole("dialog", {
				name: "Files matching README.md",
			});
			expect(
				within(picker).getByText(/^Showing 200 matches from the first 200/),
			).toBeDefined();
			await user.keyboard("{Escape}");

			await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
			expect(files.openFile).not.toHaveBeenCalled();
		});

		it("reports when no file matches", async () => {
			const user = userEvent.setup();
			vi.spyOn(API, "findWorkspaceAgentFiles").mockResolvedValue({
				results: [findResult("/home/coder/coder", "src/other.go")],
			});
			const toastError = vi.spyOn(toast, "error");
			const { files } = renderWithFiles("Look at `main.go`.");

			await user.click(await screen.findByRole("button", { name: "main.go" }));

			await waitFor(() =>
				expect(toastError).toHaveBeenCalledWith(
					"No file matching main.go in this chat's repositories",
				),
			);
			expect(files.openFile).not.toHaveBeenCalled();
		});

		it("marks messages rendered before workspace files became available", async () => {
			const user = userEvent.setup();
			const files = makeFiles();
			const Harness = () => {
				const [enabled, setEnabled] = useState(false);
				return (
					<>
						<button type="button" onClick={() => setEnabled(true)}>
							Enable files
						</button>
						<WorkspaceFilesContext.Provider value={enabled ? files : undefined}>
							<Response>Open /home/coder/coder/go.mod now.</Response>
						</WorkspaceFilesContext.Provider>
					</>
				);
			};
			render(<Harness />);
			await screen.findByText(/Open/);
			expect(buttonTexts()).toEqual(["Enable files"]);

			await user.click(screen.getByRole("button", { name: "Enable files" }));
			await user.click(
				await screen.findByRole("button", { name: "/home/coder/coder/go.mod" }),
			);

			expect(files.openFile).toHaveBeenCalledWith("/home/coder/coder/go.mod", {
				line: undefined,
			});
		});

		it("ignores a lookup that finishes after the mention unmounts", async () => {
			const user = userEvent.setup();
			let finish: (value: {
				results: WorkspaceAgentFindFilesResult[];
			}) => void = () => {};
			vi.spyOn(API, "findWorkspaceAgentFiles").mockReturnValue(
				new Promise((resolve) => {
					finish = resolve;
				}),
			);
			const toastError = vi.spyOn(toast, "error");
			const { files, unmount } = renderWithFiles("Look at `main.go`.");

			await user.click(await screen.findByRole("button", { name: "main.go" }));
			unmount();
			finish({ results: [findResult("/home/coder/coder", "main.go")] });
			await new Promise((resolve) => setTimeout(resolve, 0));

			expect(files.openFile).not.toHaveBeenCalled();
			expect(toastError).not.toHaveBeenCalled();
		});

		const notFoundResponse = (data: unknown) => ({
			status: 404,
			statusText: "Not Found",
			data,
			headers: {},
			config: { headers: new AxiosHeaders() },
		});

		it.each([
			{ shape: "a Coder error body", data: { message: "Route not found." } },
			{ shape: "a plain text body", data: "404 page not found\n" },
		])("asks for an agent update on a 404 with $shape", async ({ data }) => {
			const user = userEvent.setup();
			vi.spyOn(API, "findWorkspaceAgentFiles").mockRejectedValue(
				new AxiosError(
					"Request failed with status code 404",
					AxiosError.ERR_BAD_REQUEST,
					undefined,
					undefined,
					notFoundResponse(data),
				),
			);
			const toastError = vi.spyOn(toast, "error");
			renderWithFiles("Look at `main.go`.");

			await user.click(await screen.findByRole("button", { name: "main.go" }));

			await waitFor(() =>
				expect(toastError).toHaveBeenCalledWith(
					"Update the workspace agent to open files",
				),
			);
		});

		describe("paths with spaces", () => {
			const flush = () => new Promise((resolve) => setTimeout(resolve, 0));
			// Directories by the path form the walk lists, with "~" standing for
			// /home/coder. Names ending in "/" are directories.
			const listing = (path: string, names: readonly string[]) => {
				const absolute = path.replace(/^~/, "/home/coder");
				const contents = names.map((name): WorkspaceAgentFile => {
					const isDir = name.endsWith("/");
					const entryName = isDir ? name.slice(0, -1) : name;
					return {
						name: entryName,
						absolute_path_string: `${absolute}/${entryName}`,
						is_dir: isDir,
						is_symlink: false,
						size: 0,
						mod_time: "2026-01-01T00:00:00Z",
					};
				});
				return {
					absolute_path: absolute.split("/").filter(Boolean),
					absolute_path_string: absolute,
					contents,
				};
			};
			const mockTree = (tree: Record<string, string[]>) =>
				vi
					.spyOn(API, "listWorkspaceAgentDirectory")
					.mockImplementation(async (_agentId, path) => {
						const names = tree[path];
						if (!names) {
							throw new Error(`no directory ${path}`);
						}
						return listing(path, names);
					});
			const listCalls = (
				list: { mock: { calls: unknown[][] } },
				path: string,
			) => list.mock.calls.filter((call) => call[1] === path);

			type LinkCase = {
				markdown: string;
				tree: Record<string, string[]>;
				link: string;
				opens: string;
				line?: number;
				buttons: string[];
			};

			it.each<LinkCase>([
				{
					markdown: "Open /path/to/file (2).txt and site/src/a.ts.",
					tree: { "/path/to": ["file", "file (2).txt"] },
					link: "/path/to/file (2).txt",
					opens: "/path/to/file (2).txt",
					buttons: ["/path/to/file (2).txt", "site/src/a.ts"],
				},
				{
					markdown: "Edit src/my dir/a.ts:12 please",
					tree: {
						"/home/coder/coder/src": ["my dir/"],
						"/home/coder/coder/src/my dir": ["a.ts"],
					},
					link: "src/my dir/a.ts:12",
					opens: "/home/coder/coder/src/my dir/a.ts",
					line: 12,
					buttons: ["src/my dir/a.ts:12"],
				},
				{
					markdown: "See ~/My Documents/x.md for notes",
					tree: {
						"~": ["My Documents/"],
						"/home/coder/My Documents": ["x.md"],
					},
					link: "~/My Documents/x.md",
					opens: "/home/coder/My Documents/x.md",
					buttons: ["~/My Documents/x.md"],
				},
				{
					markdown: "Open `my dir/a.ts` next",
					tree: {
						"/home/coder/other": ["my dir/"],
						"/home/coder/other/my dir": ["a.ts"],
					},
					link: "my dir/a.ts",
					opens: "/home/coder/other/my dir/a.ts",
					buttons: ["my dir/a.ts"],
				},
			])(
				"links $link once the listings confirm it",
				async ({ markdown, tree, link, opens, line, buttons }) => {
					const user = userEvent.setup();
					mockTree(tree);
					const { files } = renderWithFiles(markdown);

					await user.click(await screen.findByRole("button", { name: link }));

					expect(files.openFile).toHaveBeenCalledWith(opens, { line });
					expect(buttonTexts()).toEqual(buttons);
					expect(document.body).toHaveTextContent(markdown.replaceAll("`", ""));
				},
			);

			it.each([
				{
					markdown: "Open /path/to/missing file.txt and site/src/a.ts.",
					buttons: ["file.txt", "site/src/a.ts"],
				},
				{
					markdown: "Open `/path/to/file (2).txt extra` now",
					buttons: [],
				},
			])(
				"keeps today's mentions in $markdown when no file matches",
				async ({ markdown, buttons }) => {
					const list = mockTree({ "/path/to": ["file (2).txt", "other"] });
					renderWithFiles(markdown);

					await waitFor(() => expect(list).toHaveBeenCalled());
					await flush();
					await screen.findByText(/Open/);
					expect(buttonTexts()).toEqual(buttons);
				},
			);

			const spacedMarkdown = "Open /path/to/file (2).txt now";

			it("lists nothing without workspace files", async () => {
				const list = mockTree({ "/path/to": ["file (2).txt"] });
				render(<Response>{spacedMarkdown}</Response>);
				await screen.findByText(/Open/);
				await flush();

				expect(list).not.toHaveBeenCalled();
				expect(buttonTexts()).toEqual([]);
			});

			it("lists nothing while the mention is off screen", async () => {
				onScreen = false;
				const list = mockTree({ "/path/to": ["file (2).txt"] });
				renderWithFiles(spacedMarkdown);
				await screen.findByText(/Open/);
				await flush();

				expect(list).not.toHaveBeenCalled();
			});

			it("waits for the message to finish streaming", async () => {
				const user = userEvent.setup();
				const list = mockTree({ "/path/to": ["file (2).txt"] });
				const files = makeFiles();
				const Harness = () => {
					const [streaming, setStreaming] = useState(true);
					return (
						<>
							<button type="button" onClick={() => setStreaming(false)}>
								Finish
							</button>
							<WorkspaceFilesContext.Provider value={files}>
								<Response streaming={streaming}>{spacedMarkdown}</Response>
							</WorkspaceFilesContext.Provider>
						</>
					);
				};
				render(<Harness />);
				await screen.findByText(/Open/);
				await flush();
				expect(list).not.toHaveBeenCalled();

				await user.click(screen.getByRole("button", { name: "Finish" }));

				await screen.findByRole("button", { name: "/path/to/file (2).txt" });
				expect(list).toHaveBeenCalledWith(
					"agent-1",
					"/path/to",
					expect.any(AbortSignal),
				);
			});

			it("keeps a mention's link, focus, and selected text when verification does not extend it", async () => {
				const pending = createDeferred<ReturnType<typeof listing>>();
				vi.spyOn(API, "listWorkspaceAgentDirectory").mockReturnValue(
					pending.promise,
				);
				renderWithFiles("Edit /path/to/a.ts and more");

				const link = await screen.findByRole("button", {
					name: "/path/to/a.ts",
				});
				const text = screen.getByText(/and more/).lastChild;
				if (!text) {
					throw new Error("missing text node");
				}
				link.focus();
				const range = document.createRange();
				range.selectNodeContents(text);
				window.getSelection()?.removeAllRanges();
				window.getSelection()?.addRange(range);

				await act(async () => {
					pending.resolve(listing("/path/to", ["a.ts"]));
					await flush();
				});

				expect(link.isConnected).toBe(true);
				expect(document.activeElement).toBe(link);
				expect(text.isConnected).toBe(true);
				expect(window.getSelection()?.toString()).toBe(" and more");
			});

			it("keeps the focused link when verification extends it", async () => {
				const pending = createDeferred<ReturnType<typeof listing>>();
				vi.spyOn(API, "listWorkspaceAgentDirectory").mockReturnValue(
					pending.promise,
				);
				renderWithFiles("Open /path/to/file.md (2).md now");

				const link = await screen.findByRole("button", {
					name: "/path/to/file.md",
				});
				link.focus();
				await act(async () => {
					pending.resolve(listing("/path/to", ["file.md (2).md"]));
					await flush();
				});

				expect(
					await screen.findByRole("button", {
						name: "/path/to/file.md (2).md",
					}),
				).toBe(link);
				expect(document.activeElement).toBe(link);
			});

			it("links every spaced path on a line, absolute and repository-relative", async () => {
				const user = userEvent.setup();
				mockTree({
					"/home/coder/coder/ux-notes": ["my notes/"],
					"/home/coder/coder/ux-notes/my notes": ["notes (2).md"],
				});
				const { files } = renderWithFiles(
					"The notes live in /home/coder/coder/ux-notes/my notes/notes (2).md and in ux-notes/my notes/notes (2).md as well.",
				);

				await user.click(
					await screen.findByRole("button", {
						name: "ux-notes/my notes/notes (2).md",
					}),
				);

				expect(buttonTexts()).toEqual([
					"/home/coder/coder/ux-notes/my notes/notes (2).md",
					"ux-notes/my notes/notes (2).md",
				]);
				expect(files.openFile).toHaveBeenCalledWith(
					"/home/coder/coder/ux-notes/my notes/notes (2).md",
					{ line: undefined },
				);
			});

			it("offers equally long matches from several repositories", async () => {
				const user = userEvent.setup();
				mockTree({
					"/home/coder/coder": ["my dir/"],
					"/home/coder/coder/my dir": ["a.ts"],
					"/home/coder/other": ["my dir/"],
					"/home/coder/other/my dir": ["a.ts"],
				});
				const { files } = renderWithFiles("Open `my dir/a.ts` next");

				await user.click(
					await screen.findByRole("button", { name: "my dir/a.ts" }),
				);
				const picker = await screen.findByRole("dialog", {
					name: "Files matching my dir/a.ts",
				});
				await user.click(
					within(picker).getByRole("option", { name: "my dir/a.ts in other" }),
				);

				expect(files.openFile).toHaveBeenCalledWith(
					"/home/coder/other/my dir/a.ts",
					{ line: undefined },
				);
			});

			it("waits for every repository before linking a spaced path", async () => {
				const user = userEvent.setup();
				const slowRoot = createDeferred<ReturnType<typeof listing>>();
				const tree: Record<string, string[]> = {
					"/home/coder/coder": ["my dir/"],
					"/home/coder/coder/my dir": ["a.ts"],
					"/home/coder/other/my dir": ["a.ts"],
				};
				vi.spyOn(API, "listWorkspaceAgentDirectory").mockImplementation(
					async (_agentId, path) =>
						path === "/home/coder/other"
							? slowRoot.promise
							: listing(path, tree[path] ?? []),
				);
				const { files } = renderWithFiles("Open `my dir/a.ts` next");
				await waitFor(() =>
					expect(API.listWorkspaceAgentDirectory).toHaveBeenCalledWith(
						"agent-1",
						"/home/coder/coder/my dir",
						expect.any(AbortSignal),
					),
				);
				await flush();
				expect(buttonTexts()).toEqual([]);

				await act(async () => {
					slowRoot.resolve(listing("/home/coder/other", ["my dir/"]));
					await flush();
				});
				await user.click(
					await screen.findByRole("button", { name: "my dir/a.ts" }),
				);

				expect(
					await screen.findByRole("dialog", {
						name: "Files matching my dir/a.ts",
					}),
				).toBeDefined();
				expect(files.openFile).not.toHaveBeenCalled();
			});

			it("does not verify again for a new agent while off screen", async () => {
				const user = userEvent.setup();
				const list = mockTree({ "/path/to": ["file (2).txt"] });
				const Harness = () => {
					const [agentId, setAgentId] = useState("agent-1");
					return (
						<>
							<button type="button" onClick={() => setAgentId("agent-2")}>
								Switch agent
							</button>
							<WorkspaceFilesContext.Provider
								value={{ ...makeFiles(), agentId }}
							>
								<Response>{spacedMarkdown}</Response>
							</WorkspaceFilesContext.Provider>
						</>
					);
				};
				render(<Harness />);
				await screen.findByRole("button", { name: "/path/to/file (2).txt" });

				setOnScreen(false);
				await user.click(screen.getByRole("button", { name: "Switch agent" }));
				await flush();

				expect(list.mock.calls.map((call) => call[0])).toEqual(["agent-1"]);
				setOnScreen(true);
				await waitFor(() =>
					expect(list).toHaveBeenCalledWith(
						"agent-2",
						"/path/to",
						expect.any(AbortSignal),
					),
				);
			});

			it("stops walking when workspace files go away", async () => {
				const user = userEvent.setup();
				const pending = createDeferred<ReturnType<typeof listing>>();
				const list = vi
					.spyOn(API, "listWorkspaceAgentDirectory")
					.mockImplementation((_agentId, path) =>
						path === "/home/coder"
							? pending.promise
							: Promise.resolve(listing(path, ["a.ts"])),
					);
				const files = makeFiles();
				const Harness = () => {
					const [enabled, setEnabled] = useState(true);
					return (
						<>
							<button type="button" onClick={() => setEnabled(false)}>
								Disable files
							</button>
							<WorkspaceFilesContext.Provider
								value={enabled ? files : undefined}
							>
								<Response>See /home/coder/my dir/a.ts now</Response>
							</WorkspaceFilesContext.Provider>
						</>
					);
				};
				render(<Harness />);
				await waitFor(() =>
					expect(list).toHaveBeenCalledWith(
						"agent-1",
						"/home/coder",
						expect.any(AbortSignal),
					),
				);

				await user.click(screen.getByRole("button", { name: "Disable files" }));
				await act(async () => {
					pending.resolve(listing("/home/coder", ["my dir/"]));
					await flush();
				});

				expect(listCalls(list, "/home/coder/my dir")).toEqual([]);
			});

			it("remembers a missing directory across candidates", async () => {
				const user = userEvent.setup();
				const list = mockTree({});
				const Harness = () => {
					const [count, setCount] = useState(1);
					const markdown = Array.from(
						{ length: count },
						(_, index) => `Use /r/and/or option ${index}.`,
					).join("\n\n");
					return (
						<>
							<button type="button" onClick={() => setCount(5)}>
								More
							</button>
							<WorkspaceFilesContext.Provider value={makeFiles()}>
								<Response>{markdown}</Response>
							</WorkspaceFilesContext.Provider>
						</>
					);
				};
				render(<Harness />);
				await waitFor(() => expect(listCalls(list, "/r/and")).toHaveLength(1));
				await flush();

				await user.click(screen.getByRole("button", { name: "More" }));
				await screen.findByText(/option 4/);
				await flush();

				expect(listCalls(list, "/r/and")).toHaveLength(1);
			});

			it("follows a refreshed directory listing", async () => {
				mockTree({ "/r": ["my dir/"], "/r/my dir": ["old.ts"] });
				const { queryClient } = renderWithFiles("Edit /r/my dir/new.ts now");
				await waitFor(() =>
					expect(
						queryClient.getQueryData(
							workspaceAgentDirectoryKey("agent-1", "/r/my dir"),
						),
					).toBeDefined(),
				);
				expect(buttonTexts()).toEqual(["dir/new.ts"]);

				act(() => {
					queryClient.setQueryData(
						workspaceAgentDirectoryKey("agent-1", "/r/my dir"),
						listing("/r/my dir", ["old.ts", "new.ts"]),
					);
				});

				expect(
					await screen.findByRole("button", { name: "/r/my dir/new.ts" }),
				).toBeDefined();
			});
		});
	});
});
