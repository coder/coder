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
	MockUserMember,
	MockUserOwner,
	MockWorkspace,
} from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
	withProxyProvider,
	withWebSocket,
} from "#/testHelpers/storybook";
import AgentChatPage, { RIGHT_PANEL_OPEN_KEY } from "./AgentChatPage";
import type { AgentsOutletContext } from "./AgentsPage";

// ---------------------------------------------------------------------------
// Layout wrapper – provides outlet context for the child route.
// ---------------------------------------------------------------------------
const AgentChatPageLayout: FC = () => {
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
							regeneratingTitleChatIds: [],
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
				agents: [],
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
	organization_id: "test-org-id",
	owner_id: MockUserOwner.id,
	workspace_id: mockWorkspace.id,
	last_model_config_id: MODEL_CONFIG_ID,
	mcp_server_ids: [],
	labels: {},
	created_at: "2026-02-18T00:00:00.000Z",
	updated_at: "2026-02-18T00:00:00.000Z",
	archived: false,
	pin_order: 0,
	has_unread: false,
	client_type: "ui",
	children: [],
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

// ---------------------------------------------------------------------------
// Every-tool showcase: a single completed assistant turn that exercises
// every tool renderer registered in Tool.tsx, plus the SubagentRenderer
// variants and the generic MCP fallback. Used by WithEveryTool
// so the page demonstrates every tool card type at once.
//
// The generated `ChatToolCallPart`/`ChatToolResultPart` types declare
// `args`/`result` as `Record<string, string>`, but the runtime accepts
// arbitrary JSON. We cast through `unknown` here so the showcase can
// use realistic numbers, booleans, arrays, and nested objects without
// stringifying every field.
// ---------------------------------------------------------------------------

const EVERY_TOOL_ASSISTANT_TURN = {
	id: 4,
	chat_id: CHAT_ID,
	created_at: "2026-02-18T00:00:04.000Z",
	role: "assistant",
	content: [
		{
			type: "text",
			text: "Here is a recap of every tool I have at my disposal, exercised against this workspace so each card type renders.",
		},

		// execute -- shell command output
		{
			type: "tool-call",
			tool_call_id: "every-execute",
			tool_name: "execute",
			args: { command: "go test ./coderd/httpmw/..." },
		},
		{
			type: "tool-result",
			tool_call_id: "every-execute",
			tool_name: "execute",
			result: {
				output: [
					"ok  \tgithub.com/coder/coder/coderd/httpmw\t1.842s",
					"ok  \tgithub.com/coder/coder/coderd/httpmw/auth\t0.612s",
				].join("\n"),
				exit_code: 0,
			},
		},

		// process_output -- background process output
		{
			type: "tool-call",
			tool_call_id: "every-process-output",
			tool_name: "process_output",
			args: { process_id: "proc-42" },
		},
		{
			type: "tool-result",
			tool_call_id: "every-process-output",
			tool_name: "process_output",
			result: {
				output: [
					"[server] listening on :3000",
					"[server] handled GET / 200 in 4ms",
					"[server] handled GET /api/v2/users 200 in 12ms",
				].join("\n"),
				exit_code: null,
			},
		},

		// process_signal -- signal sent to a background process
		{
			type: "tool-call",
			tool_call_id: "every-process-signal",
			tool_name: "process_signal",
			args: { process_id: "proc-42", signal: "terminate" },
		},
		{
			type: "tool-result",
			tool_call_id: "every-process-signal",
			tool_name: "process_signal",
			result: { success: true, signal: "terminate" },
		},

		// read_file -- completed file read with content viewer
		{
			type: "tool-call",
			tool_call_id: "every-read-file",
			tool_name: "read_file",
			args: { path: "coderd/httpmw/apikey.go" },
		},
		{
			type: "tool-result",
			tool_call_id: "every-read-file",
			tool_name: "read_file",
			result: {
				content: [
					"package httpmw",
					"",
					"// ExtractAPIKeyMW will be split into a dedicated auth",
					"// package with three layers: transport, validation, authz.",
					"func ExtractAPIKeyMW(/* ... */) {}",
				].join("\n"),
			},
		},

		// write_file -- completed file write with diff viewer
		{
			type: "tool-call",
			tool_call_id: "every-write-file",
			tool_name: "write_file",
			args: {
				path: "coderd/httpmw/auth/transport.go",
				content: [
					"package auth",
					"",
					"// ExtractCredentials reads credentials from the request.",
					"func ExtractCredentials(r *http.Request) (Credential, error) {",
					"    return Credential{}, nil",
					"}",
				].join("\n"),
			},
		},
		{
			type: "tool-result",
			tool_call_id: "every-write-file",
			tool_name: "write_file",
			result: { content: "wrote 6 lines" },
		},

		// edit_files -- completed multi-file edit
		{
			type: "tool-call",
			tool_call_id: "every-edit-files",
			tool_name: "edit_files",
			args: {
				files: [
					{
						path: "coderd/coderd.go",
						edits: [
							{
								search: "httpmw.ExtractAPIKeyMW(opts)",
								replace: "auth.Middleware(opts)",
							},
						],
					},
				],
			},
		},
		{
			type: "tool-result",
			tool_call_id: "every-edit-files",
			tool_name: "edit_files",
			result: { applied: 1 },
		},

		// list_templates -- completed template listing
		{
			type: "tool-call",
			tool_call_id: "every-list-templates",
			tool_name: "list_templates",
			args: {},
		},
		{
			type: "tool-result",
			tool_call_id: "every-list-templates",
			tool_name: "list_templates",
			result: {
				templates: [
					{
						id: "tpl-go",
						name: "go-template",
						display_name: "Go Development",
						description: "Workspace for Go services with VS Code.",
					},
					{
						id: "tpl-py",
						name: "python-template",
						description: "Python development environment.",
					},
				],
				count: 2,
			},
		},

		// read_template -- completed single-template read
		{
			type: "tool-call",
			tool_call_id: "every-read-template",
			tool_name: "read_template",
			args: { name: "go-template" },
		},
		{
			type: "tool-result",
			tool_call_id: "every-read-template",
			tool_name: "read_template",
			result: {
				template: { name: "go-template", display_name: "Go Development" },
			},
		},

		// read_skill -- completed skill load
		{
			type: "tool-call",
			tool_call_id: "every-read-skill",
			tool_name: "read_skill",
			args: { name: "deep-review" },
		},
		{
			type: "tool-result",
			tool_call_id: "every-read-skill",
			tool_name: "read_skill",
			result: {
				name: "deep-review",
				body: [
					"## Deep Review Skill",
					"",
					"Review the code changes thoroughly:",
					"",
					"1. Check for correctness",
					"2. Verify tests cover the new branches",
					"3. Ensure style consistency",
				].join("\n"),
				files: ["roles/security-reviewer.md"],
			},
		},

		// read_skill_file -- completed skill file fetch
		{
			type: "tool-call",
			tool_call_id: "every-read-skill-file",
			tool_name: "read_skill_file",
			args: { name: "deep-review", path: "roles/security-reviewer.md" },
		},
		{
			type: "tool-result",
			tool_call_id: "every-read-skill-file",
			tool_name: "read_skill_file",
			result: {
				content: [
					"# Security Reviewer Role",
					"",
					"Focus on authentication, authorization, and input validation.",
				].join("\n"),
			},
		},

		// chat_summarized -- compaction summary card
		{
			type: "tool-call",
			tool_call_id: "every-summarized",
			tool_name: "chat_summarized",
			args: {},
		},
		{
			type: "tool-result",
			tool_call_id: "every-summarized",
			tool_name: "chat_summarized",
			result: {
				summary: [
					"Summarized the previous turns to free up context: the agent",
					"split the auth middleware into transport and validation layers",
					"and is about to wire up the call sites.",
				].join(" "),
			},
		},

		// ask_user_question -- completed clarification
		{
			type: "tool-call",
			tool_call_id: "every-ask-user",
			tool_name: "ask_user_question",
			args: {
				questions: [
					{
						header: "Migration approach",
						question:
							"How should we structure the database migration for the auth split?",
						options: [
							{
								label: "Single migration",
								description: "One migration with all changes.",
							},
							{
								label: "Incremental migrations",
								description: "Multiple sequential migrations.",
							},
						],
					},
				],
			},
		},
		{
			type: "tool-result",
			tool_call_id: "every-ask-user",
			tool_name: "ask_user_question",
			result: { questions: [{ answer: "Incremental migrations" }] },
		},

		// propose_plan -- proposed plan with content
		{
			type: "tool-call",
			tool_call_id: "every-propose-plan",
			tool_name: "propose_plan",
			args: { path: "/home/coder/.coder/plans/AUTH_SPLIT.md" },
		},
		{
			type: "tool-result",
			tool_call_id: "every-propose-plan",
			tool_name: "propose_plan",
			result: {
				file_id: "plan-file-1",
				content: [
					"# Auth Split Plan",
					"",
					"1. Carve transport, validation, and authz packages.",
					"2. Update call sites in coderd and enterprise/coderd.",
					"3. Add an incremental migration for session lookups.",
					"4. Refresh the changelog and run the suite.",
				].join("\n"),
			},
		},

		// computer -- screenshot tool result with text fallback
		{
			type: "tool-call",
			tool_call_id: "every-computer",
			tool_name: "computer",
			args: { action: "screenshot" },
		},
		{
			type: "tool-result",
			tool_call_id: "every-computer",
			tool_name: "computer",
			result: {
				data: "",
				text: "Screen resolution: 1920x1080\nActive window: Terminal",
				mime_type: "image/png",
			},
		},

		// attach_file -- generic renderer with explicit attach label
		{
			type: "tool-call",
			tool_call_id: "every-attach-file",
			tool_name: "attach_file",
			args: { path: "docs/runbooks/auth-split.md" },
		},
		{
			type: "tool-result",
			tool_call_id: "every-attach-file",
			tool_name: "attach_file",
			result: {},
		},

		// generic / MCP fallback -- unknown tool name with no server
		{
			type: "tool-call",
			tool_call_id: "every-generic",
			tool_name: "web_search",
			args: { query: "OAuth2 token rotation strategies" },
		},
		{
			type: "tool-result",
			tool_call_id: "every-generic",
			tool_name: "web_search",
			result: {
				results: [
					{
						title: "OAuth 2.0 RFC 6749",
						url: "https://datatracker.ietf.org/doc/html/rfc6749",
					},
				],
			},
		},

		// spawn_agent (general subagent variant)
		{
			type: "tool-call",
			tool_call_id: "every-spawn-general",
			tool_name: "spawn_agent",
			args: {
				type: "general",
				title: "Workspace diagnostics",
				prompt: "Collect logs and summarize why startup failed.",
			},
		},
		{
			type: "tool-result",
			tool_call_id: "every-spawn-general",
			tool_name: "spawn_agent",
			result: {
				chat_id: "every-general-child",
				type: "general",
				title: "Workspace diagnostics",
				status: "completed",
				duration_ms: 3200,
			},
		},

		// spawn_explore_agent (explore subagent variant)
		{
			type: "tool-call",
			tool_call_id: "every-spawn-explore",
			tool_name: "spawn_explore_agent",
			args: {
				prompt: "Read the repo and summarize the auth flow.",
			},
		},
		{
			type: "tool-result",
			tool_call_id: "every-spawn-explore",
			tool_name: "spawn_explore_agent",
			result: {
				chat_id: "every-explore-child",
				type: "explore",
				status: "completed",
				duration_ms: 4100,
			},
		},

		// spawn_computer_use_agent (desktop subagent variant)
		{
			type: "tool-call",
			tool_call_id: "every-spawn-desktop",
			tool_name: "spawn_computer_use_agent",
			args: {
				title: "Visual regression check",
				prompt: "Open the dashboard and look for visual regressions.",
			},
		},
		{
			type: "tool-result",
			tool_call_id: "every-spawn-desktop",
			tool_name: "spawn_computer_use_agent",
			result: {
				chat_id: "every-desktop-child",
				type: "computer_use",
				title: "Visual regression check",
				status: "completed",
				duration_ms: 12400,
			},
		},

		// message_agent -- send a follow-up to a subagent
		{
			type: "tool-call",
			tool_call_id: "every-message-agent",
			tool_name: "message_agent",
			args: { chat_id: "every-general-child", message: "continue" },
		},
		{
			type: "tool-result",
			tool_call_id: "every-message-agent",
			tool_name: "message_agent",
			result: {
				chat_id: "every-general-child",
				type: "general",
				status: "running",
			},
		},

		// wait_agent -- wait for a subagent to finish
		{
			type: "tool-call",
			tool_call_id: "every-wait-agent",
			tool_name: "wait_agent",
			args: { chat_id: "every-general-child" },
		},
		{
			type: "tool-result",
			tool_call_id: "every-wait-agent",
			tool_name: "wait_agent",
			result: {
				chat_id: "every-general-child",
				type: "general",
				status: "completed",
				report: "Diagnostics finished; logs were uploaded.",
			},
		},

		// close_agent -- terminate a subagent
		{
			type: "tool-call",
			tool_call_id: "every-close-agent",
			tool_name: "close_agent",
			args: { chat_id: "every-explore-child" },
		},
		{
			type: "tool-result",
			tool_call_id: "every-close-agent",
			tool_name: "close_agent",
			result: {
				chat_id: "every-explore-child",
				type: "explore",
				status: "completed",
			},
		},

		{
			type: "text",
			text: "With every renderer exercised, I'll continue the auth split below. Note the streaming flurry of file tool calls in the next turn.",
		},
	],
} as unknown as TypesGen.ChatMessage;

// ---------------------------------------------------------------------------
// Meta
// ---------------------------------------------------------------------------
const meta: Meta<typeof AgentChatPageLayout> = {
	title: "pages/AgentsPage/AgentChatPage",
	component: AgentChatPageLayout,
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
			routing: reactRouterOutlet(
				{ path: "/agents/:agentId" },
				<AgentChatPage />,
			),
		}),
	},
	beforeEach: () => {
		localStorage.removeItem(RIGHT_PANEL_OPEN_KEY);
		spyOn(API, "getApiKey").mockRejectedValue(new Error("missing API key"));
		spyOn(API.experimental, "updateChat").mockResolvedValue();
		spyOn(API.experimental, "getMCPServerConfigs").mockResolvedValue([]);
		return () => localStorage.removeItem(RIGHT_PANEL_OPEN_KEY);
	},
};

