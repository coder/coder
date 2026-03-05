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

/**
 * Generate a long conversation for testing scroll-back pagination.
 * Produces alternating user/assistant messages so the windowing
 * sentinel becomes visible when the user scrolls up.
 */
const generateLongConversation = (count: number): TypesGen.ChatMessage[] => {
	const topics = [
		"Can you explain how the database connection pooling works?",
		"What's the best way to handle rate limiting in Go?",
		"How should we structure the error handling middleware?",
		"Can you review the caching strategy for workspace metadata?",
		"What changes are needed for the OAuth2 refresh token flow?",
		"How do we migrate the existing templates to the new format?",
		"Can you help debug this intermittent test failure?",
		"What's the recommended approach for WebSocket reconnection?",
		"How should we handle concurrent template imports?",
		"Can you walk me through the provisioner daemon lifecycle?",
	];
	const responses = [
		"The connection pool is managed through `pgxpool.Pool`. Each incoming request acquires a connection from the pool, and it's returned when the request handler completes. The pool size is configured via `CODER_PG_CONNECTION_URL` parameters:\n\n```go\nconfig.MaxConns = 20\nconfig.MinConns = 5\nconfig.MaxConnLifetime = 30 * time.Minute\n```\n\nThis ensures we don't exhaust database connections under load while keeping enough warm connections for typical traffic.",
		"For rate limiting, I recommend a token bucket algorithm with per-user and per-endpoint granularity:\n\n```go\ntype RateLimiter struct {\n    buckets sync.Map\n    rate    rate.Limit\n    burst   int\n}\n\nfunc (rl *RateLimiter) Allow(key string) bool {\n    v, _ := rl.buckets.LoadOrStore(key, rate.NewLimiter(rl.rate, rl.burst))\n    return v.(*rate.Limiter).Allow()\n}\n```\n\nThe key insight is separating read-heavy endpoints (higher limits) from write endpoints (stricter limits).",
		"The error handling middleware should follow a chain-of-responsibility pattern. Each middleware layer can decide to handle the error or pass it up:\n\n1. **Validation errors** → 400 with structured field errors\n2. **Authentication errors** → 401 with WWW-Authenticate header\n3. **Authorization errors** → 403 with a human-readable message\n4. **Not found** → 404\n5. **Everything else** → 500 with a correlation ID for debugging\n\nThe correlation ID is critical — it links the user-facing error to our internal logs.",
		"The caching strategy uses a two-tier approach:\n\n- **L1 (in-process)**: An LRU cache with a 5-minute TTL for workspace metadata that rarely changes (template names, owner info)\n- **L2 (Redis)**: A shared cache for cross-instance consistency with a 15-minute TTL\n\nCache invalidation happens through PostgreSQL LISTEN/NOTIFY — when a workspace is updated, all instances receive the notification and evict the stale entry.",
		"The OAuth2 refresh token flow needs these changes:\n\n1. Store refresh tokens encrypted at rest (AES-256-GCM)\n2. Implement token rotation — each refresh issues a new refresh token\n3. Add a grace period for concurrent refresh attempts\n4. Set absolute lifetime limits (30 days for refresh tokens)\n\nThe tricky part is handling race conditions when multiple tabs try to refresh simultaneously.",
		"Template migration involves three phases:\n\n**Phase 1**: Add the new schema fields alongside the old ones\n**Phase 2**: Backfill existing templates with computed values\n**Phase 3**: Drop the deprecated columns\n\nEach phase should be a separate migration to allow rollback. The backfill query needs to handle NULL values gracefully.",
		"Looking at the test failure, it appears to be a timing issue with the workspace build watcher. The test expects the build to complete within 5 seconds, but under CI load it sometimes takes longer.\n\nInstead of increasing the timeout, I'd recommend using a polling approach with exponential backoff. This makes the test both faster on average and more resilient.",
		"For WebSocket reconnection, use exponential backoff with jitter:\n\n```go\nbackoff := time.Second\nfor attempt := 0; attempt < maxRetries; attempt++ {\n    conn, err := dial(ctx, url)\n    if err == nil {\n        return conn, nil\n    }\n    jitter := time.Duration(rand.Int63n(int64(backoff / 2)))\n    time.Sleep(backoff + jitter)\n    backoff = min(backoff*2, maxBackoff)\n}\n```\n\nAlso maintain a sequence number so the server can replay missed messages on reconnect.",
		"Concurrent template imports should use a semaphore to limit parallelism:\n\n1. Acquire a slot from the semaphore (max 3 concurrent imports)\n2. Lock the template row with `SELECT FOR UPDATE SKIP LOCKED`\n3. Perform the import\n4. Release the semaphore slot\n\nThe `SKIP LOCKED` ensures we don't block — if another import is in progress for the same template, we return a 409 Conflict.",
		"The provisioner daemon lifecycle has four states:\n\n1. **Starting** → Registers with coderd, receives a unique ID\n2. **Idle** → Polling for new jobs every 5 seconds\n3. **Busy** → Executing a provisioner job (plan or apply)\n4. **Draining** → Finishing current job, accepting no new ones\n\nThe transition from Busy → Draining happens on SIGTERM. The daemon has 5 minutes to complete before being forcefully stopped.",
	];

	const messages: TypesGen.ChatMessage[] = [];
	for (let i = 0; i < count; i++) {
		const isUser = i % 2 === 0;
		const topicIdx = Math.floor(i / 2) % topics.length;
		messages.push({
			id: i + 1,
			chat_id: CHAT_ID,
			created_at: new Date(Date.UTC(2026, 1, 18, 0, i, 0)).toISOString(),
			role: isUser ? "user" : "assistant",
			content: [
				{
					type: "text",
					text: isUser ? topics[topicIdx] : responses[topicIdx],
				},
			],
		} as TypesGen.ChatMessage);
	}
	return messages;
};

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

/**
 * Long conversation (80 messages) to exercise scroll-back pagination.
 * The default pageSize is 50, so scrolling up will trigger the
 * IntersectionObserver sentinel and load older messages. Use this
 * story to manually verify that the viewport does not jump when
 * earlier messages are prepended.
 */
export const LongConversation: Story = {
	parameters: {
		queries: buildQueries(
			{
				chat: {
					id: CHAT_ID,
					...baseChatFields,
					title: "Long architecture discussion",
					status: "completed",
				},
				messages: generateLongConversation(80),
				queued_messages: [],
			},
			{ diffUrl: undefined },
		),
	},
};
