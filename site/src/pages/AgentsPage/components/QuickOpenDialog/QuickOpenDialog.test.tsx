import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { createTestQueryClient } from "#/testHelpers/renderHelpers";
import {
	WorkspaceFilesContext,
	type WorkspaceFilesContextValue,
} from "../../context/WorkspaceFilesContext";
import { QuickOpenDialog } from "./QuickOpenDialog";

const contextValue = (
	overrides: Partial<WorkspaceFilesContextValue> = {},
): WorkspaceFilesContextValue => ({
	agentId: "agent-1",
	defaultDirectory: "/home/coder",
	repoRoots: ["/home/coder/a", "/home/coder/b"],
	repositories: new Map(),
	openFile: vi.fn(),
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
		);

		await user.keyboard("{ArrowDown}{Enter}");

		expect(value.openFile).toHaveBeenCalledWith("/home/coder/b/main_test.go");
		await waitFor(() =>
			expect(
				screen.queryByRole("combobox", { name: "Search files" }),
			).toBeNull(),
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
			),
		);
	});

	it("ignores the shortcut without a workspace agent", async () => {
		const user = userEvent.setup();
		renderDialog(undefined);

		await user.keyboard("{Control>}p{/Control}");

		expect(screen.queryByRole("dialog")).toBeNull();
	});
});