export default meta;
type Story = StoryObj<typeof AgentChatPageLayout>;

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
					// -- Turn 3: user follow-up (long message) --
					{
						id: 3,
						chat_id: CHAT_ID,
						created_at: "2026-02-18T00:02:00.000Z",
						role: "user",
						content: [
							{
								type: "text",
								text: [
									"Can you show me the token validation code and a comparison of the old vs new approach?",
									"",
									"I have a lot of context I want to share so you can give me the best possible answer.",
									"The current token validation is scattered across multiple files and it is really hard",
									"to follow the flow from HTTP request to database lookup to response. The middleware in",
									"coderd/httpmw/apikey.go does way too much - it parses the token, validates the signature,",
									"checks expiration, looks up the user, checks if the user is suspended, and then sets up",
									"the context. That is at least 6 different responsibilities in a single middleware function.",
									"",
									"Here are the specific files I have been looking at:",
									"- coderd/httpmw/apikey.go (main middleware, ~400 lines)",
									"- coderd/httpmw/oauth2.go (OAuth2 token handling)",
									"- coderd/httpmw/session.go (session cookie management)",
									"- coderd/userauth.go (login/logout handlers)",
									"- coderd/apikey.go (API key CRUD operations)",
									"- enterprise/coderd/proxyhealth.go (proxy authentication)",
									"",
									"The problem is that ExtractAPIKeyMW is doing too many things at once:",
									"1. Extracting the token from the request (cookie or header)",
									"2. Splitting the token into key ID and secret",
									"3. Looking up the API key in the database",
									"4. Hashing the secret and comparing it",
									"5. Checking if the key is expired",
									"",
									"Can you incorporate all of this into your comparison of the old vs new approach?",
								].join("\n"),
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
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			await canvas.findByText("Markdown rendering showcase"),
		).toBeVisible();
		await waitFor(() =>
			expect(
				canvas.queryByText(/^This is not your chat/),
			).not.toBeInTheDocument(),
		);
	},
};

