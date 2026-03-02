import {
	MockUserOwner,
	MockWorkspace,
	MockWorkspaceAgent,
} from "testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
	withWebSocket,
} from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { API } from "api/api";
import {
	chatDiffContentsKey,
	chatDiffStatusKey,
	chatKey,
	chatModelsKey,
	chatsKey,
} from "api/queries/chats";
import { workspaceByIdKey } from "api/queries/workspaces";
import type * as TypesGen from "api/typesGenerated";
import type { FC } from "react";
import { Outlet } from "react-router";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import {
	reactRouterOutlet,
	reactRouterParameters,
} from "storybook-addon-remix-react-router";
import AgentDetail from "./AgentDetail";
import type { AgentsOutletContext } from "./AgentsPage";

// ---------------------------------------------------------------------------
// Layout wrapper – provides outlet context for the child route.
// ---------------------------------------------------------------------------
const AgentDetailLayout: FC = () => {
	return (
		<div className="flex h-full">
			<div className="flex min-w-0 flex-1 flex-col overflow-hidden">
				<Outlet
					context={
						{
							chatErrorReasons: {},
							setChatErrorReason: () => {},
							clearChatErrorReason: () => {},
							requestArchiveAgent: () => {},
							isSidebarCollapsed: false,
							onToggleSidebarCollapsed: () => {},
						} satisfies AgentsOutletContext
					}
				/>
			</div>
		</div>
	);
};

// ---------------------------------------------------------------------------
// Shared mock data
// ---------------------------------------------------------------------------
const CHAT_ID = "chat-1";

const mockWorkspaceAgent: TypesGen.WorkspaceAgent = {
	...MockWorkspaceAgent,
	id: "workspace-agent-1",
	name: "workspace-agent",
	expanded_directory: "/workspace/project",
	apps: [],
};

const mockWorkspace: TypesGen.Workspace = {
	...MockWorkspace,
	id: "workspace-1",
	owner_name: "owner",
	name: "workspace-name",
	latest_build: {
		...MockWorkspace.latest_build,
		resources: [
			{
				...MockWorkspace.latest_build.resources[0],
				agents: [mockWorkspaceAgent],
			},
		],
	},
};

const mockModelCatalog: TypesGen.ChatModelsResponse = {
	providers: [
		{
			provider: "openai",
			available: true,
			models: [
				{
					id: "openai:gpt-4o",
					provider: "openai",
					model: "gpt-4o",
					display_name: "GPT-4o",
				},
			],
		},
	],
};

const baseChatFields = {
	owner_id: "owner-id",
	workspace_id: mockWorkspace.id,
	last_model_config_id: "model-config-1",
	created_at: "2026-02-18T00:00:00.000Z",
	updated_at: "2026-02-18T00:00:00.000Z",
	archived: false,
	last_error: null,
} as const;

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** A small sample unified diff for stories that show the diff panel. */
const sampleDiff = `diff --git a/main.go b/main.go
index abc1234..def5678 100644
--- a/main.go
+++ b/main.go
@@ -10,6 +10,9 @@ func main() {
 	fmt.Println("hello")
+	fmt.Println("new feature")
+	fmt.Println("added line")
+	fmt.Println("another addition")
 	fmt.Println("world")
-	fmt.Println("old line")
 }
`;

/** Build `parameters.queries` entries for a given chat data object. */
const buildQueries = (
	chatData: TypesGen.ChatWithMessages,
	opts?: { diffUrl?: string },
) => [
	{ key: chatKey(CHAT_ID), data: chatData },
	{ key: chatsKey, data: [chatData.chat] },
	{
		key: chatDiffStatusKey(CHAT_ID),
		data: {
			chat_id: CHAT_ID,
			url: opts?.diffUrl,
			changes_requested: false,
			additions: opts?.diffUrl ? 4 : 0,
			deletions: opts?.diffUrl ? 1 : 0,
			changed_files: opts?.diffUrl ? 2 : 0,
		} satisfies TypesGen.ChatDiffStatus,
	},
	{
		key: chatDiffContentsKey(CHAT_ID),
		data: {
			chat_id: CHAT_ID,
			diff: opts?.diffUrl ? sampleDiff : undefined,
			pull_request_url: opts?.diffUrl,
		} satisfies TypesGen.ChatDiffContents,
	},
	{
		key: workspaceByIdKey(mockWorkspace.id),
		data: mockWorkspace,
	},
	{ key: chatModelsKey, data: mockModelCatalog },
];

/**
 * Wrap a chat stream event payload in the JSON string format that
 * OneWayWebSocket expects when receiving a WebSocket message event.
 * The result is a `ServerSentEvent` of type `"data"` serialised to JSON.
 */
const wrapSSE = (payload: unknown): string =>
	JSON.stringify({ type: "data", data: payload });

// ---------------------------------------------------------------------------
// Meta
// ---------------------------------------------------------------------------
const meta: Meta<typeof AgentDetailLayout> = {
	title: "pages/AgentsPage/AgentDetail",
	component: AgentDetailLayout,
	decorators: [withAuthProvider, withDashboardProvider, withWebSocket],
	parameters: {
		layout: "fullscreen",
		user: MockUserOwner,
		webSocket: [],
		reactRouter: reactRouterParameters({
			location: {
				path: `/agents/${CHAT_ID}`,
				pathParams: { agentId: CHAT_ID },
			},
			routing: reactRouterOutlet({ path: "/agents/:agentId" }, <AgentDetail />),
		}),
	},
	beforeEach: () => {
		spyOn(API, "getApiKey").mockRejectedValue(new Error("missing API key"));
	},
};

export default meta;
type Story = StoryObj<typeof AgentDetailLayout>;

// ---------------------------------------------------------------------------
// Stories
// ---------------------------------------------------------------------------

