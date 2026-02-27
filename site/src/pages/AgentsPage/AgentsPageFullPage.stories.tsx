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
	chatModelConfigsKey,
	chatModelsKey,
	chatsKey,
} from "api/queries/chats";
import { workspaceByIdKey } from "api/queries/workspaces";
import type * as TypesGen from "api/typesGenerated";
import { spyOn } from "storybook/test";
import {
	reactRouterOutlet,
	reactRouterParameters,
} from "storybook-addon-remix-react-router";
import AgentDetail from "./AgentDetail";
import AgentsPage from "./AgentsPage";

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

const mockModelConfig: TypesGen.ChatModelConfig = {
	id: "config-openai-gpt-4o",
	provider: "openai",
	model: "gpt-4o",
	display_name: "GPT-4o",
	enabled: true,
	is_default: false,
	context_limit: 200000,
	compression_threshold: 70,
	created_at: "2026-02-18T00:00:00.000Z",
	updated_at: "2026-02-18T00:00:00.000Z",
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

const buildChat = (overrides: Partial<TypesGen.Chat> = {}): TypesGen.Chat => ({
	id: "chat-default",
	owner_id: "owner-id",
	title: "Agent",
	status: "completed",
	last_model_config_id: mockModelConfig.id,
	created_at: "2026-02-18T00:00:00.000Z",
	updated_at: "2026-02-18T00:00:00.000Z",
	...overrides,
});

// ---------------------------------------------------------------------------
// Meta
// ---------------------------------------------------------------------------

const meta: Meta<typeof AgentsPage> = {
	title: "pages/AgentsPage/FullPage",
	component: AgentsPage,
	decorators: [withAuthProvider, withDashboardProvider, withWebSocket],
	parameters: {
		layout: "fullscreen",
		user: MockUserOwner,
		permissions: { editDeploymentConfig: true },
		webSocket: [],
	},
	beforeEach: () => {
		localStorage.clear();
		spyOn(API, "getApiKey").mockRejectedValue(new Error("missing API key"));
		spyOn(API, "getWorkspaces").mockResolvedValue({
			workspaces: [mockWorkspace],
			count: 1,
		});
	},
};

export default meta;
type Story = StoryObj<typeof AgentsPage>;

// ---------------------------------------------------------------------------
// Stories
// ---------------------------------------------------------------------------

/** Empty state — no chats yet, shows the create-chat input. */
export const EmptyState: Story = {
	parameters: {
		queries: [
			{ key: chatsKey, data: [] },
			{ key: chatModelsKey, data: mockModelCatalog },
			{ key: chatModelConfigsKey, data: [mockModelConfig] },
		],
		reactRouter: reactRouterParameters({
			location: { path: "/agents" },
			routing: reactRouterOutlet({ path: "/agents/:agentId" }, <AgentDetail />),
		}),
	},
};

/**
 * Wrap a chat-list watch event in the JSON format that OneWayWebSocket parses.
 * AgentsPage's watchChats listener expects `{ type: "data", data: payload }`.
 */
const wrapChatListSSE = (payload: unknown): string =>
	JSON.stringify({ type: "data", data: payload });

/** Sidebar with multiple chats, viewing a completed conversation with diff panel. */
export const WithConversation: Story = {
	parameters: {
		// A WebSocket event that fires after setTimeout(0) — after the initial
		// commit — to trigger one more re-render of AgentsPage.  This allows
		// the portal in AgentDetailTopBarPortals to find the right-panel ref
		// that was attached during the previous commit.
		webSocket: [
			{
				event: "message",
				data: wrapChatListSSE({
					kind: "updated",
					chat: {
						id: CHAT_ID,
						title: "Build a feature",
						status: "completed",
						updated_at: "2026-02-18T00:00:00.000Z",
					},
				}),
			},
		],
		queries: [
			{
				key: chatsKey,
				data: [
					buildChat({
						id: CHAT_ID,
						title: "Build a feature",
						status: "completed",
					}),
					buildChat({
						id: "chat-2",
						title: "Fix login bug",
						status: "completed",
						updated_at: "2026-02-17T12:00:00.000Z",
					}),
					buildChat({
						id: "chat-3",
						title: "Add dark mode support",
						status: "completed",
						updated_at: "2026-02-16T08:00:00.000Z",
					}),
				],
			},
			{
				key: chatKey(CHAT_ID),
				data: {
					chat: buildChat({
						id: CHAT_ID,
						title: "Build a feature",
						status: "completed",
						workspace_id: mockWorkspace.id,
						workspace_agent_id: mockWorkspaceAgent.id,
					}),
					messages: [
						{
							id: 1,
							chat_id: CHAT_ID,
							created_at: "2026-02-18T00:00:01.000Z",
							role: "user",
							content: [{ type: "text", text: "Build a login page" }],
						},
						{
							id: 2,
							chat_id: CHAT_ID,
							created_at: "2026-02-18T00:00:02.000Z",
							role: "assistant",
							content: [
								{
									type: "text",
									text: 'I\'ll create a login page with email and password fields, form validation, and error handling. Let me set that up for you.\n\n```tsx\nexport function LoginPage() {\n  return (\n    <form>\n      <input type="email" placeholder="Email" />\n      <input type="password" placeholder="Password" />\n      <button type="submit">Sign in</button>\n    </form>\n  );\n}\n```\n\nThe login page is ready with basic form structure.',
								},
							],
						},
					],
					queued_messages: [],
				} satisfies TypesGen.ChatWithMessages,
			},
			{
				key: chatDiffStatusKey(CHAT_ID),
				data: {
					chat_id: CHAT_ID,
					url: "https://github.com/coder/coder/pull/123",
					changes_requested: false,
					additions: 103,
					deletions: 0,
					changed_files: 5,
				} satisfies TypesGen.ChatDiffStatus,
			},
			{
				key: chatDiffContentsKey(CHAT_ID),
				data: {
					chat_id: CHAT_ID,
					provider: "github",
					remote_origin: "https://github.com/coder/coder.git",
					branch: "feat/login-page",
					pull_request_url: "https://github.com/coder/coder/pull/123",
					diff: [
						"diff --git a/src/pages/LoginPage.tsx b/src/pages/LoginPage.tsx",
						"new file mode 100644",
						"--- /dev/null",
						"+++ b/src/pages/LoginPage.tsx",
						"@@ -0,0 +1,24 @@",
						"+import { useState } from 'react';",
						"+",
						"+export function LoginPage() {",
						"+  const [email, setEmail] = useState('');",
						"+  const [password, setPassword] = useState('');",
						"+",
						"+  const handleSubmit = (e: React.FormEvent) => {",
						"+    e.preventDefault();",
						"+    // TODO: call auth API",
						"+  };",
						"+",
						"+  return (",
						"+    <form onSubmit={handleSubmit}>",
						'+      <input type="email" value={email}',
						"+        onChange={(e) => setEmail(e.target.value)}",
						'+        placeholder="Email" />',
						'+      <input type="password" value={password}',
						"+        onChange={(e) => setPassword(e.target.value)}",
						'+        placeholder="Password" />',
						'+      <button type="submit">Sign in</button>',
						"+    </form>",
						"+  );",
						"+}",
						"diff --git a/src/pages/LoginPage.test.tsx b/src/pages/LoginPage.test.tsx",
						"new file mode 100644",
						"--- /dev/null",
						"+++ b/src/pages/LoginPage.test.tsx",
						"@@ -0,0 +1,35 @@",
						"+import { render, screen } from '@testing-library/react';",
						"+import userEvent from '@testing-library/user-event';",
						"+import { LoginPage } from './LoginPage';",
						"+",
						"+describe('LoginPage', () => {",
						"+  it('renders email and password fields', () => {",
						"+    render(<LoginPage />);",
						"+    expect(screen.getByPlaceholderText('Email')).toBeInTheDocument();",
						"+    expect(screen.getByPlaceholderText('Password')).toBeInTheDocument();",
						"+  });",
						"+",
						"+  it('renders the sign-in button', () => {",
						"+    render(<LoginPage />);",
						"+    expect(screen.getByRole('button', { name: 'Sign in' })).toBeInTheDocument();",
						"+  });",
						"+",
						"+  it('updates email field on input', async () => {",
						"+    render(<LoginPage />);",
						"+    const emailInput = screen.getByPlaceholderText('Email');",
						"+    await userEvent.type(emailInput, 'user@example.com');",
						"+    expect(emailInput).toHaveValue('user@example.com');",
						"+  });",
						"+",
						"+  it('updates password field on input', async () => {",
						"+    render(<LoginPage />);",
						"+    const passwordInput = screen.getByPlaceholderText('Password');",
						"+    await userEvent.type(passwordInput, 'secret123');",
						"+    expect(passwordInput).toHaveValue('secret123');",
						"+  });",
						"+",
						"+  it('calls preventDefault on submit', async () => {",
						"+    render(<LoginPage />);",
						"+    const button = screen.getByRole('button', { name: 'Sign in' });",
						"+    await userEvent.click(button);",
						"+  });",
						"+});",
						"diff --git a/src/router.tsx b/src/router.tsx",
						"--- a/src/router.tsx",
						"+++ b/src/router.tsx",
						"@@ -12,6 +12,7 @@",
						" import { HomePage } from './pages/HomePage';",
						" import { SettingsPage } from './pages/SettingsPage';",
						"+import { LoginPage } from './pages/LoginPage';",
						" ",
						" export const router = createBrowserRouter([",
						"   { path: '/', element: <HomePage /> },",
						"+  { path: '/login', element: <LoginPage /> },",
						"   { path: '/settings', element: <SettingsPage /> },",
						" ]);",
						"diff --git a/src/styles/login.css b/src/styles/login.css",
						"new file mode 100644",
						"--- /dev/null",
						"+++ b/src/styles/login.css",
						"@@ -0,0 +1,42 @@",
						"+.login-container {",
						"+  display: flex;",
						"+  align-items: center;",
						"+  justify-content: center;",
						"+  min-height: 100vh;",
						"+  background: var(--surface-primary);",
						"+}",
						"+",
						"+.login-form {",
						"+  display: flex;",
						"+  flex-direction: column;",
						"+  gap: 1rem;",
						"+  width: 100%;",
						"+  max-width: 400px;",
						"+  padding: 2rem;",
						"+  border-radius: 8px;",
						"+  background: var(--surface-secondary);",
						"+  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);",
						"+}",
						"+",
						"+.login-form input {",
						"+  padding: 0.75rem 1rem;",
						"+  border: 1px solid var(--border-default);",
						"+  border-radius: 6px;",
						"+  font-size: 0.875rem;",
						"+  background: var(--surface-primary);",
						"+  color: var(--content-primary);",
						"+  transition: border-color 0.15s ease;",
						"+}",
						"+",
						"+.login-form input:focus {",
						"+  outline: none;",
						"+  border-color: var(--highlight-primary);",
						"+  box-shadow: 0 0 0 2px rgba(var(--highlight-primary-rgb), 0.2);",
						"+}",
						"+",
						"+.login-form button[type='submit'] {",
						"+  padding: 0.75rem;",
						"+  border: none;",
						"+  border-radius: 6px;",
						"+  background: var(--highlight-primary);",
						"+  color: white;",
						"+  font-weight: 600;",
						"+  cursor: pointer;",
						"+}",
					].join("\n"),
				} satisfies TypesGen.ChatDiffContents,
			},
			{
				key: workspaceByIdKey(mockWorkspace.id),
				data: mockWorkspace,
			},
			{ key: chatModelsKey, data: mockModelCatalog },
			{ key: chatModelConfigsKey, data: [mockModelConfig] },
		],
		reactRouter: reactRouterParameters({
			location: {
				path: `/agents/${CHAT_ID}`,
				pathParams: { agentId: CHAT_ID },
			},
			routing: reactRouterOutlet({ path: "/agents/:agentId" }, <AgentDetail />),
		}),
	},
};

/** A chat tree with parent and child agents. */
export const WithChildAgents: Story = {
	parameters: {
		queries: [
			{
				key: chatsKey,
				data: [
					buildChat({
						id: "root-1",
						title: "Root planner",
						status: "running",
					}),
					buildChat({
						id: "child-1",
						title: "Frontend executor",
						status: "running",
						parent_chat_id: "root-1",
						root_chat_id: "root-1",
					}),
					buildChat({
						id: "child-2",
						title: "Backend executor",
						status: "completed",
						parent_chat_id: "root-1",
						root_chat_id: "root-1",
					}),
				],
			},
			{
				key: chatKey("child-1"),
				data: {
					chat: buildChat({
						id: "child-1",
						title: "Frontend executor",
						status: "running",
						parent_chat_id: "root-1",
						root_chat_id: "root-1",
						workspace_id: mockWorkspace.id,
						workspace_agent_id: mockWorkspaceAgent.id,
					}),
					messages: [
						{
							id: 1,
							chat_id: "child-1",
							created_at: "2026-02-18T00:01:00.000Z",
							role: "user",
							content: [
								{
									type: "text",
									text: "Implement the React components for the dashboard.",
								},
							],
						},
						{
							id: 2,
							chat_id: "child-1",
							created_at: "2026-02-18T00:01:05.000Z",
							role: "assistant",
							content: [
								{
									type: "text",
									text: "Working on the dashboard components now. I'll create the layout, sidebar, and main content area.",
								},
							],
						},
					],
					queued_messages: [],
				} satisfies TypesGen.ChatWithMessages,
			},
			{
				key: chatDiffStatusKey("child-1"),
				data: {
					chat_id: "child-1",
					url: undefined,
					changes_requested: false,
					additions: 0,
					deletions: 0,
					changed_files: 0,
				} satisfies TypesGen.ChatDiffStatus,
			},
			{
				key: workspaceByIdKey(mockWorkspace.id),
				data: mockWorkspace,
			},
			{ key: chatModelsKey, data: mockModelCatalog },
			{ key: chatModelConfigsKey, data: [mockModelConfig] },
		],
		reactRouter: reactRouterParameters({
			location: {
				path: "/agents/child-1",
				pathParams: { agentId: "child-1" },
			},
			routing: reactRouterOutlet({ path: "/agents/:agentId" }, <AgentDetail />),
		}),
	},
};