/** Skeleton placeholder when no query data is available yet. */
export const Loading: Story = {
	parameters: {
		queries: buildQueries(
			{
				id: CHAT_ID,
				...baseChatFields,
				title: "",
				status: "running",
			},
			// An empty messages response keeps the shell visible while
			// the conversation area shows its loading skeleton.
			{ messages: [], queued_messages: [], has_more: false },
			{ diffUrl: undefined },
		),
	},
};

export const AdminViewingOtherUserChat: Story = {
	parameters: {
		queries: [
			...buildQueries(
				{
					id: CHAT_ID,
					...baseChatFields,
					owner_id: "other-user-id",
					title: "Other user's chat",
					status: "completed",
				},
				{ messages: [], queued_messages: [], has_more: false },
				{ diffUrl: undefined },
			),
			{
				key: ["user", "other-user-id"],
				data: {
					...MockUserMember,
					id: "other-user-id",
					username: "OtherUser",
				},
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const banner = await canvas.findByText(
			"This is not your chat. Prompting here will use @OtherUser's identity.",
		);
		expect(banner).toBeVisible();
		expect(banner).toHaveAttribute("role", "status");
	},
};

export const ArchivedOtherUserChat: Story = {
	parameters: {
		queries: buildQueries(
			{
				id: CHAT_ID,
				...baseChatFields,
				archived: true,
				owner_id: "other-user-id",
				title: "Archived other user's chat",
				status: "completed",
			},
			{ messages: [], queued_messages: [], has_more: false },
			{ diffUrl: undefined },
		),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			await canvas.findByText("This agent has been archived and is read-only."),
		).toBeVisible();
		expect(
			canvas.queryByText(/^This is not your chat/),
		).not.toBeInTheDocument();
	},
};

/** Persisted structured errors rehydrate the failed callout after refresh. */
export const PersistedStructuredError: Story = {
	parameters: {
		queries: buildQueries(
			{
				id: CHAT_ID,
				...baseChatFields,
				title: "Persisted provider error",
				status: "error",
				last_error: {
					message: "Anthropic returned an unexpected error.",
					detail:
						"messages.0.content.1.image.source.base64: image exceeds 5 MB maximum.",
					kind: "generic",
					provider: "anthropic",
					retryable: false,
					status_code: 400,
				},
			},
			{ messages: [], queued_messages: [], has_more: false },
			{ diffUrl: undefined },
		),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("heading", { name: /request failed/i }),
		).toBeVisible();
		expect(
			canvas.getByText(/anthropic returned an unexpected error\./i),
		).toBeVisible();
		expect(canvas.getByText(/^HTTP 400$/)).toBeVisible();
		expect(canvas.getByText(/image exceeds 5 mb maximum/i)).toBeVisible();
	},
};

