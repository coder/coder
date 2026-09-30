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

// FileDiff's web component cannot construct its stylesheet in jsdom. The
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
	const args = {
		edits: [
			{ path: "/repo/a.go", old_text: "x := 1", new_text: "x := 2" },
			{ path: "/repo/b.go", old_text: "foo()", new_text: "bar()" },
		],
	};
	// The server diffs add lines that differ from new_text, which a diff
	// built from the args would not show.
	const diffA =
		"--- /repo/a.go\n+++ /repo/a.go\n@@ -1,1 +1,1 @@\n-x := 1\n+x := 2 // server a\n";
	const diffB =
		"--- /repo/b.go\n+++ /repo/b.go\n@@ -1,1 +1,1 @@\n-foo()\n+bar() // server b\n";

	it.each([
		{
			name: "result with ok: true",
			result: {
				ok: true,
				files: [
					{ path: "/repo/a.go", diff: diffA },
					{ path: "/repo/b.go", diff: diffB },
				],
			},
		},
		{
			name: "result with status: applied",
			result: {
				status: "applied",
				message: "Applied 2 of 2 edits.",
				files: [
					{ path: "/repo/a.go", status: "applied", diff: diffA },
					{ path: "/repo/b.go", status: "applied", diff: diffB },
				],
			},
		},
	])("$name shows server diffs and no error", ({ result }) => {
		renderComponent(
			<Tool
				name="edit_files"
				status="completed"
				args={args}
				result={result}
				codeDiffDisplayMode="always_expanded"
			/>,
		);

		// Matching the exact button name also proves there is no
		// failed-status icon, whose label would be part of the name.
		screen.getByRole("button", { name: "Edited 2 files" });
		const regions = screen.getAllByRole("region", { name: /^Diff of / });
		expect(
			regions.map((el) => [el.getAttribute("aria-label"), el.textContent]),
		).toEqual([
			["Diff of /repo/a.go", "x := 2 // server a\n"],
			["Diff of /repo/b.go", "bar() // server b\n"],
		]);
		expect(screen.queryByText("Applied 2 of 2 edits.")).toBeNull();
	});
});
