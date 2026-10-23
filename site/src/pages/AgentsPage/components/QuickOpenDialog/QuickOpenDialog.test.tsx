import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AxiosError, AxiosHeaders } from "axios";
import { QueryClientProvider } from "react-query";
import { MemoryRouter, Route, Routes } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import {
	WorkspaceFilesContext,
	type WorkspaceFilesContextValue,
} from "../../context/WorkspaceFilesContext";
import { filesPanelRootStorageKeyPrefix } from "../RightPanel/FilesPanel";
import { QuickOpenDialog } from "./QuickOpenDialog";

const contextValue = (
	overrides: Partial<WorkspaceFilesContextValue> = {},
): WorkspaceFilesContextValue => ({
	agentId: "agent-1",
	defaultDirectory: "/home/coder",
	repoRoots: ["/home/coder/a", "/home/coder/b"],
	repositories: new Map(),
	openFile: vi.fn(),
	revealInFiles: vi.fn(),
	diffStatuses: [],
	...overrides,
});

const renderDialog = (value: WorkspaceFilesContextValue | undefined) =>
	render(
		<QueryClientProvider client={createTestQueryClient()}>
			<WorkspaceFilesContext value={value}>
				<QuickOpenDialog />
			</WorkspaceFilesContext>
		</QueryClientProvider>,
	);

