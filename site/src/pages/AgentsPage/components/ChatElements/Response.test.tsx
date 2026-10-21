import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AxiosError, AxiosHeaders } from "axios";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
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
		afterEach(() => {
			vi.restoreAllMocks();
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
			const { unmount } = render(
				<WorkspaceFilesContext.Provider value={files}>
					<Response>{markdown}</Response>
				</WorkspaceFilesContext.Provider>,
			);
			return { files, unmount };
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
			const { files } = renderWithFiles("See README.md for setup.");

			await user.click(
				await screen.findByRole("button", { name: "README.md" }),
			);
			const picker = await screen.findByRole("dialog", {
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
	});
});