export const PlanModeFromChatState: Story = {
	parameters: {
		queries: buildQueries(
			{
				id: CHAT_ID,
				...baseChatFields,
				title: "Plan mode persists",
				status: "completed",
				plan_mode: "plan",
			},
			{ messages: [], queued_messages: [], has_more: false },
			{ diffUrl: undefined },
		),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		const user = userEvent.setup();

		expect(await canvas.findByText("Planning")).toBeVisible();

		await user.click(canvas.getByRole("button", { name: "More options" }));
		await body.findByRole("dialog");
		const toggles = await body.findAllByRole("menuitemcheckbox", {
			name: "Plan first",
		});
		const toggle = toggles.at(-1);
		if (!toggle) {
			throw new Error("Plan mode toggle did not render.");
		}
		expect(toggle).toHaveAttribute("aria-checked", "true");
		await user.click(toggle);

		await waitFor(() => {
			expect(canvas.queryByText("Planning")).not.toBeInTheDocument();
		});
	},
};

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
			expect(body.getByText("Archive Agent")).toBeInTheDocument();
		});
		// Workspace items moved to the workspace pill popover.
		expect(body.queryByText("Open in Cursor")).not.toBeInTheDocument();
		expect(body.queryByText("Open in VS Code")).not.toBeInTheDocument();
		expect(body.queryByText("View Workspace")).not.toBeInTheDocument();
		expect(body.queryByText("Copy SSH Command")).not.toBeInTheDocument();
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

