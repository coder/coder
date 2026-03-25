import type { Meta, StoryObj } from "@storybook/react-vite";
import type { FC } from "react";
import { useRef } from "react";
import { Outlet } from "react-router";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import {
	reactRouterOutlet,
	reactRouterParameters,
} from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import {
	chatDiffContentsKey,
	chatKey,
	chatMessagesKey,
	chatModelConfigs,
	chatModelsKey,
	chatsKey,
	mcpServerConfigsKey,
} from "#/api/queries/chats";
import { workspaceByIdKey } from "#/api/queries/workspaces";
import type * as TypesGen from "#/api/typesGenerated";
import {
	MockUserOwner,
	MockWorkspace,
	MockWorkspaceAgent,
} from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
	withProxyProvider,
	withWebSocket,
} from "#/testHelpers/storybook";
import AgentDetail, { RIGHT_PANEL_OPEN_KEY } from "./AgentDetail";
import type { AgentsOutletContext } from "./AgentsPage";

// ---------------------------------------------------------------------------
// Layout wrapper – provides outlet context for the child route.
// ---------------------------------------------------------------------------
const AgentDetailLayout: FC = () => {
	const scrollContainerRef = useRef<HTMLDivElement | null>(null);
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
							requestArchiveAndDeleteWorkspace: (
								_chatId: string,
								_workspaceId: string,
							) => {},
							requestUnarchiveAgent: () => {},
							requestPinAgent: () => {},
							requestUnpinAgent: () => {},
							onRegenerateTitle: () => {},
							isRegeneratingTitle: false,
							regeneratingTitleChatId: null,
							isSidebarCollapsed: false,
							onToggleSidebarCollapsed: () => {},
							onExpandSidebar: () => {},
							onChatReady: () => {},
							scrollContainerRef,
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
const MODEL_CONFIG_ID = "model-config-1";

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

const mockModelConfigs: TypesGen.ChatModelConfig[] = [
	{
		id: MODEL_CONFIG_ID,
		provider: "openai",
		model: "gpt-4o",
		display_name: "GPT-4o",
		enabled: true,
		is_default: true,
		context_limit: 200000,
		compression_threshold: 70,
		created_at: "2026-02-18T00:00:00.000Z",
		updated_at: "2026-02-18T00:00:00.000Z",
	},
];

const baseChatFields = {
	owner_id: "owner-id",
	workspace_id: mockWorkspace.id,
	last_model_config_id: MODEL_CONFIG_ID,
	mcp_server_ids: [],
	labels: {},
	created_at: "2026-02-18T00:00:00.000Z",
	updated_at: "2026-02-18T00:00:00.000Z",
	archived: false,
	pin_order: 0,
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

/** Build `parameters.queries` entries for a given chat and messages. */
const buildQueries = (
	chat: TypesGen.Chat,
	messagesData: TypesGen.ChatMessagesResponse,
	opts?: { diffUrl?: string },
) => {
	const diffStatus: TypesGen.ChatDiffStatus = {
		chat_id: CHAT_ID,
		url: opts?.diffUrl,
		pull_request_title: "",
		pull_request_draft: false,
		changes_requested: false,
		additions: opts?.diffUrl ? 4 : 0,
		deletions: opts?.diffUrl ? 1 : 0,
		changed_files: opts?.diffUrl ? 2 : 0,
	};
	const chatWithDiffStatus: TypesGen.Chat = {
		...chat,
		diff_status: diffStatus,
	};
	return [
		{ key: chatKey(CHAT_ID), data: chatWithDiffStatus },
		{
			key: chatMessagesKey(CHAT_ID),
			data: { pages: [messagesData], pageParams: [undefined] },
		},
		{ key: chatsKey, data: [chatWithDiffStatus] },
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
		{ key: chatModelConfigs().queryKey, data: mockModelConfigs },
		{ key: mcpServerConfigsKey, data: [] },
	];
};

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
		localStorage.removeItem(RIGHT_PANEL_OPEN_KEY);
		spyOn(API, "getApiKey").mockRejectedValue(new Error("missing API key"));
		spyOn(API.experimental, "getMCPServerConfigs").mockResolvedValue([]);
		return () => localStorage.removeItem(RIGHT_PANEL_OPEN_KEY);
	},
};

export default meta;
type Story = StoryObj<typeof AgentDetailLayout>;