describe("QuickOpenDialog", () => {
	beforeEach(() => {
		localStorage.clear();
		vi.spyOn(API, "findWorkspaceAgentFiles").mockResolvedValue({
			results: [
				{
					absolute_path: "/home/coder/a/src/main.go",
					path: "src/main.go",
					root: "/home/coder/a",
					is_dir: false,
					score: 10,
				},
				{
					absolute_path: "/home/coder/b/main_test.go",
					path: "main_test.go",
					root: "/home/coder/b",
					is_dir: false,
					score: 5,
				},
			],
		});
	});

	it("searches the repo roots and opens the chosen file", async () => {
		const user = userEvent.setup();
		const value = contextValue();
		renderDialog(value);

		await user.keyboard("{Control>}p{/Control}");
		await user.type(
			screen.getByRole("combobox", { name: "Search files" }),
			"main",
		);
		await screen.findByRole("option", { name: /main_test\.go/ });

		expect(API.findWorkspaceAgentFiles).toHaveBeenLastCalledWith(
			"agent-1",
			["/home/coder/a", "/home/coder/b"],
			"main",
			undefined,
		);

		await user.keyboard("{ArrowDown}{Enter}");

		expect(value.openFile).toHaveBeenCalledWith("/home/coder/b/main_test.go");
		await waitFor(() =>
			expect(
				screen.queryByRole("combobox", { name: "Search files" }),
			).toBeNull(),
		);
	});

	it("also searches the Files panel root of the chat", async () => {
		const user = userEvent.setup();
		localStorage.setItem(`${filesPanelRootStorageKeyPrefix}chat-1`, "~");
		vi.spyOn(API, "listWorkspaceAgentDirectory").mockResolvedValue({
			absolute_path: ["home", "coder"],
			absolute_path_string: "/home/coder",
			contents: [],
		});
		vi.mocked(API.findWorkspaceAgentFiles).mockResolvedValue({
			results: [
				{
					absolute_path: "/home/coder/demo/hello.go",
					path: "demo/hello.go",
					root: "/home/coder",
					is_dir: false,
					score: 10,
				},
				{
					absolute_path: "/home/coder/a/hello.go",
					path: "hello.go",
					root: "/home/coder/a",
					is_dir: false,
					score: 5,
				},
			],
		});
		render(
			<QueryClientProvider client={createTestQueryClient()}>
				<WorkspaceFilesContext value={contextValue()}>
					<MemoryRouter initialEntries={["/agents/chat-1"]}>
						<Routes>
							<Route path="/agents/:agentId" element={<QuickOpenDialog />} />
						</Routes>
					</MemoryRouter>
				</WorkspaceFilesContext>
			</QueryClientProvider>,
		);

		await user.keyboard("{Control>}p{/Control}");
		await user.type(
			screen.getByRole("combobox", { name: "Search files" }),
			"hello",
		);

		await waitFor(() =>
			expect(API.findWorkspaceAgentFiles).toHaveBeenCalledWith(
				"agent-1",
				["/home/coder/a", "/home/coder/b", "~"],
				"hello",
				undefined,
			),
		);
		// Home and a repo can share a name, so labels start from home.
		await waitFor(() =>
			expect(
				screen.getAllByRole("option").map((option) => option.textContent),
			).toEqual(["hello.go~/demo/hello.go", "hello.go~/a/hello.go"]),
		);
	});

	it("searches the default directory without repo roots", async () => {
		const user = userEvent.setup();
		renderDialog(contextValue({ repoRoots: [] }));

		await user.keyboard("{Control>}p{/Control}");
		await user.type(
			screen.getByRole("combobox", { name: "Search files" }),
			"x",
		);

		await waitFor(() =>
			expect(API.findWorkspaceAgentFiles).toHaveBeenCalledWith(
				"agent-1",
				[""],
				"x",
				undefined,
			),
		);
	});

	it("does not open results of an older query", async () => {
		const user = userEvent.setup();
		const value = contextValue();
		renderDialog(value);

		await user.keyboard("{Control>}p{/Control}");
		const input = screen.getByRole("combobox", { name: "Search files" });
		await user.type(input, "main");
		await screen.findByRole("option", { name: /main_test\.go/ });

		vi.mocked(API.findWorkspaceAgentFiles).mockReturnValue(
			new Promise(() => {}),
		);
		await user.clear(input);
		await user.type(input, "beta");
		await waitFor(() =>
			expect(API.findWorkspaceAgentFiles).toHaveBeenLastCalledWith(
				"agent-1",
				["/home/coder/a", "/home/coder/b"],
				"beta",
				undefined,
			),
		);
		await user.keyboard("{Enter}");

		expect(value.openFile).not.toHaveBeenCalled();
	});

	it("returns focus to the opener on Escape", async () => {
		const user = userEvent.setup();
		render(
			<QueryClientProvider client={createTestQueryClient()}>
				<WorkspaceFilesContext value={contextValue()}>
					<button type="button">Opener</button>
					<QuickOpenDialog />
				</WorkspaceFilesContext>
			</QueryClientProvider>,
		);
		const opener = screen.getByRole("button", { name: "Opener" });
		opener.focus();

		await user.keyboard("{Control>}p{/Control}");
		await user.keyboard("{Escape}");

		await waitFor(() => expect(opener).toHaveFocus());
	});

	it.each([
		["a JSON", { message: "Route not found." }],
		["a plain-text", "404 page not found\n"],
	])("asks to update the agent on %s 404", async (_kind, data) => {
		const user = userEvent.setup();
		vi.mocked(API.findWorkspaceAgentFiles).mockRejectedValue(
			new AxiosError(
				"Request failed with status code 404",
				AxiosError.ERR_BAD_REQUEST,
				undefined,
				undefined,
				{
					data,
					status: 404,
					statusText: "Not Found",
					headers: {},
					config: { headers: new AxiosHeaders() },
				},
			),
		);
		renderDialog(contextValue());

		await user.keyboard("{Control>}p{/Control}");
		await user.type(
			screen.getByRole("combobox", { name: "Search files" }),
			"main",
		);

		await screen.findByText("Update the workspace agent to search files.");
	});

	it("ignores the shortcut without a workspace agent", async () => {
		const user = userEvent.setup();
		renderDialog(undefined);

		await user.keyboard("{Control>}p{/Control}");

		expect(screen.queryByRole("dialog")).toBeNull();
	});
});
