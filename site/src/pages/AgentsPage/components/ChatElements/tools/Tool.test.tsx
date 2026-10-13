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

describe("Tool web search rows", () => {
	const queries = { type: "search", queries: [" q1 ", "q2", "q2", ""] };
	const openPage = { type: "open_page", url: "https://coder.com/docs" };
	const findInPage = { ...openPage, type: "find_in_page", pattern: "tpl" };

	it.each<{
		case: string;
		props: Partial<React.ComponentProps<typeof Tool>>;
		label: string;
	}>([
		{
			case: "OpenAI queries, trimmed and deduplicated",
			props: { args: queries },
			label: "Searched for q1, q2",
		},
		{
			case: "Anthropic query",
			props: { args: { query: "coder templates" } },
			label: "Searched for coder templates",
		},
		{
			case: "no queries",
			props: { args: { type: "search" } },
			label: "Searched the web",
		},
		{
			case: "open_page",
			props: { args: openPage },
			label: "Opened https://coder.com/docs",
		},
		{
			case: "find_in_page",
			props: { args: findInPage },
			label: "Searched https://coder.com/docs for tpl",
		},
		{
			case: "running",
			props: { args: queries, status: "running", result: undefined },
			label: "Searching for q1, q2",
		},
		{
			case: "failed",
			props: {
				args: openPage,
				status: "error",
				isError: true,
				result: { error: "web search failed" },
			},
			label: "Failed to open https://coder.com/docs",
		},
		{
			case: "message ended without a result",
			props: { args: queries, result: undefined },
			label: "Did not finish searching for q1, q2",
		},
	])("labels $case", ({ props, label }) => {
		renderWebSearch(props);
		expect(screen.getByText(label)).toBeInTheDocument();
	});

	it("uses the generic row for a client tool named web_search", () => {
		renderWebSearch({
			providerExecuted: false,
			args: { query: "coder templates" },
		});
		expect(screen.getByRole("button")).toHaveAccessibleName("web_search");
	});

	it("links found pages only for http and https URLs", async () => {
		renderWebSearch({
			args: queries,
			foundPages: [
				{ url: "https://coder.com/docs/ai", title: "" },
				{ url: "http://coder.com/", title: "Coder" },
				{ url: "ftp://example.com/release.txt", title: "" },
				{ url: "data:text/html,unsafe", title: "" },
			],
		});
		await userEvent.click(screen.getByRole("button", { expanded: false }));

		const links = screen.getAllByRole("link");
		expect(links.map((link) => link.getAttribute("href"))).toEqual([
			"https://coder.com/docs/ai",
			"http://coder.com/",
		]);
		// Untitled pages show hostname and path; titled pages their title.
		expect(links[0]).toHaveTextContent("coder.com/docs/ai");
		expect(links[1]).toHaveTextContent("Coder");
		expect(screen.getByText("example.com/release.txt")).toBeInTheDocument();
		expect(screen.getByText("data:text/html,unsafe")).toBeInTheDocument();
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