// ---------------------------------------------------------------------------
// Stories
// ---------------------------------------------------------------------------

/** Multi-turn conversation with rich markdown rendering: headings, tables,
 *  ordered/unordered lists, nested lists, code blocks, blockquotes,
 *  horizontal rules, inline formatting, links, images, and task lists. */
export const WithMessageHistory: Story = {
	parameters: {
		queries: buildQueries(
			{
				id: CHAT_ID,
				...baseChatFields,
				title: "Markdown rendering showcase",
				status: "completed",
			},
			{
				messages: [
					// -- Turn 1: user asks for a summary --
					{
						id: 1,
						chat_id: CHAT_ID,
						created_at: "2026-02-18T00:01:00.000Z",
						role: "user",
						content: [
							{
								type: "text",
								text: "Give me a comprehensive overview of the auth module refactor. Include tables, lists, code examples, and anything else that would help me understand the plan.",
							},
						],
					},
					// -- Turn 2: assistant with headings, lists, table, blockquote --
					{
						id: 2,
						chat_id: CHAT_ID,
						created_at: "2026-02-18T00:01:30.000Z",
						role: "assistant",
						content: [
							{
								type: "text",
								text: [
									"# Auth Module Refactor Plan",
									"",
									"## Current Problems",
									"",
									"The existing authentication module has several issues that need addressing:",
									"",
									"1. **Mixed concerns** - token validation and session management are interleaved",
									"2. **No error hierarchy** - all auth errors are treated the same",
									"3. **Duplicated middleware** - the same checks appear in three places",
									"4. **Missing observability** - no structured logging or tracing",
									"5. **Poor test coverage** - only 34% of branches tested",
									"",
									"### Impact Matrix",
									"",
									"| Component | Severity | Effort | Priority |",
									"|---|---|---|---|",
									"| Token Validation | High | Low | P0 |",
									"| Session Management | High | Medium | P0 |",
									"| Middleware Dedup | Medium | Low | P1 |",
									"| Error Hierarchy | Medium | Low | P1 |",
									"| Observability | Low | Medium | P2 |",
									"| Test Coverage | Low | High | P2 |",
									"",
									"> **Note:** The P0 items are blocking the v2 API release. We should tackle those first before moving to P1 and P2.",
									"",
									"---",
									"",
									"## Proposed Architecture",
									"",
									"The new architecture splits auth into three layers:",
									"",
									"- **Transport layer** - extracts credentials from HTTP requests",
									"  - Bearer tokens from `Authorization` header",
									"  - Session cookies from `Cookie` header",
									"  - API keys from `X-API-Key` header",
									"- **Validation layer** - verifies credentials",
									"  - JWT signature and expiration",
									"  - Session lookup in database",
									"  - API key hash comparison",
									"- **Authorization layer** - checks permissions",
									"  - Role-based access control (RBAC)",
									"  - Resource-level permissions",
									"",
									"### Error Types",
									"",
									"The new error hierarchy uses typed sentinel errors:",
									"",
									"```go",
									"var (",
									"    // ErrInvalidToken indicates a malformed or unsigned token.",
									'    ErrInvalidToken = errors.New("invalid token")',
									"",
									"    // ErrTokenExpired indicates the token's exp claim is in the past.",
									'    ErrTokenExpired = errors.New("token expired")',
									"",
									"    // ErrSessionNotFound means the session ID doesn't exist.",
									'    ErrSessionNotFound = errors.New("session not found")',
									"",
									"    // ErrInsufficientPermissions means the user lacks required roles.",
									'    ErrInsufficientPermissions = errors.New("insufficient permissions")',
									")",
									"```",
								].join("\n"),
							},
						],
					},
					// -- Turn 3: user follow-up --
					{
						id: 3,
						chat_id: CHAT_ID,
						created_at: "2026-02-18T00:02:00.000Z",
						role: "user",
						content: [
							{
								type: "text",
								text: "Can you show me the token validation code and a comparison of the old vs new approach?",
							},
						],
					},
					// -- Turn 4: assistant with code, table, nested list, task list --
					{
						id: 4,
						chat_id: CHAT_ID,
						created_at: "2026-02-18T00:02:30.000Z",
						role: "assistant",
						content: [
							{
								type: "text",
								text: [
									"## Token Validation (Refactored)",
									"",
									"Here is the new `ValidateToken` function with proper error handling and context propagation:",
									"",
									"```go",
									"func ValidateToken(ctx context.Context, token string) (*Claims, error) {",
									"    claims, err := parseToken(token)",
									"    if err != nil {",
									'        return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)',
									"    }",
									"    if claims.ExpiresAt.Before(time.Now()) {",
									"        return nil, ErrTokenExpired",
									"    }",
									"    return claims, nil",
									"}",
									"```",
									"",
									"### Old vs New Comparison",
									"",
									"| Aspect | Old Implementation | New Implementation |",
									"|---|---|---|",
									"| Error types | Generic `error` | Typed sentinel errors |",
									"| Token parsing | Inline in handler | Extracted to `parseToken()` |",
									"| Expiry check | Mixed with validation | Separate step with `ErrTokenExpired` |",
									"| Context | Not used | Passed through for tracing |",
									"| Testability | Requires HTTP server | Pure function, unit-testable |",
									"",
									"### Key Changes",
									"",
									"The refactored version improves several areas:",
									"",
									"1. **Separation of concerns**",
									"   - Parsing is isolated from expiration checking",
									"   - Each failure mode has its own error type",
									"   - Callers can match on `errors.Is(err, ErrTokenExpired)` to refresh tokens",
									"2. **Observability**",
									"   - Context carries trace spans",
									"   - Errors wrap the original cause with `%w`",
									"3. **Testing**",
									"   - Pure function with no HTTP dependency",
									"   - Table-driven tests for each error path",
									"",
									"> **Warning:** The old `ValidateAndRefresh` function is now deprecated. Callers should migrate to the new two-step pattern:",
									"> ```go",
									"> claims, err := ValidateToken(ctx, token)",
									"> if errors.Is(err, ErrTokenExpired) {",
									">     newToken, err := RefreshToken(ctx, token)",
									"> }",
									"> ```",
									"",
									"---",
									"",
									"#### Migration Checklist",
									"",
									"- [x] Extract `parseToken()` helper",
									"- [x] Define sentinel error types",
									"- [x] Add context parameter",
									"- [ ] Update all callers in `coderd/`",
									"- [ ] Add table-driven unit tests",
									"- [ ] Remove deprecated `ValidateAndRefresh`",
								].join("\n"),
							},
						],
					},
					// -- Turn 5: user asks about middleware --
					{
						id: 5,
						chat_id: CHAT_ID,
						created_at: "2026-02-18T00:03:00.000Z",
						role: "user",
						content: [
							{
								type: "text",
								text: "Now show me the middleware consolidation with the full config and some inline code references.",
							},
						],
					},
					// -- Turn 6: assistant with code, inline code, links, images, nested blockquote --
					{
						id: 6,
						chat_id: CHAT_ID,
						created_at: "2026-02-18T00:03:30.000Z",
						role: "assistant",
						content: [
							{
								type: "text",
								text: [
									"## Middleware Consolidation",
									"",
									"I consolidated the three middleware instances into a single composable chain using the *functional options* pattern. The `AuthMiddleware` function accepts variadic `AuthOption` values to customize behavior per-route.",
									"",
									"### Configuration Struct",
									"",
									"```go",
									"type authConfig struct {",
									"    // RequireAuth rejects unauthenticated requests when true.",
									"    RequireAuth   bool",
									"    // ErrorHandler is called when authentication fails.",
									"    ErrorHandler  func(http.ResponseWriter, *http.Request, error)",
									"    // AllowedRoles restricts access to specific roles.",
									"    AllowedRoles  []string",
									"    // TokenSources defines where to look for credentials.",
									"    TokenSources  []TokenSource",
									"}",
									"",
									"func defaultAuthConfig() authConfig {",
									"    return authConfig{",
									"        RequireAuth:  true,",
									"        ErrorHandler: defaultErrorHandler,",
									"        AllowedRoles: nil, // all roles",
									"        TokenSources: []TokenSource{BearerToken, SessionCookie},",
									"    }",
									"}",
									"```",
									"",
									"### Middleware Function",
									"",
									"```go",
									"func AuthMiddleware(opts ...AuthOption) func(http.Handler) http.Handler {",
									"    cfg := defaultAuthConfig()",
									"    for _, opt := range opts {",
									"        opt(&cfg)",
									"    }",
									"    return func(next http.Handler) http.Handler {",
									"        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {",
									"            claims, err := ValidateToken(r.Context(), extractToken(r))",
									"            if err != nil {",
									"                cfg.ErrorHandler(w, r, err)",
									"                return",
									"            }",
									"            ctx := context.WithValue(r.Context(), claimsKey, claims)",
									"            next.ServeHTTP(w, r.WithContext(ctx))",
									"        })",
									"    }",
									"}",
									"```",
									"",
									"### Usage Examples",
									"",
									"Here is how different routes use the middleware:",
									"",
									"| Route | Options | Behavior |",
									"|---|---|---|",
									'| `GET /api/users` | `WithRoles("admin")` | Requires admin role |',
									"| `GET /api/me` | *(defaults)* | Any authenticated user |",
									"| `GET /api/health` | `WithOptionalAuth()` | Auth is optional |",
									"| `POST /api/webhooks` | `WithAPIKeyOnly()` | Only API key auth |",
									"",
									"### Inline References",
									"",
									"The key types involved are:",
									"",
									"- `AuthOption` is a `func(*authConfig)` that mutates the config",
									"- `TokenSource` is an enum: `BearerToken`, `SessionCookie`, or `APIKey`",
									"- `Claims` holds the decoded JWT payload including `sub`, `exp`, and `roles`",
									"",
									"For more details, see the [Go middleware patterns](https://pkg.go.dev/net/http) documentation and the ~~old middleware README~~ (now removed).",
									"",
									"> **Tip:** You can compose multiple options together:",
									">",
									"> ```go",
									"> r.Use(AuthMiddleware(",
									'>     WithRoles("admin", "editor"),',
									">     WithCustomErrorHandler(jsonErrorHandler),",
									"> ))",
									"> ```",
									">",
									"> This replaces the three separate middlewares we had before.",
									"",
									"---",
									"",
									"#### Performance Benchmarks",
									"",
									"| Benchmark | Old (ns/op) | New (ns/op) | Delta |",
									"|---|---:|---:|---:|",
									"| `BenchmarkValidateToken` | 4,521 | 1,203 | -73.4% |",
									"| `BenchmarkMiddlewareChain` | 12,887 | 3,456 | -73.2% |",
									"| `BenchmarkSessionLookup` | 89,102 | 45,330 | -49.1% |",
									"| `BenchmarkFullAuthFlow` | 102,340 | 48,912 | -52.2% |",
									"",
									"The performance improvement comes mainly from:",
									"",
									"1. Removing redundant token parsing (was done 3x per request)",
									"2. Caching parsed claims in context",
									"3. Using `sync.Pool` for the JWT parser",
								].join("\n"),
							},
						],
					},
				],
				queued_messages: [],
				has_more: false,
			},
			{ diffUrl: undefined },
		),
	},
};

