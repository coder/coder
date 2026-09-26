import type { FileDiffMetadata } from "@pierre/diffs";
import type * as diffsReact from "@pierre/diffs/react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClientProvider } from "react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import * as apiModule from "#/api/api";
import { API } from "#/api/api";
import { workspaceByIdKey } from "#/api/queries/workspaces";
import { MockWorkspace, MockWorkspaceAgent } from "#/testHelpers/entities";
import {
	createTestQueryClient,
	renderComponent,
} from "#/testHelpers/renderHelpers";
import { createMockWebSocket } from "#/testHelpers/websockets";
import { OneWayWebSocket } from "#/utils/OneWayWebSocket";
import { ChatWorkspaceContext } from "../../../context/ChatWorkspaceContext";
import { Tool } from "./Tool";

afterEach(() => {
	vi.restoreAllMocks();
});

// The diff web component cannot construct its stylesheet in jsdom. The
// stub renders the added lines so tests can tell which diff was shown.
vi.mock("@pierre/diffs/react", async (importOriginal) => ({
	...(await importOriginal<typeof diffsReact>()),
	FileDiff: ({ fileDiff }: { fileDiff: FileDiffMetadata }) => (
		<pre>{fileDiff.additionLines.join("")}</pre>
	),
}));

describe("Tool workspace lifecycle rows", () => {
	it.each(
		[
			{ name: "start_workspace", streamsAgentLogs: true },
			{ name: "create_workspace", streamsAgentLogs: true },
			{ name: "stop_workspace", streamsAgentLogs: false },
		].flatMap((row) => [
			{ ...row, status: "running" as const },
			{ ...row, status: "completed" as const },
		]),
	)(
		"$name $status shows its build's logs, agent logs: $streamsAgentLogs",
		async ({ name, status, streamsAgentLogs }) => {
			const buildId = MockWorkspace.latest_build.id;
			const isRunning = status === "running";
			const watchBuildLogs = vi
				.spyOn(apiModule, "watchBuildLogsByBuildId")
				.mockImplementation(() => createMockWebSocket("ws://test")[0]);
			const getBuildLogs = vi
				.spyOn(API, "getWorkspaceBuildLogs")
				.mockResolvedValue([]);
			const watchAgentLogs = vi
				.spyOn(apiModule, "watchWorkspaceAgentLogs")
				.mockImplementation(
					(agentId) =>
						new OneWayWebSocket({
							apiRoute: `/api/v2/workspaceagents/${agentId}/logs`,
							websocketInit: (url, protocol) =>
								createMockWebSocket(url, protocol)[0],
						}),
				);
			vi.spyOn(API, "getWorkspace").mockResolvedValue(MockWorkspace);
			const queryClient = createTestQueryClient();
			queryClient.setQueryData(
				workspaceByIdKey(MockWorkspace.id),
				MockWorkspace,
			);

			// Completed rows omit the binding so only the result build_id matches.
			render(
				<QueryClientProvider client={queryClient}>
					<ChatWorkspaceContext
						value={{
							workspaceId: MockWorkspace.id,
							buildId: isRunning ? buildId : undefined,
							agentId: MockWorkspaceAgent.id,
						}}
					>
						<Tool
							name={name}
							status={status}
							result={isRunning ? undefined : { build_id: buildId }}
						/>
					</ChatWorkspaceContext>
				</QueryClientProvider>,
			);
			if (!isRunning) {
				await userEvent.click(screen.getByRole("button", { expanded: false }));
			}

			if (isRunning) {
				expect(watchBuildLogs).toHaveBeenCalledWith(buildId, expect.anything());
			} else {
				await waitFor(() => {
					expect(getBuildLogs).toHaveBeenCalledWith(buildId);
				});
			}
			if (streamsAgentLogs) {
				expect(watchAgentLogs).toHaveBeenCalledWith(
					MockWorkspaceAgent.id,
					expect.anything(),
				);
			} else {
				expect(watchAgentLogs).not.toHaveBeenCalled();
			}
		},
	);
});

