import {
	MockUserOwner,
	MockWorkspace,
	MockWorkspaceAgent,
} from "testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
	withProxyProvider,
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
import { RIGHT_PANEL_OPEN_KEY } from "./AgentDetailView";
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
							requestArchiveAndDeleteWorkspace: () => {},
							requestUnarchiveAgent: () => {},
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
	decorators: [
		withAuthProvider,
		withDashboardProvider,
		withProxyProvider(),
		withWebSocket,
	],
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

/** Multi-turn conversation with message history and the chat input visible. */
export const WithMessageHistory: Story = {
	parameters: {
		queries: buildQueries(
			{
				chat: {
					id: CHAT_ID,
					...baseChatFields,
					title: "Help me refactor this module",
					status: "completed",
				},
				messages: [
					{
						id: 1,
						chat_id: CHAT_ID,
						created_at: "2026-02-18T00:01:00.000Z",
						role: "user",
						content: [
							{
								type: "text",
								text: "Can you help me refactor the authentication module? It's gotten pretty messy.",
							},
						],
					},
					{
						id: 2,
						chat_id: CHAT_ID,
						created_at: "2026-02-18T00:01:30.000Z",
						role: "assistant",
						content: [
							{
								type: "text",
								text: "Sure! I'll start by looking at the current structure. The main issues I can see are:\n\n1. **Mixed concerns** — token validation and session management are interleaved\n2. **No error hierarchy** — all auth errors are treated the same\n3. **Duplicated middleware** — the same checks appear in three places\n\nLet me propose a cleaner separation.",
							},
						],
					},
					{
						id: 3,
						chat_id: CHAT_ID,
						created_at: "2026-02-18T00:02:00.000Z",
						role: "user",
						content: [
							{
								type: "text",
								text: "That sounds right. Can you start with the token validation? I want to make sure we handle JWT expiration properly.",
							},
						],
					},
					{
						id: 4,
						chat_id: CHAT_ID,
						created_at: "2026-02-18T00:02:30.000Z",
						role: "assistant",
						content: [
							{
								type: "text",
								text: "Here's the refactored token validation:\n\n```go\nfunc ValidateToken(ctx context.Context, token string) (*Claims, error) {\n    claims, err := parseToken(token)\n    if err != nil {\n        return nil, ErrInvalidToken\n    }\n    if claims.ExpiresAt.Before(time.Now()) {\n        return nil, ErrTokenExpired\n    }\n    return claims, nil\n}\n```\n\nKey changes:\n- Separated parsing from expiration checking\n- Added typed errors (`ErrInvalidToken`, `ErrTokenExpired`) so callers can distinguish between a malformed token and an expired one\n- The context parameter allows us to add tracing later",
							},
						],
					},
					{
						id: 5,
						chat_id: CHAT_ID,
						created_at: "2026-02-18T00:03:00.000Z",
						role: "user",
						content: [
							{
								type: "text",
								text: "Looks good. Now what about the middleware deduplication?",
							},
						],
					},
					{
						id: 6,
						chat_id: CHAT_ID,
						created_at: "2026-02-18T00:03:30.000Z",
						role: "assistant",
						content: [
							{
								type: "text",
								text: "I've consolidated the three middleware instances into a single composable chain:\n\n```go\nfunc AuthMiddleware(opts ...AuthOption) func(http.Handler) http.Handler {\n    cfg := defaultAuthConfig()\n    for _, opt := range opts {\n        opt(&cfg)\n    }\n    return func(next http.Handler) http.Handler {\n        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {\n            claims, err := ValidateToken(r.Context(), extractToken(r))\n            if err != nil {\n                cfg.ErrorHandler(w, r, err)\n                return\n            }\n            ctx := context.WithValue(r.Context(), claimsKey, claims)\n            next.ServeHTTP(w, r.WithContext(ctx))\n        })\n    }\n}\n```\n\nThe functional options pattern lets each route customize behavior (e.g. optional auth, different error responses) without duplicating the core logic.",
							},
						],
					},
				],
				queued_messages: [],
			},
			{ diffUrl: undefined },
		),
	},
};

/** Skeleton placeholder when no query data is available yet. */
export const Loading: Story = {};