export const WithMixedSubagentTranscript: Story = {
	parameters: {
		queries: buildQueries(
			{
				id: CHAT_ID,
				...baseChatFields,
				title: "Mixed subagent transcript",
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
								type: "tool-call",
								tool_call_id: "legacy-spawn",
								tool_name: "spawn_agent",
								args: { title: "Legacy helper" },
							},
							{
								type: "tool-result",
								tool_call_id: "legacy-spawn",
								tool_name: "spawn_agent",
								result: {
									chat_id: "legacy-child",
									title: "Legacy helper",
									status: "completed",
								},
							},
							{
								type: "tool-call",
								tool_call_id: "unified-spawn",
								tool_name: "spawn_agent",
								args: { type: "explore" },
							},
							{
								type: "tool-result",
								tool_call_id: "unified-spawn",
								tool_name: "spawn_agent",
								result: {
									chat_id: "explore-child",
									type: "explore",
									status: "completed",
								},
							},
							{
								type: "tool-call",
								tool_call_id: "unified-wait",
								tool_name: "wait_agent",
								args: { chat_id: "explore-child" },
							},
							{
								type: "tool-result",
								tool_call_id: "unified-wait",
								tool_name: "wait_agent",
								result: {
									chat_id: "explore-child",
									type: "explore",
									status: "completed",
								},
							},
							{
								type: "tool-call",
								tool_call_id: "legacy-close",
								tool_name: "close_agent",
								args: { chat_id: "legacy-child" },
							},
							{
								type: "tool-result",
								tool_call_id: "legacy-close",
								tool_name: "close_agent",
								result: {
									chat_id: "legacy-child",
									type: "general",
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
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			expect(
				canvas.getByText(
					(_content, element) =>
						element?.tagName === "SPAN" &&
						element.textContent?.includes("Spawned") === true &&
						element.textContent?.includes("Legacy helper") === true,
				),
			).toBeInTheDocument();
			expect(
				canvas.getAllByText(/Legacy helper/).length,
			).toBeGreaterThanOrEqual(2);
			expect(
				canvas.getByText(
					(_content, element) =>
						element?.tagName === "SPAN" &&
						element.textContent?.includes("Spawned") === true &&
						element.textContent?.includes("Explore agent") === true,
				),
			).toBeInTheDocument();
			expect(
				canvas.getByText(
					(_content, element) =>
						element?.tagName === "SPAN" &&
						element.textContent?.includes("Waited for") === true &&
						element.textContent?.includes("Explore agent") === true,
				),
			).toBeInTheDocument();
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

		// Reasoning renders inside a collapsible disclosure.
		const trigger = canvas.getByRole("button", { name: "Thinking" });
		expect(trigger).toBeInTheDocument();
		await userEvent.click(trigger);
		await waitFor(() => {
			expect(canvas.getByText("Reasoning body")).toBeVisible();
		});
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
					data: JSON.stringify([
						{
							type: "message_part",
							chat_id: CHAT_ID,
							message_part: {
								part: {
									type: "tool-call",
									tool_call_id: "tool-subagent-stream-1",
									tool_name: "spawn_agent",
									args_delta: '{"title":"Streamed Child"',
								},
							},
						},
					] satisfies TypesGen.ChatStreamEvent[]),
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
					data: JSON.stringify([
						{
							type: "message_part",
							chat_id: CHAT_ID,
							message_part: {
								part: {
									type: "reasoning",
									text: "Streaming reasoning body",
								},
							},
						},
					] satisfies TypesGen.ChatStreamEvent[]),
				},
			],
		},
	},
};