describe("Tool edit_files rows", () => {
	const edit = (path: string) => ({ path, old_text: "old", new_text: "new" });
	const twoFiles = { edits: [edit("/repo/a.go"), edit("/repo/b.go")] };
	const threeFiles = {
		edits: [edit("/repo/a.go"), edit("/repo/b.go"), edit("/repo/c.go")],
	};
	// Server diffs add a line that differs from new_text, so a diff built
	// from the args instead would not show it.
	const diff = (path: string) =>
		`--- ${path}\n+++ ${path}\n@@ -1,1 +1,1 @@\n-old\n+new // server\n`;
	const applied = (path: string) => ({
		path,
		status: "applied",
		diff: diff(path),
	});
	const ambiguous =
		"old_text matches 3 occurrences (expected exactly 1). Include more surrounding context to make the match unique, or set replace_all to true";
	// Each row is [accessible name, text content] in document order.
	const serverDiffRow = (path: string) => [
		`Diff of ${path}`,
		"new // server\n",
	];
	const argsDiffRow = (path: string) => [`Diff of ${path}`, "new"];
	const rejectedRow = (path: string, error: string) => [
		`Edits to ${path} not applied`,
		`${path}${error}`,
	];
	const unreportedRow = (path: string) => [
		`No result reported for ${path}`,
		`${path}No result reported for this file.`,
	];
	const unknownRow = (path: string, error: string) => [
		`Edits to ${path} may not have been applied`,
		`${path}${error}`,
	];
	const transportError =
		"the workspace agent connection closed before a response arrived";

	it.each<{
		name: string;
		args: unknown;
		result: unknown;
		isError?: boolean;
		header: string;
		rows: string[][];
		shownError?: string;
		hiddenText?: string;
	}>([
		{
			name: "stored ok result",
			args: twoFiles,
			result: {
				ok: true,
				files: [
					{ path: "/repo/a.go", diff: diff("/repo/a.go") },
					{ path: "/repo/b.go", diff: diff("/repo/b.go") },
				],
			},
			header: "Edited 2 files",
			rows: [serverDiffRow("/repo/a.go"), serverDiffRow("/repo/b.go")],
		},
		{
			name: "stored ok result missing a file falls back to the args diff",
			args: twoFiles,
			result: {
				ok: true,
				files: [{ path: "/repo/a.go", diff: diff("/repo/a.go") }],
			},
			header: "Edited 2 files",
			rows: [serverDiffRow("/repo/a.go"), argsDiffRow("/repo/b.go")],
		},
		{
			name: "applied status result",
			args: twoFiles,
			result: {
				status: "applied",
				message: "Applied edits to 2 files.",
				files: [applied("/repo/a.go"), applied("/repo/b.go")],
			},
			header: "Edited 2 files",
			rows: [serverDiffRow("/repo/a.go"), serverDiffRow("/repo/b.go")],
			hiddenText: "Applied edits to 2 files.",
		},
		{
			// Older agents return no per-file results; every file was written.
			name: "applied result without per-file results falls back to args diffs",
			args: twoFiles,
			result: {
				status: "applied",
				message: "Applied edits to 2 files.",
				files: [],
			},
			header: "Edited 2 files",
			rows: [argsDiffRow("/repo/a.go"), argsDiffRow("/repo/b.go")],
		},
		{
			name: "applied entry without a diff falls back to the args diff",
			args: twoFiles,
			result: {
				status: "partial",
				message: "Applied 1 file. /repo/b.go was not applied.",
				files: [
					{
						path: "/repo/b.go",
						status: "rejected",
						edits: [1],
						error: ambiguous,
					},
					{ path: "/repo/a.go", status: "applied" },
				],
			},
			header: "Edited 1 of 2 files",
			rows: [argsDiffRow("/repo/a.go"), rejectedRow("/repo/b.go", ambiguous)],
		},
		{
			name: "applied entry with an empty diff shows no diff",
			args: twoFiles,
			result: {
				status: "applied",
				message: "Applied edits to 2 files.",
				files: [
					{ path: "/repo/a.go", status: "applied", diff: "" },
					applied("/repo/b.go"),
				],
			},
			header: "Edited 2 files",
			rows: [serverDiffRow("/repo/b.go")],
		},
		{
			name: "untrimmed flat paths match trimmed result paths",
			args: { edits: [edit("/repo/a.go\n"), edit(" /repo/b.go")] },
			result: {
				status: "applied",
				message: "Applied edits to 2 files.",
				files: [applied("/repo/a.go"), applied("/repo/b.go")],
			},
			header: "Edited 2 files",
			rows: [serverDiffRow("/repo/a.go"), serverDiffRow("/repo/b.go")],
		},
		{
			name: "untrimmed files paths match trimmed result paths",
			args: {
				files: [
					{
						path: "/repo/a.go\n",
						edits: [{ old_text: "old", new_text: "new" }],
					},
				],
			},
			result: {
				ok: true,
				files: [{ path: "/repo/a.go", diff: diff("/repo/a.go") }],
			},
			header: "Edited a.go",
			rows: [serverDiffRow("/repo/a.go")],
		},
		{
			name: "partial result keeps args order",
			args: threeFiles,
			result: {
				status: "partial",
				message: "Applied 2 files. /repo/b.go was not applied.",
				files: [
					{
						path: "/repo/b.go",
						status: "rejected",
						edits: [1],
						error: ambiguous,
					},
					applied("/repo/a.go"),
					applied("/repo/c.go"),
				],
			},
			header: "Edited 2 of 3 files",
			rows: [
				serverDiffRow("/repo/a.go"),
				rejectedRow("/repo/b.go", ambiguous),
				serverDiffRow("/repo/c.go"),
			],
			hiddenText: "Applied 2 files. /repo/b.go was not applied.",
		},
		{
			name: "partial result missing a file shows it as unreported",
			args: threeFiles,
			result: {
				status: "partial",
				message: "Applied 1 file. /repo/c.go was not applied.",
				files: [
					{
						path: "/repo/c.go",
						status: "rejected",
						edits: [2],
						error: ambiguous,
					},
					applied("/repo/a.go"),
				],
			},
			header: "Edited 1 of 3 files",
			rows: [
				serverDiffRow("/repo/a.go"),
				unreportedRow("/repo/b.go"),
				rejectedRow("/repo/c.go", ambiguous),
			],
		},
		{
			name: "partial result with an unknown file outcome",
			args: threeFiles,
			result: {
				status: "partial",
				message: "Applied 1 file.",
				files: [
					{
						path: "/repo/c.go",
						status: "rejected",
						edits: [2],
						error: ambiguous,
					},
					{ path: "/repo/b.go", status: "unknown", error: transportError },
					applied("/repo/a.go"),
				],
			},
			header: "Edited 1 of 3 files",
			rows: [
				serverDiffRow("/repo/a.go"),
				unknownRow("/repo/b.go", transportError),
				rejectedRow("/repo/c.go", ambiguous),
			],
		},
		{
			name: "error result",
			args: twoFiles,
			result: { error: "No files were applied. old_text not found" },
			isError: true,
			header: "Failed to edit 2 files",
			rows: [],
			shownError: "No files were applied. old_text not found",
		},
	])(
		"$name",
		({ args, result, isError, header, rows, shownError, hiddenText }) => {
			renderComponent(
				<Tool
					name="edit_files"
					status={isError ? "error" : "completed"}
					isError={isError}
					args={args}
					result={result}
					codeDiffDisplayMode="always_expanded"
				/>,
			);

			if (isError) {
				// The failed-status icon adds its label to the button name.
				screen.getByRole("button", {
					name: (name) => name.startsWith(header),
				});
				screen.getByText(shownError ?? "");
			} else {
				// An exact name also proves there is no failed-status icon.
				screen.getByRole("button", { name: header });
			}
			const rendered = [
				...screen.queryAllByRole("region", { name: /^Diff of / }),
				...screen.queryAllByRole("group"),
			].sort((a, b) =>
				a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING
					? -1
					: 1,
			);
			expect(
				rendered.map((el) => [el.getAttribute("aria-label"), el.textContent]),
			).toEqual(rows);
			if (hiddenText) {
				expect(screen.queryByText(hiddenText)).toBeNull();
			}
		},
	);
});