/** Skeleton placeholder when no query data is available yet. */ export const Loading: Story =
	{};

/** Full layout with actions menu and diff panel portaled to the right slot. */
export const CompletedWithDiffPanel: Story = {
	beforeEach: () => {
		localStorage.setItem(RIGHT_PANEL_OPEN_KEY, "true");
		return () => localStorage.removeItem(RIGHT_PANEL_OPEN_KEY);
	},
	parameters: {
		queries: buildQueries(
			{
				id: CHAT_ID,
				...baseChatFields,
				title: "Build a feature",
				status: "completed",
			},
			{ messages: [], queued_messages: [], has_more: false },
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

/** Subagent tool-call/result messages render subagent cards. */
export const WithSubagentCards: Story = {
	parameters: {
		queries: buildQueries(
			{
				id: CHAT_ID,
				...baseChatFields,
				title: "Parent agent",
				status: "running",
			},
			{
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
				has_more: false,
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

/** spawn_computer_use_agent tool renders with an "Open Desktop" button
 *  that opens the right sidebar panel and switches to the Desktop tab. */
export const WithComputerUseAgent: Story = {
	parameters: {
		queries: [
			...buildQueries(
				{
					id: CHAT_ID,
					...baseChatFields,
					title: "Desktop automation task",
					status: "running",
				},
				{
					messages: [
						{
							id: 1,
							chat_id: CHAT_ID,
							created_at: "2026-02-18T00:00:01.000Z",
							role: "user",
							content: [
								{
									type: "text",
									text: "Can you check the browser for visual regressions?",
								},
							],
						},
						{
							id: 2,
							chat_id: CHAT_ID,
							created_at: "2026-02-18T00:00:02.000Z",
							role: "assistant",
							content: [
								{
									type: "text",
									text: "I'll spawn a computer use agent to visually inspect the browser.",
								},
								{
									type: "tool-call",
									tool_call_id: "tool-desktop-1",
									tool_name: "spawn_computer_use_agent",
									args: {
										title: "Visual regression check",
										prompt:
											"Open the browser and check for visual regressions on the dashboard page.",
									},
								},
								{
									type: "tool-result",
									tool_call_id: "tool-desktop-1",
									tool_name: "spawn_computer_use_agent",
									result: {
										chat_id: "desktop-child-1",
										title: "Visual regression check",
										status: "completed",
										duration_ms: "12400",
									},
								},
								{
									type: "text",
									text: "The desktop agent has finished its visual inspection. No regressions found. You can click **Open Desktop** above to view the desktop session.",
								},
							],
						},
					],
					queued_messages: [],
					has_more: false,
				},
				{ diffUrl: undefined },
			),
			// Enable the desktop feature so the Desktop tab appears in the sidebar.
			{
				key: ["chat-desktop-enabled"],
				data: { enable_desktop: true },
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// The tool should show "Spawned ... Visual regression check".
		await waitFor(() => {
			expect(canvas.getByText(/Visual regression check/)).toBeInTheDocument();
		});
	},
};

/** Completed reasoning part renders inline. */
export const WithReasoningInline: Story = {
	parameters: {
		queries: buildQueries(
			{
				id: CHAT_ID,
				...baseChatFields,
				title: "Reasoning title",
				status: "completed",
			},
			{
				messages: [
					{
						id: 1,
						chat_id: CHAT_ID,
						created_at: "2026-02-18T00:00:01.000Z",
						role: "assistant",
						content: [
							{
								type: "reasoning",
								text: "Reasoning body",
							},
						],
					},
				],
				queued_messages: [],
				has_more: false,
			},
			{ diffUrl: undefined },
		),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// Reasoning text renders inline.
		expect(canvas.getByText("Reasoning body")).toBeInTheDocument();
		expect(canvas.queryByRole("button", { name: "Thinking" })).toBeNull();
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
				id: CHAT_ID,
				...baseChatFields,
				title: "Streaming title",
				status: "running",
			},
			{ messages: [], queued_messages: [], has_more: false },
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
				id: CHAT_ID,
				...baseChatFields,
				title: "Full sidebar demo",
				status: "completed",
			},
			{ messages: [], queued_messages: [], has_more: false },
			{ diffUrl: "https://github.com/coder/coder/pull/456" },
		),
		webSocket: {
			"/stream/git": [
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
				id: CHAT_ID,
				...baseChatFields,
				title: "Single repo sidebar",
				status: "completed",
			},
			{ messages: [], queued_messages: [], has_more: false },
			{ diffUrl: undefined },
		),
		webSocket: {
			"/stream/git": [
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
 * Streaming reasoning part via WebSocket — renders inline text.
 */
export const StreamedReasoning: Story = {
	parameters: {
		queries: buildQueries(
			{
				id: CHAT_ID,
				...baseChatFields,
				title: "Streaming reasoning title",
				status: "running",
			},
			{ messages: [], queued_messages: [], has_more: false },
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

		await expect(
			canvas.findByText("Streaming reasoning body"),
		).resolves.toBeInTheDocument();
	},
};

/**
 * Validates that text currently being streamed via WebSocket is not lost
 * when the user sends a follow-up message and the server responds with a
 * queued acknowledgement. The streaming content must remain visible in the
 * DOM after the send completes.
 */
export const QueuedSendWithActiveStream: Story = {
	beforeEach: () => {
		const spy = spyOn(API.experimental, "createChatMessage").mockResolvedValue({
			queued: true,
			queued_message: {
				id: 99,
				chat_id: CHAT_ID,
				created_at: "2026-02-18T00:00:02.000Z",
				content: [{ type: "text", text: "follow-up" }],
			},
		});
		return () => spy.mockRestore();
	},
	parameters: {
		queries: buildQueries(
			{
				id: CHAT_ID,
				...baseChatFields,
				title: "Streaming survives queued send",
				status: "running",
			},
			{ messages: [], queued_messages: [], has_more: false },
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
								type: "text",
								text: "I am helping you with the implementation",
							},
						},
					}),
				},
			],
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// Wait for the streamed text to appear.
		await expect(
			canvas.findByText("I am helping you with the implementation"),
		).resolves.toBeInTheDocument();

		// Type a follow-up message and send it.
		const textbox = canvas.getByRole("textbox");
		await userEvent.type(textbox, "follow-up");
		await userEvent.keyboard("{Enter}");

		// Verify the send actually fired (guards against the test
		// passing trivially if a future change blocks the send).
		await waitFor(() => {
			expect(API.experimental.createChatMessage).toHaveBeenCalledTimes(1);
		});

		// After the queued send, the streaming text must still be visible.
		expect(
			canvas.getByText("I am helping you with the implementation"),
		).toBeInTheDocument();
	},
};

/**
 * Validates that a failed POST during an active stream does not wipe
 * the streaming output. The catch block re-throws before reaching
 * clearStreamState(), so the in-progress text must survive.
 */
export const FailedSendWithActiveStream: Story = {
	beforeEach: () => {
		const spy = spyOn(API.experimental, "createChatMessage").mockRejectedValue(
			new Error("network error"),
		);
		return () => spy.mockRestore();
	},
	parameters: {
		queries: buildQueries(
			{
				id: CHAT_ID,
				...baseChatFields,
				title: "Failed send preserves stream",
				status: "running",
			},
			{ messages: [], queued_messages: [], has_more: false },
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
								type: "text",
								text: "I am helping you with the implementation",
							},
						},
					}),
				},
			],
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// Wait for the streamed text to appear.
		await expect(
			canvas.findByText("I am helping you with the implementation"),
		).resolves.toBeInTheDocument();

		// Type a message and send it (the POST will reject).
		const textbox = canvas.getByRole("textbox");
		await userEvent.type(textbox, "this will fail");
		await userEvent.keyboard("{Enter}");

		// Verify the send was attempted.
		await waitFor(() => {
			expect(API.experimental.createChatMessage).toHaveBeenCalledTimes(1);
		});

		// The streaming text must survive the failed send.
		expect(
			canvas.getByText("I am helping you with the implementation"),
		).toBeInTheDocument();
	},
};

/** wait_agent for a computer-use subagent renders the VNC preview card
 *  (SubagentTool with computer-use variant) instead of the plain SubagentTool card. */
export const WithWaitAgentComputerUseVNC: Story = {
	parameters: {
		queries: [
			...buildQueries(
				{
					id: CHAT_ID,
					...baseChatFields,
					title: "Wait agent computer use",
					status: "running",
				},
				{
					messages: [
						{
							id: 1,
							chat_id: CHAT_ID,
							created_at: "2026-02-18T00:00:01.000Z",
							role: "assistant",
							content: [
								{
									type: "tool-call",
									tool_call_id: "tool-spawn-desktop",
									tool_name: "spawn_computer_use_agent",
									args: {
										title: "Visual check",
										prompt: "Check the browser.",
									},
								},
								{
									type: "tool-result",
									tool_call_id: "tool-spawn-desktop",
									tool_name: "spawn_computer_use_agent",
									result: {
										chat_id: "desktop-child-1",
										title: "Visual check",
										status: "completed",
									},
								},
							],
						},
					],
					queued_messages: [],
					has_more: false,
				},
				{ diffUrl: undefined },
			),
			{
				key: ["chat-desktop-enabled"],
				data: { enable_desktop: true },
			},
		],
		// The wait_agent arrives via WebSocket so it renders in
		// the streaming/running state (no tool-result yet).
		webSocket: {
			"/chats/": [
				{
					event: "message",
					data: wrapSSE({
						type: "message_part",
						chat_id: CHAT_ID,
						message_part: {
							part: {
								type: "tool-call",
								tool_call_id: "tool-wait-desktop",
								tool_name: "wait_agent",
								args_delta: '{"chat_id":"desktop-child-1"}',
							},
						},
					}),
				},
			],
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// The wait_agent card should show "Waiting for" (running state)
		// rendered via SubagentTool with VNC preview.
		await waitFor(() => {
			expect(canvas.getByText(/Waiting for/)).toBeInTheDocument();
		});
	},
};