/** Full layout with actions menu and diff panel portaled to the right slot. */
export const CompletedWithDiffPanel: Story = {
	beforeEach: () => {
		localStorage.setItem(RIGHT_PANEL_OPEN_KEY, "true");
		return () => localStorage.removeItem(RIGHT_PANEL_OPEN_KEY);
	},
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
		webSocket: {
			"/chats/": [
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
 * Sidebar with a PR tab and two git repo tabs. The git watcher receives
 * repo data via the mocked WebSocket, and the diff-status query provides
 * the PR tab.
 */
export const SidebarWithPRAndRepos: Story = {
	beforeEach: () => {
		localStorage.setItem(RIGHT_PANEL_OPEN_KEY, "true");
		return () => localStorage.removeItem(RIGHT_PANEL_OPEN_KEY);
	},
	parameters: {
		queries: buildQueries(
			{
				chat: {
					id: CHAT_ID,
					...baseChatFields,
					title: "Full sidebar demo",
					status: "completed",
				},
				messages: [],
				queued_messages: [],
			},
			{ diffUrl: "https://github.com/coder/coder/pull/456" },
		),
		webSocket: {
			"/git/watch": [
				{
					event: "message",
					data: JSON.stringify({
						type: "changes",
						scanned_at: new Date().toISOString(),
						repositories: [
							{
								repo_root: "/home/coder/frontend",
								branch: "feat/ui-overhaul",
								remote_origin: "https://github.com/coder/frontend.git",
								unified_diff: [
									"diff --git a/src/index.ts b/src/index.ts",
									"index aaa1111..bbb2222 100644",
									"--- a/src/index.ts",
									"+++ b/src/index.ts",
									"@@ -1,4 +1,6 @@",
									' import { render } from "react-dom";',
									'+import { ThemeProvider } from "./theme";',
									"+",
									" const root = document.getElementById('root');",
									"-render(<App />, root);",
									"+render(<ThemeProvider><App /></ThemeProvider>, root);",
									"",
									"diff --git a/src/old-utils.ts b/src/old-utils.ts",
									"deleted file mode 100644",
									"index fff6666..0000000",
									"--- a/src/old-utils.ts",
									"+++ /dev/null",
									"@@ -1,3 +0,0 @@",
									"-export const deprecatedHelper = () => {};",
									"-export const oldFormat = () => {};",
									"-export const legacyParse = () => {};",
									"",
									"diff --git a/src/helpers.ts b/src/utils.ts",
									"similarity index 90%",
									"rename from src/helpers.ts",
									"rename to src/utils.ts",
									"index aaa1111..bbb2222 100644",
									"--- a/src/helpers.ts",
									"+++ b/src/utils.ts",
									"",
									"diff --git a/src/components/Button.tsx b/src/components/Button.tsx",
									"new file mode 100644",
									"index 0000000..abc1234",
									"--- /dev/null",
									"+++ b/src/components/Button.tsx",
									"@@ -0,0 +1,8 @@",
									'+import { type FC } from "react";',
									"+",
									"+interface ButtonProps {",
									"+  children: React.ReactNode;",
									"+}",
									"+",
									"+export const Button: FC<ButtonProps> = ({ children }) => {",
									'+  return <button className="btn">{children}</button>;',
									"+};",
								].join("\n"),
							},
							{
								repo_root: "/home/coder/backend",
								branch: "feat/api-v2",
								remote_origin: "https://github.com/coder/backend.git",
								unified_diff: [
									"diff --git a/cmd/server/main.go b/cmd/server/main.go",
									"index ddd4444..eee5555 100644",
									"--- a/cmd/server/main.go",
									"+++ b/cmd/server/main.go",
									"@@ -15,3 +15,7 @@ func main() {",
									'   srv := &http.Server{Addr: ":8080"}',
									"+  srv.ReadTimeout = 30 * time.Second",
									"+  srv.WriteTimeout = 30 * time.Second",
									"+",
									'+  log.Println("starting server on :8080")',
									"   log.Fatal(srv.ListenAndServe())",
									" }",
									"",
									"diff --git a/internal/old-handler.go b/internal/old-handler.go",
									"deleted file mode 100644",
									"index ccc3333..0000000",
									"--- a/internal/old-handler.go",
									"+++ /dev/null",
									"@@ -1,5 +0,0 @@",
									"-package internal",
									"-",
									"-func OldHandler() {",
									"-  // deprecated",
									"-}",
									"",
									"diff --git a/internal/handler.go b/internal/router.go",
									"similarity index 85%",
									"rename from internal/handler.go",
									"rename to internal/router.go",
									"index aaa1111..bbb2222 100644",
									"--- a/internal/handler.go",
									"+++ b/internal/router.go",
									"",
									"diff --git a/internal/middleware.go b/internal/middleware.go",
									"new file mode 100644",
									"index 0000000..def5678",
									"--- /dev/null",
									"+++ b/internal/middleware.go",
									"@@ -0,0 +1,12 @@",
									"+package internal",
									"+",
									'+import "net/http"',
									"+",
									"+// Logger is a middleware that logs incoming requests.",
									"+func Logger(next http.Handler) http.Handler {",
									"+  return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {",
									'+    log.Printf("%s %s", r.Method, r.URL.Path)',
									"+    next.ServeHTTP(w, r)",
									"+  })",
									"+}",
								].join("\n"),
							},
							{
								repo_root: "/home/coder/docs",
								branch: "feat/ui-overhaul",
								remote_origin: "https://github.com/coder/docs.git",
								unified_diff: [
									"diff --git a/guides/setup.md b/guides/setup.md",
									"index aaa1111..bbb2222 100644",
									"--- a/guides/setup.md",
									"+++ b/guides/setup.md",
									"@@ -5,7 +5,9 @@ ## Installation",
									" ",
									" ```bash",
									"-npm install @coder/sdk",
									"+npm install @coder/sdk@latest",
									" ```",
									" ",
									"+> **Note:** Requires Node.js 18 or later.",
									"+",
									" ## Configuration",
									"",
									"diff --git a/guides/auth.md b/guides/auth.md",
									"index ccc3333..ddd4444 100644",
									"--- a/guides/auth.md",
									"+++ b/guides/auth.md",
									"@@ -12,8 +12,10 @@ ## Token refresh",
									" Tokens are valid for 24 hours.",
									" ",
									"-To refresh a token, call `refreshToken()`.",
									"-This will invalidate the old token.",
									"+To refresh a token, call `client.refreshToken()`.",
									"+This will invalidate the old token and return a",
									"+new one with an extended expiry.",
									" ",
									" ## Revoking access",
								].join("\n"),
							},
						],
					} satisfies TypesGen.WorkspaceAgentGitServerMessage),
				},
			],
		},
	},
};

/**
 * Sidebar with a single git repo tab (no PR). Because there is only one tab
 * the tab bar is hidden and the repo panel is rendered directly.
 */
export const SidebarWithSingleRepo: Story = {
	beforeEach: () => {
		localStorage.setItem(RIGHT_PANEL_OPEN_KEY, "true");
		return () => localStorage.removeItem(RIGHT_PANEL_OPEN_KEY);
	},
	parameters: {
		queries: buildQueries(
			{
				chat: {
					id: CHAT_ID,
					...baseChatFields,
					title: "Single repo sidebar",
					status: "completed",
				},
				messages: [],
				queued_messages: [],
			},
			{ diffUrl: undefined },
		),
		webSocket: {
			"/git/watch": [
				{
					event: "message",
					data: JSON.stringify({
						type: "changes",
						scanned_at: new Date().toISOString(),
						repositories: [
							{
								repo_root: "/home/coder/project",
								branch: "main",
								remote_origin: "https://github.com/coder/project.git",
								unified_diff: [
									"diff --git a/src/app.ts b/src/app.ts",
									"index aaa1111..bbb2222 100644",
									"--- a/src/app.ts",
									"+++ b/src/app.ts",
									"@@ -1,5 +1,7 @@",
									' import express from "express";',
									'+import cors from "cors";',
									" ",
									" const app = express();",
									"+app.use(cors());",
									" ",
									' app.get("/", (req, res) => {',
									"",
									"diff --git a/README.md b/README.md",
									"index ccc3333..ddd4444 100644",
									"--- a/README.md",
									"+++ b/README.md",
									"@@ -1,3 +1,5 @@",
									" # Project",
									" ",
									"-A simple app.",
									"+A simple app with CORS support.",
									"+",
									"+## Getting Started",
								].join("\n"),
							},
						],
					} satisfies TypesGen.WorkspaceAgentGitServerMessage),
				},
			],
		},
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
		webSocket: {
			"/chats/": [
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
		await expect(
			canvas.findByText("Streaming reasoning body"),
		).resolves.toBeInTheDocument();
	},
};
