import { render, screen } from "@testing-library/react";
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

const renderWebSearch = (
	props: Partial<React.ComponentProps<typeof Tool>> = {},
) =>
	renderComponent(
		<QueryClientProvider client={createTestQueryClient()}>
			<Tool
				name="web_search"
				providerExecuted
				status="completed"
				result={{}}
				organizationId="organization-id"
				mcpServers={[]}
				isError={false}
				subagentTitles={new Map()}
				subagentVariants={new Map()}
				shellToolDisplayMode="auto"
				codeDiffDisplayMode="auto"
				{...props}
			/>
		</QueryClientProvider>,
	);

// Labels and pills are visual states covered by the web search stories.
describe("Tool web search rows", () => {
	it("uses the generic row for a client tool named web_search", () => {
		renderWebSearch({
			providerExecuted: false,
			args: { query: "coder templates" },
		});
		expect(screen.getByRole("button")).toHaveAccessibleName("web_search");
	});

	it("links found pages only for http and https URLs", async () => {
		renderWebSearch({
			args: { type: "search", queries: ["coder"] },
			foundPages: [
				{ url: "https://coder.com/docs/ai", title: "" },
				{ url: "http://coder.com/", title: "Coder" },
				{ url: "ftp://example.com/release.txt", title: "" },
				{ url: "data:text/html,unsafe", title: "" },
			],
		});
		await userEvent.click(screen.getByRole("button", { expanded: false }));

		expect(
			screen.getAllByRole("link").map((link) => link.getAttribute("href")),
		).toEqual(["https://coder.com/docs/ai", "http://coder.com/"]);
	});
});

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
		"$name $status opens no logs until expanded, agent logs: $streamsAgentLogs",
		async ({ name, status, streamsAgentLogs }) => {
			const buildId = MockWorkspace.latest_build.id;
			const isRunning = status === "running";
			const watchBuildLogs = vi
				.spyOn(apiModule, "watchBuildLogsByBuildId")
				.mockImplementation(() => createMockWebSocket("ws://test")[0]);
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
			// useWorkspaceWatch owns this cache entry in the app. A pending
			// mount refetch keeps the seeded workspace in place.
			vi.spyOn(API, "getWorkspace").mockReturnValue(new Promise(() => {}));
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
							organizationId="organization-id"
							mcpServers={[]}
							isError={false}
							subagentTitles={new Map()}
							subagentVariants={new Map()}
							shellToolDisplayMode="auto"
							codeDiffDisplayMode="auto"
						/>
					</ChatWorkspaceContext>
				</QueryClientProvider>,
			);

			expect(watchBuildLogs).not.toHaveBeenCalled();
			expect(watchAgentLogs).not.toHaveBeenCalled();

			await userEvent.click(screen.getByRole("button", { expanded: false }));

			expect(watchBuildLogs).toHaveBeenCalledWith(buildId, expect.anything());
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

describe("Tool generic rows", () => {
	it("falls back to the tool name when the model intent is whitespace", () => {
		renderComponent(
			<QueryClientProvider client={createTestQueryClient()}>
				<Tool
					name="custom_tool"
					organizationId="organization-id"
					mcpServers={[]}
					status="completed"
					args={{ query: "value" }}
					isError={false}
					modelIntent="   "
					subagentTitles={new Map()}
					subagentVariants={new Map()}
					shellToolDisplayMode="auto"
					codeDiffDisplayMode="auto"
				/>
			</QueryClientProvider>,
		);

		expect(screen.getByRole("button")).toHaveAccessibleName("custom_tool");
	});
});
