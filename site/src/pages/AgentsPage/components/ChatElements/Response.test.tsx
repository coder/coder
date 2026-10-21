import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { API, type WorkspaceAgentFindFilesResult } from "#/api/api";
import { toast } from "#/components/Toaster/toast";
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
		const renderWithFiles = (markdown: string) => {
			const files: WorkspaceFilesContextValue = {
				agentId: "agent-1",
				defaultDirectory: "/home/coder",
				repoRoots: ["/home/coder/coder", "/home/coder/other"],
				repositories: new Map(),
				openFile: vi.fn(),
			};
			render(
				<WorkspaceFilesContext.Provider value={files}>
					<Response>{markdown}</Response>
				</WorkspaceFilesContext.Provider>,
			);
			return files;
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
			const files = renderWithFiles("Open /home/coder/coder/go.mod#L3.");

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
			const files = renderWithFiles("Edit `api/api.ts:42`.");

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
			);
		});

		it("lets the user choose between several matching files", async () => {
			const user = userEvent.setup();
			vi.spyOn(API, "findWorkspaceAgentFiles").mockResolvedValue({
				results: [
					findResult("/home/coder/coder", "README.md"),
					findResult("/home/coder/other", "docs/README.md"),
				],
			});
			const files = renderWithFiles("See README.md for setup.");

			await user.click(
				await screen.findByRole("button", { name: "README.md" }),
			);
			const picker = await screen.findByRole("list", {
				name: "Files matching README.md",
			});
			await user.click(
				within(picker).getByRole("button", { name: "docs/README.md in other" }),
			);

			expect(files.openFile).toHaveBeenCalledWith(
				"/home/coder/other/docs/README.md",
				{ line: undefined },
			);
		});

		it("reports when no file matches", async () => {
			const user = userEvent.setup();
			vi.spyOn(API, "findWorkspaceAgentFiles").mockResolvedValue({
				results: [findResult("/home/coder/coder", "src/other.go")],
			});
			const toastError = vi.spyOn(toast, "error");
			const files = renderWithFiles("Look at `main.go`.");

			await user.click(await screen.findByRole("button", { name: "main.go" }));

			await waitFor(() =>
				expect(toastError).toHaveBeenCalledWith(
					"No file matching main.go in this chat's repositories",
				),
			);
			expect(files.openFile).not.toHaveBeenCalled();
		});
	});
});