/** Skeleton placeholder when no query data is available yet. */
export const Loading: Story = {};

/** Full layout with actions menu and diff panel portaled to the right slot. */
export const CompletedWithDiffPanel: Story = {
	parameters: {
		queries: buildQueries(
			{
				chat: {
					id: CHAT_ID,
					...baseChatFields,
					title: "Build a feature",
					status: "completed",
				},
				messages: [],
				queued_messages: [],
			},
			{ diffUrl: "https://github.com/coder/coder/pull/123" },
		),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();

		// Wait for the actions menu trigger to appear in the top bar.
		const menuTrigger = await canvas.findByRole("button", {
			name: "Open agent actions",
		});
		await user.click(menuTrigger);

		// Verify menu items are rendered.
		const body = within(document.body);
		await waitFor(() => {
			expect(body.getByText("Open in Cursor")).toBeInTheDocument();
		});
		expect(body.getByText("Open in VS Code")).toBeInTheDocument();
		expect(body.getByText("View Workspace")).toBeInTheDocument();
		expect(body.getByText("Archive Agent")).toBeInTheDocument();
	},
};

/** Right panel stays closed when no diff-status URL exists. */
export const NoDiffUrl: Story = {
	parameters: {
		queries: buildQueries(
			{
				chat: {
					id: CHAT_ID,
					...baseChatFields,
					title: "No diff yet",
					status: "completed",
				},
				messages: [],
				queued_messages: [],
			},
			{ diffUrl: undefined },
		),
	},
};

/** Subagent tool-call/result messages render subagent cards. */
export const WithSubagentCards: Story = {
	parameters: {
		queries: buildQueries(
			{
				chat: {
					id: CHAT_ID,
					...baseChatFields,
					title: "Parent agent",
					status: "running",
				},
				messages: [
					{
						id: 1,
						chat_id: CHAT_ID,
						created_at: "2026-02-18T00:00:01.000Z",
						role: "assistant",
						content: [
							{
								type: "tool-call",
								tool_call_id: "tool-subagent-1",
								tool_name: "spawn_agent",
								args: { title: "Child agent" },
							},
							{
								type: "tool-result",
								tool_call_id: "tool-subagent-1",
								tool_name: "spawn_agent",
								result: {
									chat_id: "child-chat-1",
									title: "Child agent",
									status: "pending",
								},
							},
						],
					},
				],
				queued_messages: [],
			},
			{ diffUrl: undefined },
		),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			expect(
				canvas.getByRole("button", { name: /Spawn(?:ed|ing) Child agent/ }),
			).toBeInTheDocument();
		});
	},
};

/** Reasoning part renders collapsed and can be expanded on click. */
export const WithReasoningCollapsed: Story = {
	parameters: {
		queries: buildQueries(
			{
				chat: {
					id: CHAT_ID,
					...baseChatFields,
					title: "Reasoning title",
					status: "completed",
				},
				messages: [
					{
						id: 1,
						chat_id: CHAT_ID,
						created_at: "2026-02-18T00:00:01.000Z",
						role: "assistant",
						content: [
							{
								type: "reasoning",
								title: "Plan migration",
								text: "Reasoning body",
							},
						],
					},
				],
				queued_messages: [],
			},
			{ diffUrl: undefined },
		),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();

		const reasoningToggle = await canvas.findByRole("button", {
			name: "Plan migration",
		});
		expect(reasoningToggle).toHaveAttribute("aria-expanded", "false");

		await user.click(reasoningToggle);

		expect(reasoningToggle).toHaveAttribute("aria-expanded", "true");
		expect(canvas.getByText("Reasoning body")).toBeInTheDocument();
	},
};

/**
 * Streaming subagent title via WebSocket message_part events.
 * The `withWebSocket` decorator replays all events after a setTimeout(0),
 * and OneWayWebSocket parses each JSON payload, so the streamed title
 * should appear once the play function runs.
 */
export const StreamedSubagentTitle: Story = {
	parameters: {
		queries: buildQueries(
			{
				chat: {
					id: CHAT_ID,
					...baseChatFields,
					title: "Streaming title",
					status: "running",
				},
				messages: [],
				queued_messages: [],
			},
			{ diffUrl: undefined },
		),
		webSocket: [
			{
				event: "message",
				data: wrapSSE({
					type: "message_part",
					message_part: {
						part: {
							type: "tool-call",
							tool_call_id: "tool-subagent-stream-1",
							tool_name: "spawn_agent",
							args_delta: '{"title":"Streamed Child"',
						},
					},
				}),
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			expect(
				canvas.getByRole("button", {
					name: /Spawning Streamed Child/,
				}),
			).toBeInTheDocument();
		});
	},
};

/**
 * Streaming reasoning part via WebSocket — renders collapsed and
 * can be expanded on click.
 */
export const StreamedReasoningCollapsed: Story = {
	parameters: {
		queries: buildQueries(
			{
				chat: {
					id: CHAT_ID,
					...baseChatFields,
					title: "Streaming reasoning title",
					status: "running",
				},
				messages: [],
				queued_messages: [],
			},
			{ diffUrl: undefined },
		),
		webSocket: [
			{
				event: "message",
				data: wrapSSE({
					type: "message_part",
					message_part: {
						part: {
							type: "reasoning",
							title: "Plan migration",
							text: "Streaming reasoning body",
						},
					},
				}),
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();

		const reasoningToggle = await canvas.findByRole("button", {
			name: "Plan migration",
		});
		expect(reasoningToggle).toHaveAttribute("aria-expanded", "false");

		await user.click(reasoningToggle);

		expect(reasoningToggle).toHaveAttribute("aria-expanded", "true");
		expect(canvas.getByText("Streaming reasoning body")).toBeInTheDocument();
	},
};
