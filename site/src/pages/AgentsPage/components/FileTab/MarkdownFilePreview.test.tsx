import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useEffect, useState } from "react";
import { QueryClientProvider } from "react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import {
	createTestQueryClient,
	renderComponent,
} from "#/testHelpers/renderHelpers";
import {
	WorkspaceFilesContext,
	type WorkspaceFilesContextValue,
} from "../../context/WorkspaceFilesContext";
import type FileEditor from "./FileEditor";
import { type FileDraft, FileTab } from "./FileTab";
import { MarkdownFilePreview } from "./MarkdownFilePreview";

// jsdom's stylesheet parser throws on the real FileViewer's shadow-DOM styles.
const { fileViewer } = vi.hoisted(() => ({
	fileViewer: vi.fn(({ file }: { file: { contents: string } }) => (
		<pre>{file.contents}</pre>
	)),
}));
vi.mock("@pierre/diffs/react", () => ({ File: fileViewer }));

vi.mock("./FileEditor", () => ({
	default: function MockFileEditor({
		value,
		onStatusChange,
	}: React.ComponentProps<typeof FileEditor>) {
		useEffect(() => {
			onStatusChange?.({
				language: "Markdown",
				line: 3,
				column: 1,
				selectedCharacters: 0,
			});
		}, [onStatusChange]);
		return <textarea aria-label="File contents" value={value} readOnly />;
	},
}));

const agentId = "agent-1";
const path = "/repo/README.md";

const makeFiles = (): WorkspaceFilesContextValue => ({
	agentId,
	defaultDirectory: "/home/coder",
	repoRoots: ["/repo"],
	repoRootsSettled: true,
	repositories: new Map(),
	openFile: vi.fn(),
	revealInFiles: vi.fn(),
	diffStatuses: [],
});

const renderWithProviders = (element: React.ReactNode) => {
	const files = makeFiles();
	const queryClient = createTestQueryClient();
	const result = renderComponent(
		<QueryClientProvider client={queryClient}>
			<WorkspaceFilesContext.Provider value={files}>
				{element}
			</WorkspaceFilesContext.Provider>
		</QueryClientProvider>,
	);
	return { ...result, files };
};

const renderPreview = (content: string) =>
	renderWithProviders(
		<MarkdownFilePreview agentId={agentId} path={path} content={content} />,
	);

// About one preview chunk of prose, so content around it lands in other chunks.
const filler = () => "Some filler text. ".repeat(1000);

const pngContents = () => ({
	data: new Uint8Array([137, 80, 78, 71]).buffer,
	contentType: "image/png",
	etag: '"v1"',
});

