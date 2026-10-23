import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
	API,
	type WorkspaceAgentFile,
	type WorkspaceAgentFindFilesResult,
} from "#/api/api";
import { workspaceAgentDirectoryKey } from "#/api/queries/workspaceFiles";
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
			vi.spyOn(API, "findWorkspaceAgentFiles").mockResolvedValue({
				results: [],
			});
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
			revealInFiles: vi.fn(),
			diffStatuses: [],
		});

		const renderWithFiles = (
			markdown: string,
			overrides: Partial<WorkspaceFilesContextValue> = {},
		) => {
			const files = { ...makeFiles(), ...overrides };
			const { unmount, queryClient } = render(
				<WorkspaceFilesContext.Provider value={files}>
					<Response>{markdown}</Response>
				</WorkspaceFilesContext.Provider>,
			);
			return { files, unmount, queryClient };
		};

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
					absolute_path_string: `${absolute === "/" ? "" : absolute}/${entryName}`,
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
		// Adds each absolute directory's ancestors, which a walk from "/"
		// lists on its way down.
		const withAncestors = (tree: Record<string, string[]>) => {
			const full: Record<string, string[]> = {};
			const add = (directory: string, names: readonly string[]) => {
				full[directory] = [...new Set([...(full[directory] ?? []), ...names])];
			};
			for (const [path, names] of Object.entries(tree)) {
				add(path, names);
				let child = path;
				while (child.startsWith("/") && child !== "/") {
					const slash = child.lastIndexOf("/");
					const parent = slash === 0 ? "/" : child.slice(0, slash);
					add(parent, [`${child.slice(slash + 1)}/`]);
					child = parent;
				}
			}
			return full;
		};
		const mockTree = (tree: Record<string, string[]>) => {
			const full = withAncestors(tree);
			return vi
				.spyOn(API, "listWorkspaceAgentDirectory")
				.mockImplementation(async (_agentId, path) => {
					const names = full[path];
					if (!names) {
						throw new Error(`no directory ${path}`);
					}
					return listing(path, names);
				});
		};
		const listCalls = (list: { mock: { calls: unknown[][] } }, path: string) =>
			list.mock.calls.filter((call) => call[1] === path);

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
		// Search results by the bare name searched for.
		const mockFind = (
			results: Record<string, WorkspaceAgentFindFilesResult[]>,
		) =>
			vi
				.spyOn(API, "findWorkspaceAgentFiles")
				.mockImplementation(async (_agentId, _roots, query) => ({
					results: results[query] ?? [],
				}));

		const markdown =
			"Edit `site/src/api/api.ts:42` and /home/coder/coder/go.mod#L3. " +
			"See [main.go](https://example.com/main.go) and v1.2.3.\n\n" +
			"```go\n// cmd/main.go\n```\n";
		const markdownTree = {
			"/home/coder/coder": ["go.mod", "site/"],
			"/home/coder/coder/site/src/api": ["api.ts"],
		};

		const buttonTexts = () =>
			screen.queryAllByRole("button").map((button) => button.textContent);

		it("renders mentions as buttons only when workspace files are available", async () => {
			render(<Response>{markdown}</Response>);
			await screen.findByRole("button", { name: "Copy code" });
			expect(buttonTexts()).toEqual([""]);
		});

		it("links paths in prose and inline code, not in links or code blocks", async () => {
			mockTree(markdownTree);
			renderWithFiles(markdown);
			await screen.findByRole("button", {
				name: "/home/coder/coder/go.mod#L3",
			});
			expect(buttonTexts()).toEqual([
				"site/src/api/api.ts:42",
				"/home/coder/coder/go.mod#L3",
				"",
			]);
		});

		it.each([
			{
				markdown: "Open /home/coder/coder/go.mod#L3.",
				link: "/home/coder/coder/go.mod#L3",
				opens: "/home/coder/coder/go.mod",
				line: 3,
			},
			{
				markdown: "Edit `site/src/api/api.ts:42`.",
				link: "site/src/api/api.ts:42",
				opens: "/home/coder/coder/site/src/api/api.ts",
				line: 42,
			},
			{
				markdown: "Ignore it in .gitignore.",
				link: ".gitignore",
				opens: "/home/coder/coder/.gitignore",
				line: undefined,
			},
			{
				markdown: "The walk lives in FileMention.tsx:120.",
				link: "FileMention.tsx:120",
				opens:
					"/home/coder/coder/site/src/pages/AgentsPage/components/ChatElements/FileMention.tsx",
				line: 120,
			},
		])(
			"opens $link once a lookup confirms it",
			async ({ markdown, link, opens, line }) => {
				const user = userEvent.setup();
				mockTree(markdownTree);
				mockFind({
					".gitignore": [findResult("/home/coder/coder", ".gitignore")],
					"FileMention.tsx": [
						findResult(
							"/home/coder/coder",
							"site/src/pages/AgentsPage/components/ChatElements/FileMention.tsx",
						),
						findResult("/home/coder/coder", "site/src/MyFileMention.tsx"),
					],
				});
				const { files } = renderWithFiles(markdown);

				await user.click(await screen.findByRole("button", { name: link }));

				expect(files.openFile).toHaveBeenCalledWith(opens, { line });
			},
		);

		it("leaves mentions that no lookup confirms as plain text", async () => {
			const list = mockTree({
				"/home/coder": ["conn.go"],
				"/home/coder/coder": ["main.go"],
				"/home/coder/demo": [".gitignore"],
			});
			const find = mockFind({
				"main.go": [findResult("/home/coder/coder", "main.go")],
			});
			renderWithFiles(
				[
					"See ../conn.go and `/home/coder/demo/pyproject.toml`.",
					"Also main.go, /home/coder/demo/.gitignore and src/missing.ts.",
					"Nothing defines missing.go.",
				].join("\n\n"),
			);

			await screen.findByRole("button", {
				name: "/home/coder/demo/.gitignore",
			});
			await waitFor(() =>
				expect(listCalls(list, "/home/coder/demo")).toHaveLength(1),
			);
			await flush();
			await waitFor(() =>
				expect(find).toHaveBeenCalledWith(
					"agent-1",
					["/home/coder/coder", "/home/coder/other"],
					"missing.go",
					200,
				),
			);
			await flush();
			expect(buttonTexts()).toEqual(["main.go", "/home/coder/demo/.gitignore"]);
		});

		it("searches more than eight repositories in batches the agent accepts", async () => {
			const user = userEvent.setup();
			const repoRoots = Array.from(
				{ length: 9 },
				(_, index) => `/home/coder/repo${index}`,
			);
			const find = vi
				.spyOn(API, "findWorkspaceAgentFiles")
				.mockImplementation(async (_agentId, roots) => {
					if (roots.length > 8) {
						throw new Error("at most 8 roots");
					}
					return {
						results: roots.includes("/home/coder/repo8")
							? [findResult("/home/coder/repo8", "pkg/conn.go")]
							: [],
					};
				});
			const { files } = renderWithFiles("See conn.go.", { repoRoots });

			await user.click(await screen.findByRole("button", { name: "conn.go" }));

			expect(files.openFile).toHaveBeenCalledWith(
				"/home/coder/repo8/pkg/conn.go",
				{ line: undefined },
			);
			expect(find.mock.calls.map((call) => call[1])).toEqual([
				repoRoots.slice(0, 8),
				repoRoots.slice(8),
			]);
		});

		it("remembers a failed search as no match", async () => {
			const user = userEvent.setup();
			const find = vi
				.spyOn(API, "findWorkspaceAgentFiles")
				.mockRejectedValue(new Error("agent too old"));
			const Harness = () => {
				const [count, setCount] = useState(1);
				return (
					<>
						<button type="button" onClick={() => setCount(5)}>
							More
						</button>
						<WorkspaceFilesContext.Provider value={makeFiles()}>
							{Array.from({ length: count }, (_, index) => (
								<Response key={index}>{`See conn.go, part ${index}.`}</Response>
							))}
						</WorkspaceFilesContext.Provider>
					</>
				);
			};
			render(<Harness />);
			await waitFor(() => expect(find).toHaveBeenCalledTimes(1));
			await flush();

			await user.click(screen.getByRole("button", { name: "More" }));
			await screen.findByText(/part 4/);
			await flush();

			expect(find).toHaveBeenCalledTimes(1);
			expect(buttonTexts()).toEqual(["More"]);
		});

		it("looks up a bare name once for all its mentions", async () => {
			const find = mockFind({
				"conn.go": [findResult("/home/coder/coder", "wsjson/conn.go")],
			});
			renderWithFiles(
				"See conn.go, then `conn.go:12`.\n\nAgain: conn.go and conn.go.",
			);

			await waitFor(() =>
				expect(buttonTexts()).toEqual([
					"conn.go",
					"conn.go:12",
					"conn.go",
					"conn.go",
				]),
			);
			expect(find).toHaveBeenCalledTimes(1);
		});

		it("lets the user choose between files in several repositories with the keyboard", async () => {
			const user = userEvent.setup();
			mockFind({
				"README.md": [
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
			mockFind({
				"README.md": Array.from({ length: 200 }, (_, index) =>
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

		it("prefers the chat's repositories over the default directory", async () => {
			const user = userEvent.setup();
			mockTree({
				"/home/coder/docs": ["README.md"],
				"/home/coder/coder/docs": ["README.md"],
			});
			const { files } = renderWithFiles("See docs/README.md for setup.");

			await user.click(
				await screen.findByRole("button", { name: "docs/README.md" }),
			);

			expect(files.openFile).toHaveBeenCalledWith(
				"/home/coder/coder/docs/README.md",
				{ line: undefined },
			);
		});

		it("marks messages rendered before workspace files became available", async () => {
			const user = userEvent.setup();
			mockTree(markdownTree);
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

		it("links a mention only after the message finishes streaming", async () => {
			const user = userEvent.setup();
			const list = mockTree(markdownTree);
			const files = makeFiles();
			const Harness = () => {
				const [streaming, setStreaming] = useState(true);
				return (
					<>
						<button type="button" onClick={() => setStreaming(false)}>
							Finish
						</button>
						<WorkspaceFilesContext.Provider value={files}>
							<Response streaming={streaming}>
								Open /home/coder/coder/go.mod now.
							</Response>
						</WorkspaceFilesContext.Provider>
					</>
				);
			};
			render(<Harness />);
			await screen.findByText(/Open/);
			await flush();
			expect(buttonTexts()).toEqual(["Finish"]);
			expect(list).not.toHaveBeenCalled();

			await user.click(screen.getByRole("button", { name: "Finish" }));

			expect(
				await screen.findByRole("button", { name: "/home/coder/coder/go.mod" }),
			).toBeDefined();
		});

		describe("paths with spaces", () => {
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
					tree: {
						"/path/to": ["file", "file (2).txt"],
						"/home/coder/coder/site/src": ["a.ts"],
					},
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

			it.each<{
				markdown: string;
				tree: Record<string, string[]>;
				link: string;
				reveals: string;
			}>([
				{
					markdown: "Routes live in site/src/pages/ today",
					tree: { "/home/coder/coder/site/src": ["pages/", "pages.ts"] },
					link: "site/src/pages/",
					reveals: "/home/coder/coder/site/src/pages",
				},
				{
					markdown: "Look in /home/coder/my docs for notes",
					tree: { "/home/coder": ["my docs/"] },
					link: "/home/coder/my docs",
					reveals: "/home/coder/my docs",
				},
				{
					markdown: "Open `~/projects/` next",
					tree: { "~": ["projects/"] },
					link: "~/projects/",
					reveals: "/home/coder/projects",
				},
				{
					markdown: "Look in /etc/nginx/conf.d today",
					tree: { "/etc/nginx": ["conf.d/"] },
					link: "/etc/nginx/conf.d",
					reveals: "/etc/nginx/conf.d",
				},
			])(
				"shows the folder $link in Files once the listings confirm it",
				async ({ markdown, tree, link, reveals }) => {
					const user = userEvent.setup();
					mockTree(tree);
					const { files } = renderWithFiles(markdown);

					const button = await screen.findByRole("button", {
						name: `Show ${reveals} in Files`,
					});
					expect(button).toHaveTextContent(link);
					await user.click(button);

					expect(files.revealInFiles).toHaveBeenCalledWith(reveals, {
						isDirectory: true,
					});
					expect(files.openFile).not.toHaveBeenCalled();
					expect(buttonTexts()).toEqual([link]);
				},
			);

			it.each([
				{
					markdown: "Open /path/to/missing file.txt and site/src/a.ts.",
					buttons: [],
				},
				{
					markdown: "Open `/path/to/file (2).txt extra` now",
					buttons: [],
				},
				{
					markdown: "Open /path/to/other/ and /path/to/gone/ now",
					buttons: [],
				},
			])(
				"keeps $markdown plain when no file matches",
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

			it.each([
				'Look in "/repo/my docs/missing/" now',
				'Look in "/repo/my docs" now',
				"Look in /repo/my docs/missing/ now",
			])("does not link the existing prefix of %j", async (markdown) => {
				const list = mockTree({ "/repo": ["my/"] });
				renderWithFiles(markdown);

				await waitFor(() => expect(list).toHaveBeenCalled());
				await flush();
				await screen.findByText(/Look in/);
				expect(buttonTexts()).toEqual([]);
			});

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

			it("keeps the surrounding text and selection when a mention links", async () => {
				const pending = createDeferred<ReturnType<typeof listing>>();
				const ancestors = withAncestors({ "/path/to": [] });
				vi.spyOn(API, "listWorkspaceAgentDirectory").mockImplementation(
					async (_agentId, path) =>
						path === "/path/to"
							? pending.promise
							: listing(path, ancestors[path] ?? []),
				);
				renderWithFiles("Edit /path/to/a.ts and more");

				const text = (await screen.findByText(/and more/)).lastChild;
				if (!text) {
					throw new Error("missing text node");
				}
				const range = document.createRange();
				range.selectNodeContents(text);
				window.getSelection()?.removeAllRanges();
				window.getSelection()?.addRange(range);

				await act(async () => {
					pending.resolve(listing("/path/to", ["a.ts"]));
					await flush();
				});

				expect(
					await screen.findByRole("button", { name: "/path/to/a.ts" }),
				).toBeDefined();
				expect(text.isConnected).toBe(true);
				expect(window.getSelection()?.toString()).toBe(" and more");
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

			it("does not read the words of a rejected path as a path of their own", async () => {
				const list = mockTree({
					"/home/coder/coder/documents": ["notes (2).md"],
				});
				renderWithFiles("See /missing/my documents/notes (2).md now.");
				await waitFor(() => expect(listCalls(list, "/")).toHaveLength(1));
				await flush();

				expect(buttonTexts()).toEqual([]);
				expect(listCalls(list, "/missing")).toEqual([]);
				expect(listCalls(list, "/home/coder/coder/documents")).toEqual([]);
			});

			it("links a later independent spaced path after a rejected one", async () => {
				mockTree({
					"/home/coder/coder/ux-notes": ["my notes/"],
					"/home/coder/coder/ux-notes/my notes": ["notes (2).md"],
				});
				renderWithFiles(
					"See /missing/my notes.md and ux-notes/my notes/notes (2).md now.",
				);

				expect(
					await screen.findByRole("button", {
						name: "ux-notes/my notes/notes (2).md",
					}),
				).toBeDefined();
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

				expect([...new Set(list.mock.calls.map((call) => call[0]))]).toEqual([
					"agent-1",
				]);
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
				const tree = withAncestors({ "/home/coder/my dir": ["a.ts"] });
				const list = vi
					.spyOn(API, "listWorkspaceAgentDirectory")
					.mockImplementation((_agentId, path) =>
						path === "/home/coder"
							? pending.promise
							: Promise.resolve(listing(path, tree[path] ?? [])),
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

			it("lists only directories that a parent listing shows", async () => {
				const tree = {
					"/home/coder/coder": [],
					"/home/coder/other": [],
					"/home/coder/demo": [],
				};
				const list = mockTree(tree);
				renderWithFiles(
					[
						"Look in docs/my notes.md for it.",
						"Run gobwas/ws x.go now.",
						"See /home/coder/websocket/gobwas and more.",
						"See /home/coder/demo/src for it.",
					].join("\n\n"),
				);
				await waitFor(() =>
					expect(listCalls(list, "/home/coder/demo")).toHaveLength(1),
				);
				await flush();
				await flush();

				const existing = Object.keys(withAncestors(tree));
				const listed = list.mock.calls.map((call) => call[1]);
				expect(listed.filter((path) => !existing.includes(path))).toEqual([]);
			});

			it("lists a shared directory once across candidates", async () => {
				const user = userEvent.setup();
				const list = mockTree({ "/r": [] });
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
				await waitFor(() => expect(listCalls(list, "/r")).toHaveLength(1));
				await flush();

				await user.click(screen.getByRole("button", { name: "More" }));
				await screen.findByText(/option 4/);
				await flush();

				expect(listCalls(list, "/r")).toHaveLength(1);
				expect(listCalls(list, "/r/and")).toEqual([]);
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
				await flush();
				expect(buttonTexts()).toEqual([]);

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