// NOTE: QueuedSendWithActiveStream and FailedSendWithActiveStream
// were removed. They relied on the Storybook WebSocket mock
// delivering streamed message_part events, but the mock fires via
// setTimeout(0) which resolves before the chat store subscribes.
// This made the stories render empty chats and fail interaction
// tests in both local and CI environments.

/**
 * Live agent turn with streaming reasoning and a back-to-back flurry of
 * in-progress file tool calls. The persisted history establishes context
 * (an earlier completed read+write pair); the WebSocket then streams an
 * assistant turn with reasoning followed by read/write/edit/read/write
 * tool calls that intentionally have no matching tool-result parts, so
 * each card stays in the "running" state and shows its loading spinner.
 */
export const WithEveryTool: Story = {
	parameters: {
		queries: buildQueries(
			{
				id: CHAT_ID,
				...baseChatFields,
				title: "Refactoring the auth module",
				status: "running",
			},
			{
				messages: [
					// -- Turn 1: user kicks off the task --
					{
						id: 1,
						chat_id: CHAT_ID,
						created_at: "2026-02-18T00:00:01.000Z",
						role: "user",
						content: [
							{
								type: "text",
								text: "Refactor the auth module: split httpmw/apikey.go into a transport, validation, and authorization layer. Update the imports and add a CHANGELOG entry while you are at it.",
							},
						],
					},
					// -- Turn 2: previous assistant turn (completed) --
					//    Establishes that the agent already inspected and patched
					//    a couple of files before the streaming turn begins.
					{
						id: 2,
						chat_id: CHAT_ID,
						created_at: "2026-02-18T00:00:02.000Z",
						role: "assistant",
						content: [
							{
								type: "reasoning",
								text: "I'll start by reading the existing middleware to understand its responsibilities, then sketch out the three-layer split before touching anything else.",
							},
							{
								type: "text",
								text: "Reading the existing middleware first so I can plan the split.",
							},
							{
								type: "tool-call",
								tool_call_id: "call-read-apikey",
								tool_name: "read_file",
								args: { path: "coderd/httpmw/apikey.go" },
							},
							{
								type: "tool-result",
								tool_call_id: "call-read-apikey",
								tool_name: "read_file",
								result: {
									content: [
										"package httpmw",
										"",
										"// ExtractAPIKeyMW does too many things: it parses",
										"// the token, validates the signature, looks up the",
										"// session, and authorizes the user. We will split",
										"// these into transport, validation, and authz.",
										"func ExtractAPIKeyMW(/* ... */) {}",
									].join("\n"),
								},
							},
							{
								type: "tool-call",
								tool_call_id: "call-write-transport",
								tool_name: "write_file",
								args: {
									path: "coderd/httpmw/auth/transport.go",
									content: [
										"package auth",
										"",
										"// ExtractCredentials pulls the credential out of the",
										"// HTTP request without doing any validation.",
										"func ExtractCredentials(r *http.Request) (Credential, error) {",
										'    if h := r.Header.Get("Authorization"); h != "" {',
										"        return Credential{Kind: KindBearer, Value: h}, nil",
										"    }",
										'    if c, err := r.Cookie("coder_session_token"); err == nil {',
										"        return Credential{Kind: KindCookie, Value: c.Value}, nil",
										"    }",
										"    return Credential{}, ErrNoCredential",
										"}",
									].join("\n"),
								},
							},
							{
								type: "tool-result",
								tool_call_id: "call-write-transport",
								tool_name: "write_file",
								result: { content: "wrote 12 lines" },
							},
							{
								type: "text",
								text: "Transport layer is in place. Next I'll carve out validation and authorization, update the call sites, and bump the changelog.",
							},
						],
					},
					// -- Turn 3: user asks for a tool-by-tool tour --
					{
						id: 3,
						chat_id: CHAT_ID,
						created_at: "2026-02-18T00:00:03.000Z",
						role: "user",
						content: [
							{
								type: "text",
								text: "Before you keep going, can you take one quick pass and exercise every tool you have so I can confirm each one renders the way I expect?",
							},
						],
					},
					// -- Turn 4: assistant runs every tool exactly once --
					EVERY_TOOL_ASSISTANT_TURN,
				],
				queued_messages: [],
				has_more: false,
			},
			{ diffUrl: undefined },
		),
		// The streaming turn arrives over the WebSocket. None of the
		// tool-call parts have a matching tool-result, so each card
		// renders in the "running" state with its spinner.
		webSocket: {
			"/chats/": [
				{
					event: "message",
					data: JSON.stringify([
						// Streaming reasoning, delivered in two deltas so the
						// thinking block grows as the turn unfolds.
						{
							type: "message_part",
							chat_id: CHAT_ID,
							message_part: {
								part: {
									type: "reasoning",
									text: "Now I'll pull in the validation file and start carving out the JWT and session checks. ",
								},
							},
						},
						{
							type: "message_part",
							chat_id: CHAT_ID,
							message_part: {
								part: {
									type: "reasoning",
									text: "After that I need to wire the new package into the existing call sites and refresh the changelog so reviewers can follow the split.",
								},
							},
						},
						// Some streaming response text before the tool calls.
						{
							type: "message_part",
							chat_id: CHAT_ID,
							message_part: {
								part: {
									type: "text",
									text: "Reading the validation helpers, then writing the new package, patching the consumers, and updating the changelog.",
								},
							},
						},
						// 1) read_file - in progress.
						{
							type: "message_part",
							chat_id: CHAT_ID,
							message_part: {
								part: {
									type: "tool-call",
									tool_call_id: "stream-read-validate",
									tool_name: "read_file",
									args_delta: JSON.stringify({
										path: "coderd/httpmw/validate.go",
									}),
								},
							},
						},
						// 2) write_file - in progress.
						{
							type: "message_part",
							chat_id: CHAT_ID,
							message_part: {
								part: {
									type: "tool-call",
									tool_call_id: "stream-write-validation",
									tool_name: "write_file",
									args_delta: JSON.stringify({
										path: "coderd/httpmw/auth/validation.go",
										content: [
											"package auth",
											"",
											"// Validate verifies the supplied credential and",
											"// returns the resolved subject. It does not make",
											"// authorization decisions.",
											"func Validate(ctx context.Context, c Credential) (Subject, error) {",
											"    switch c.Kind {",
											"    case KindBearer:",
											"        return validateBearer(ctx, c.Value)",
											"    case KindCookie:",
											"        return validateSession(ctx, c.Value)",
											"    default:",
											"        return Subject{}, ErrUnsupportedCredential",
											"    }",
											"}",
										].join("\n"),
									}),
								},
							},
						},
						// 3) edit_files - in progress (multi-file).
						{
							type: "message_part",
							chat_id: CHAT_ID,
							message_part: {
								part: {
									type: "tool-call",
									tool_call_id: "stream-edit-callsites",
									tool_name: "edit_files",
									args_delta: JSON.stringify({
										files: [
											{
												path: "coderd/coderd.go",
												edits: [
													{
														search: "httpmw.ExtractAPIKeyMW(opts)",
														replace: "auth.Middleware(opts)",
													},
												],
											},
											{
												path: "enterprise/coderd/coderd.go",
												edits: [
													{
														search: "httpmw.ExtractAPIKeyMW(opts)",
														replace: "auth.Middleware(opts)",
													},
												],
											},
										],
									}),
								},
							},
						},
						// 4) read_file - in progress.
						{
							type: "message_part",
							chat_id: CHAT_ID,
							message_part: {
								part: {
									type: "tool-call",
									tool_call_id: "stream-read-changelog",
									tool_name: "read_file",
									args_delta: JSON.stringify({
										path: "CHANGELOG.md",
									}),
								},
							},
						},
						// 5) write_file - in progress.
						{
							type: "message_part",
							chat_id: CHAT_ID,
							message_part: {
								part: {
									type: "tool-call",
									tool_call_id: "stream-write-changelog",
									tool_name: "write_file",
									args_delta: JSON.stringify({
										path: "CHANGELOG.md",
										content: [
											"# Changelog",
											"",
											"## Unreleased",
											"",
											"- Split the auth middleware into transport, validation,",
											"  and authorization layers under coderd/httpmw/auth.",
										].join("\n"),
									}),
								},
							},
						},
					] satisfies TypesGen.ChatStreamEvent[]),
				},
			],
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// All five streamed tool calls should appear simultaneously.
		// read_file, write_file, and edit_files all switch to a
		// progressive label ("Reading" / "Writing" / "Editing")
		// while running; the spinner conveys progress.
		await waitFor(() => {
			expect(canvas.getByText(/Reading validate\.go/)).toBeInTheDocument();
			expect(canvas.getByText(/Writing validation\.go/)).toBeInTheDocument();
			expect(canvas.getByText(/Editing 2 files/)).toBeInTheDocument();
			expect(canvas.getByText(/Reading CHANGELOG\.md/)).toBeInTheDocument();
			expect(canvas.getByText(/Writing CHANGELOG\.md/)).toBeInTheDocument();
		});
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
					data: JSON.stringify([
						{
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
						},
					] satisfies TypesGen.ChatStreamEvent[]),
				},
			],
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// The wait_agent card should show "Using the computer..." (running
		// state) rendered via SubagentTool with VNC preview.
		await waitFor(() => {
			expect(canvas.getByText(/Using the computer/)).toBeInTheDocument();
		});
	},
};