describe("MarkdownFilePreview", () => {
	beforeEach(() => {
		fileViewer.mockClear();
		vi.spyOn(URL, "createObjectURL").mockImplementation(
			() => `blob:preview-${Math.random()}`,
		);
		vi.spyOn(URL, "revokeObjectURL").mockImplementation(() => {});
	});

	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("renders allowed raw HTML and drops comments, scripts, and styles", async () => {
		const { container } = renderPreview(
			[
				"<!-- markdownlint-disable MD041 -->",
				'<div align="center">',
				'<p onclick="alert(1)">Centered <kbd>Ctrl</kbd> H<sub>2</sub>O</p>',
				"<details><summary>More</summary>Hidden text</details>",
				"<script>alert('script')</script>",
				"<style>p { color: red }</style>",
				'<iframe src="https://example.com"></iframe>',
				"</div>",
			].join("\n"),
		);

		await screen.findByText("Ctrl");
		expect(container.textContent).not.toMatch(
			/markdownlint|<div|alert|color: red/,
		);
		expect(screen.getByText("More").tagName).toBe("SUMMARY");
		expect(screen.getByText(/Centered/).getAttribute("onclick")).toBeNull();
	});

	it("opens relative links as file tabs and external links in a new tab", async () => {
		const user = userEvent.setup();
		const { files } = renderPreview(
			"[License](./LICENSE) [Guide](docs/guide.md#L4) [Docs](https://coder.com/docs)",
		);

		await user.click(await screen.findByRole("button", { name: "License" }));
		await user.click(screen.getByRole("button", { name: "Guide" }));

		expect(files.openFile).toHaveBeenNthCalledWith(1, "/repo/LICENSE", {
			line: undefined,
		});
		expect(files.openFile).toHaveBeenNthCalledWith(2, "/repo/docs/guide.md", {
			line: 4,
		});
		expect(
			screen.getByRole("link", { name: "Docs" }).getAttribute("target"),
		).toBe("_blank");
	});

	it("blocks remote images until the viewer loads them", async () => {
		const user = userEvent.setup();
		const { files } = renderPreview(
			"[![license](https://img.shields.io/github/license/coder/coder)](./LICENSE)",
		);

		await user.click(
			await screen.findByRole("button", { name: "license (image blocked)" }),
		);
		expect(files.openFile).toHaveBeenCalledWith("/repo/LICENSE", {
			line: undefined,
		});
		expect(screen.queryByRole("img")).toBeNull();

		await user.click(
			screen.getByRole("button", { name: "Load remote images" }),
		);

		expect(
			(await screen.findByRole("img", { name: "license" })).getAttribute("src"),
		).toBe("https://img.shields.io/github/license/coder/coder");
		expect(document.activeElement).toBe(
			screen.getByRole("region", { name: "Preview of README.md" }),
		);
	});

	it("scrolls to a heading when an anchor link is clicked", async () => {
		const user = userEvent.setup();
		const scrolled: string[] = [];
		vi.spyOn(HTMLElement.prototype, "scrollIntoView").mockImplementation(
			function (this: HTMLElement) {
				scrolled.push(this.textContent ?? "");
			},
		);
		renderPreview("[Jump](#getting-started)\n\n## Getting Started!\n");

		await user.click(await screen.findByRole("link", { name: "Jump" }));

		expect(scrolled).toEqual(["Getting Started!"]);
	});

	it("loads workspace images with a bounded read and revokes them on unmount", async () => {
		const readFile = vi
			.spyOn(API, "readWorkspaceAgentFile")
			.mockImplementation(async (_agentId, path) =>
				path.endsWith("huge.png")
					? {
							...pngContents(),
							data: new ArrayBuffer(2 * 1024 * 1024 + 1),
						}
					: pngContents(),
			);
		const { unmount } = renderPreview(
			'![Logo](./docs/logo.png) <img src="docs/huge.png" alt="Huge">',
		);

		const logo = await screen.findByRole("img", { name: "Logo" });
		expect(logo.getAttribute("src")).toMatch(/^blob:preview-/);
		await screen.findByText("Huge (image too large to preview)");
		expect(readFile).toHaveBeenCalledWith(agentId, "/repo/docs/huge.png", {
			limit: 2 * 1024 * 1024 + 1,
		});
		expect(URL.createObjectURL).toHaveBeenCalledTimes(1);

		unmount();
		expect(URL.revokeObjectURL).toHaveBeenCalledWith(logo.getAttribute("src"));
	});

	it.each([
		{ layout: "in one chunk", separator: " " },
		{ layout: "spread across chunks", separator: `\n\n${filler()}` },
	])("loads at most 20 workspace images $layout", async ({ separator }) => {
		const readFile = vi
			.spyOn(API, "readWorkspaceAgentFile")
			.mockResolvedValue(pngContents());
		renderPreview(
			Array.from(
				{ length: 25 },
				(_, index) => `![${index}](docs/${index}.png)`,
			).join(separator),
		);

		await screen.findByText("24 (image not loaded)", {}, { timeout: 5000 });
		await waitFor(() => expect(readFile).toHaveBeenCalledTimes(20));
	});

	it("fills in a large file a chunk at a time with references resolved across chunks", async () => {
		const user = userEvent.setup();
		const { files } = renderPreview(
			[
				"[guide]: ./guide.md",
				"# Part 1",
				filler(),
				"# Part 2",
				filler(),
				"# Part 3",
				"Read the [Guide][guide].",
			].join("\n\n"),
		);

		expect(
			(await screen.findByRole("status")).textContent?.includes(
				"Rendering preview",
			),
		).toBe(true);
		await user.click(await screen.findByRole("button", { name: "Guide" }));

		expect(files.openFile).toHaveBeenCalledWith("/repo/guide.md", {
			line: undefined,
		});
		expect(screen.queryByRole("status")).toBeNull();
	});

	it("resolves references across chunks the way the whole file does", async () => {
		const user = userEvent.setup();
		const { files } = renderPreview(
			[
				"Read the [Guide][guide].",
				filler(),
				"```\n[guide]: ./wrong.md\n```",
				filler(),
				"[guide]:\n  ./correct.md",
			].join("\n\n"),
		);

		await user.click(await screen.findByRole("button", { name: "Guide" }));

		expect(files.openFile).toHaveBeenCalledWith("/repo/correct.md", {
			line: undefined,
		});
	});

	it("numbers repeated headings across the whole file", async () => {
		const user = userEvent.setup();
		const scrolled: string[] = [];
		vi.spyOn(HTMLElement.prototype, "scrollIntoView").mockImplementation(
			function (this: HTMLElement) {
				scrolled.push(this.id);
			},
		);
		renderPreview(
			[
				"[Second example](#example-1)",
				"## Example",
				filler(),
				"## Example",
				filler(),
			].join("\n\n"),
		);
		await waitFor(() => expect(screen.queryByRole("status")).toBeNull());

		expect(
			screen
				.getAllByRole("heading", { name: "Example" })
				.map((heading) => heading.id),
		).toEqual(["user-content-example", "user-content-example-1"]);
		await user.click(screen.getByRole("link", { name: "Second example" }));
		expect(scrolled).toEqual(["user-content-example-1"]);
	});

	it("keeps rendered chunks when the file is reread", async () => {
		const user = userEvent.setup();
		const base = ["# Part 1", filler(), "# Part 2", filler(), "# Part 3"].join(
			"\n\n",
		);
		const Harness = () => {
			const [content, setContent] = useState(base);
			return (
				<>
					<button
						type="button"
						onClick={() => setContent(`${content}\n\nAppended line.`)}
					>
						Append
					</button>
					<MarkdownFilePreview
						agentId={agentId}
						path={path}
						content={content}
					/>
				</>
			);
		};
		renderWithProviders(<Harness />);
		await waitFor(() => expect(screen.queryByRole("status")).toBeNull());
		const first = screen.getByRole("heading", { name: "Part 1" });

		await user.click(screen.getByRole("button", { name: "Append" }));

		expect(screen.getByRole("heading", { name: "Part 1" })).toBe(first);
		expect(screen.getByRole("heading", { name: "Part 3" })).toBeDefined();
		expect(await screen.findByText("Appended line.")).toBeDefined();
	});

	it("shows code in large files as plain text", async () => {
		const code = "func main() {}";
		renderPreview(
			`${"Padding text. ".repeat(20_000)}\n\n\`\`\`go\n${code}\n\`\`\`\n`,
		);

		await screen.findByText(code, {}, { timeout: 5000 });
		expect(fileViewer).not.toHaveBeenCalled();
	});
});

describe("FileTab markdown preview", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	const Harness = () => {
		const [draft, setDraft] = useState<FileDraft>();
		return (
			<FileTab
				chatId="chat-1"
				agentId={agentId}
				path={path}
				reveal={undefined}
				isActive
				draft={draft}
				onDraftChange={(_, update) => setDraft(update)}
				onClose={() => {}}
				readOnly={false}
			/>
		);
	};

	it("opens large files in Edit and previews them on request", async () => {
		const user = userEvent.setup();
		const content = `# Big\n\n${"word ".repeat(60_000)}`;
		const bytes = new TextEncoder().encode(content);
		vi.spyOn(API, "readWorkspaceAgentFile").mockImplementation(
			async (_agentId, _path, range) => {
				const start = range?.offset ?? 0;
				const end =
					range?.limit === undefined ? bytes.length : start + range.limit;
				return {
					data: bytes.slice(start, end).buffer,
					contentType: "text/markdown",
					etag: '"v1"',
				};
			},
		);
		renderWithProviders(<Harness />);

		await screen.findByLabelText("File contents");
		await screen.findByText("Ln 3, Col 1");
		const preview = screen.getByRole("button", {
			name: "Preview (large file)",
		});
		expect(preview.getAttribute("aria-pressed")).toBe("false");
		await user.click(preview);

		expect((await screen.findByRole("heading", { name: "Big" })).id).toBe(
			"user-content-big",
		);
		// The preview has no cursor.
		await screen.findByText("Markdown · 300 kB · 3 lines · LF");
		expect(screen.queryByText("Ln 3, Col 1")).toBeNull();
	});
});
