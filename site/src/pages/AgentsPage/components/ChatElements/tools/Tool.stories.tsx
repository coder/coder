import type { Meta, StoryObj } from "@storybook/react-vite";
import {
	expect,
	fn,
	screen,
	spyOn,
	userEvent,
	waitFor,
	within,
} from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { chatModelConfigsKey } from "#/api/queries/chats";
import { MockChatModelConfig } from "#/testHelpers/chatModels";
import { ChatWorkspaceContext } from "../../../context/ChatWorkspaceContext";
import { BlockList } from "../../ChatConversation/ConversationTimeline";
import { DesktopPanelContext } from "./DesktopPanelContext";
import { Tool } from "./Tool";

const executeCommand = "git fetch origin";
const executeIntentCommand = "npm test";
const longExecuteCommand =
	"docker build --no-cache --build-arg NODE_ENV=production --build-arg API_URL=https://coder.example.com/api --build-arg SENTRY_DSN=https://example.com/sentry --build-arg FEATURE_FLAGS=agents,shell-tools --tag coder-agent:latest .";

// 1x1 solid coral (#FF6B6B) PNG encoded as base64.
const TEST_PNG_B64 =
	"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGP4n539HwAHFwLVF8kc1wAAAABJRU5ErkJggg==";

const expectDiffText = async (element: HTMLElement, text: string) => {
	await waitFor(() =>
		expect(
			Array.from(element.querySelectorAll("diffs-container")).some((host) =>
				host.shadowRoot?.textContent?.includes(text),
			),
		).toBe(true),
	);
};

const meta: Meta<typeof Tool> = {
	title: "pages/AgentsPage/ChatElements/tools/Tool",
	component: Tool,
	args: {
		name: "execute",
		args: { command: executeCommand },
		status: "completed",
	},
	parameters: {
		reactRouter: reactRouterParameters({
			routing: { path: "/" },
		}),
	},
};

export default meta;
type Story = StoryObj<typeof Tool>;

type ToolShowcaseItem = {
	name: string;
	status?: React.ComponentProps<typeof Tool>["status"];
	args?: unknown;
	result?: unknown;
	isError?: boolean;
	killedBySignal?: "kill" | "terminate";
	modelIntent?: string;
	parsedCommands?: readonly string[][];
	subagentVariants?: Map<string, "general" | "explore" | "computer_use">;
};

const allToolShowcaseItems: ToolShowcaseItem[] = [
	{
		name: "execute",
		args: { command: "pnpm check", model_intent: "Checking frontend" },
		modelIntent: "Checking frontend",
		parsedCommands: [["pnpm", "check"]],
		result: {
			output: "Checked 1799 files.",
			wall_duration_ms: 2400,
			exit_code: 0,
		},
	},
	{
		name: "process_output",
		args: { process_id: "storybook-process" },
		result: { output: "dev server ready on :6006" },
	},
	{
		name: "process_list",
		args: {},
		result: {
			processes: [
				{
					id: "storybook-process",
					command: "pnpm storybook",
					status: "running",
				},
			],
		},
	},
	{
		name: "process_signal",
		args: { process_id: "storybook-process", signal: "terminate" },
		result: { success: true },
	},
	{
		name: "wait_for_external_auth",
		args: { provider: "github" },
		result: { provider_display_name: "GitHub", authenticated: true },
	},
	{
		name: "read_file",
		args: { path: "site/src/pages/AgentsPage/AgentChatPage.tsx" },
		result: { content: "export const AgentChatPage = () => null;" },
	},
	{
		name: "write_file",
		args: { path: "docs/example.md", content: "# Example\n" },
		result: { path: "docs/example.md" },
	},
	{
		name: "edit_files",
		args: {
			files: [
				{
					path: "site/src/example.ts",
					edits: [{ old_text: "foo", new_text: "bar" }],
				},
			],
		},
		result: { files: [{ path: "site/src/example.ts", status: "edited" }] },
	},
	{
		name: "list_templates",
		result: {
			templates: [
				{
					id: "template-1",
					name: "go-template",
					display_name: "Go Development",
				},
			],
			count: 1,
		},
	},
	{
		name: "read_template",
		args: { template_id: "template-1" },
		result: {
			template: { name: "go-template", display_name: "Go Development" },
		},
	},
	{
		name: "create_workspace",
		result: {
			created: true,
			workspace_name: "agent-icons",
			build_id: "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
		},
	},
	{
		name: "start_workspace",
		result: {
			started: true,
			workspace_name: "agent-icons",
			agent_status: "ready",
			build_id: "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
		},
	},
	{
		name: "chat_summarized",
		result: { summary: "Earlier transcript content was compacted." },
	},
	{
		name: "propose_plan",
		args: { path: "/home/coder/.coder/plans/PLAN-example.md" },
		result: { path: "/home/coder/.coder/plans/PLAN-example.md" },
	},
	{
		name: "ask_user_question",
		args: { questions: [] },
		status: "running",
	},
	{
		name: "advisor",
		args: { question: "Which icon family should represent transcript tools?" },
		result: { answer: "Use category-level icons for better scanning." },
	},
	{
		name: "computer",
		args: { action: "screenshot" },
		result: { output: { type: "image", data: TEST_PNG_B64 } },
	},
	{
		name: "read_skill",
		args: { name: "deep-review" },
		result: {
			name: "deep-review",
			content: "# Deep Review\nReview code carefully.",
		},
	},
	{
		name: "read_skill_file",
		args: { name: "deep-review", path: "roles/security-reviewer.md" },
		result: { content: "# Security Reviewer Role\nCheck auth boundaries." },
	},
	{
		name: "spawn_agent",
		args: { title: "Repository review", prompt: "Review the code." },
		result: {
			chat_id: "bot-child",
			title: "Repository review",
			status: "completed",
		},
	},
	{
		name: "wait_agent",
		args: { chat_id: "bot-child" },
		result: {
			chat_id: "bot-child",
			title: "Repository review",
			status: "completed",
			report: "No issues found.",
		},
	},
	{
		name: "message_agent",
		args: { chat_id: "bot-child", message: "Check icon consistency." },
		result: { chat_id: "bot-child", status: "completed" },
	},
	{
		name: "interrupt_agent",
		args: { chat_id: "bot-child" },
		result: { chat_id: "bot-child", status: "completed" },
	},
	{
		name: "spawn_computer_use_agent",
		args: { prompt: "Inspect the UI." },
		result: { chat_id: "desktop-child", status: "completed" },
		subagentVariants: new Map([["desktop-child", "computer_use"]]),
	},
	{
		name: "read_file",
		args: { path: "site/src/pages/AgentsPage/Missing.tsx" },
		status: "error",
		isError: true,
		result: { error: "File not found" },
	},
	{
		name: "create_workspace",
		status: "running",
		args: { workspace_name: "agent-icons" },
		result: {
			workspace_name: "agent-icons",
			build_id: "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
		},
	},
	{
		name: "unknown_tool",
		args: { example: true },
		result: { ok: true },
	},
];

// ---------------------------------------------------------------------------
// Execute stories
// ---------------------------------------------------------------------------

export const ExecuteRunning: Story = {
	args: {
		status: "running",
		result: {
			output: "remote: Enumerating objects: 12, done.\nFetching origin...",
		},
	},
};

export const ExecuteModelIntent: Story = {
	args: {
		status: "completed",
		args: {
			command: executeIntentCommand,
			model_intent: "Running tests using npm for 5s",
		},
		modelIntent: "Running tests using npm for 5s",
		result: {
			output: "",
			wall_duration_ms: 2300,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const commandButton = canvas.getByRole("button", {
			name: "Expand command",
		});
		expect(commandButton).toHaveTextContent(
			`Running tests using ${executeIntentCommand} for 2.3s`,
		);
		expect(commandButton).not.toHaveTextContent("Ran");
	},
};

export const ExecuteModelIntentRunning: Story = {
	args: {
		shellToolDisplayMode: "always_expanded",
		status: "running",
		args: {
			command: executeCommand,
			model_intent: "checking repository state",
		},
		modelIntent: "checking repository state",
		result: {
			output: "",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const commandButton = canvas.getByRole("button", {
			name: "Collapse command",
		});
		expect(commandButton).toHaveTextContent(
			`Checking repository state using ${executeCommand}`,
		);
		expect(commandButton).not.toHaveTextContent(" for ");
		expect(commandButton).not.toHaveTextContent("Ran");
	},
};

export const ExecuteModelIntentLeadingUsing: Story = {
	args: {
		status: "completed",
		args: {
			command: executeCommand,
			model_intent: "using git fetch origin",
		},
		modelIntent: "using git fetch origin",
		result: {
			output: "",
			wall_duration_ms: 2300,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const commandButton = canvas.getByRole("button", {
			name: "Expand command",
		});
		expect(commandButton).toHaveTextContent(`Ran ${executeCommand} for 2.3s`);
		expect(commandButton).not.toHaveTextContent("using git fetch origin using");
	},
};

export const ExecuteSuccess: Story = {
	args: {
		shellToolDisplayMode: "auto",
		args: { command: longExecuteCommand },
		result: {
			wall_duration_ms: 47200,
			exit_code: 0,
			output:
				"From github.com:coder/coder\n * [new branch]      feature/agent-ui -> origin/feature/agent-ui",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/From github\.com:coder\/coder/)).toBeVisible();
		expect(canvas.queryByText("exit 0")).not.toBeInTheDocument();
		expect(
			canvas.queryByRole("img", { name: "Running in background" }),
		).not.toBeInTheDocument();
		const durationSuffix = canvas.getByText("for 47.2s");
		expect(durationSuffix).toBeVisible();
		expect(durationSuffix.tagName).toBe("SPAN");
		expect(canvas.queryByText("2 lines")).not.toBeInTheDocument();
	},
};

export const ExecuteError: Story = {
	args: {
		name: "execute",
		status: "error",
		isError: true,
		args: { command: longExecuteCommand },
		shellToolDisplayMode: "always_collapsed",
		result: {
			wall_duration_ms: 8600,
			exit_code: 1,
			output: Array.from(
				{ length: 47 },
				(_, index) => `error line ${index + 1}`,
			).join("\n"),
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.queryByText(/error line 1/)).not.toBeInTheDocument();
		expect(canvas.getByRole("img", { name: "Command failed" })).toBeVisible();
		expect(canvas.queryByText("exit 1")).not.toBeInTheDocument();
		await userEvent.click(
			canvas.getByRole("button", { name: "Expand command" }),
		);
		await waitFor(() => {
			expect(canvas.getByText(/error line 1/)).toBeVisible();
		});
	},
};

export const ExecuteBackgrounded: Story = {
	args: {
		name: "execute",
		status: "completed",
		args: { command: "npm start" },
		shellToolDisplayMode: "always_collapsed",
		result: {
			background_process_id: "process-123",
			output: "",
			wall_duration_ms: 2100,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const backgroundIndicator = canvas.getByRole("img", {
			name: "Running in background",
		});
		expect(backgroundIndicator).toBeVisible();
		await userEvent.hover(backgroundIndicator);
		expect(await screen.findByRole("tooltip")).toHaveTextContent(
			"Running in background",
		);
	},
};

export const ExecuteAlwaysCollapsed: Story = {
	args: {
		name: "execute",
		status: "completed",
		args: { command: executeCommand },
		shellToolDisplayMode: "always_collapsed",
		result: {
			output: "From github.com:coder/coder\nFetching origin/main",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const commandButton = canvas.getByRole("button", {
			name: "Expand command",
		});
		expect(commandButton).toHaveTextContent(`Ran ${executeCommand}`);
		expect(canvas.queryByText("exit 0")).not.toBeInTheDocument();
		expect(canvas.queryByText("2 lines")).not.toBeInTheDocument();
		expect(
			canvas.queryByText(/From github\.com:coder\/coder/),
		).not.toBeInTheDocument();
		await userEvent.click(commandButton);
		await waitFor(() => {
			expect(canvas.getByText(/From github\.com:coder\/coder/)).toBeVisible();
		});
	},
};

export const ExecuteLongCommandCollapsed: Story = {
	args: {
		name: "execute",
		status: "completed",
		args: { command: longExecuteCommand },
		shellToolDisplayMode: "always_collapsed",
		result: {
			wall_duration_ms: 47200,
			exit_code: 0,
			output: Array.from(
				{ length: 61 },
				(_, index) => `output line ${index + 1}`,
			).join("\n"),
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const commandButton = canvas.getByRole("button", {
			name: "Expand command",
		});
		expect(commandButton).toHaveTextContent(`Ran ${longExecuteCommand}`);
		expect(commandButton).toHaveAttribute("aria-expanded", "false");
		expect(canvas.queryByText("exit 0")).not.toBeInTheDocument();
		expect(canvas.getByText(/for 47\.2s/)).toBeVisible();
		expect(canvas.queryByText("61 lines")).not.toBeInTheDocument();
	},
};

export const ProcessOutputAlwaysCollapsed: Story = {
	args: {
		name: "process_output",
		status: "completed",
		shellToolDisplayMode: "always_collapsed",
		result: {
			output: "build completed\n0 errors",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.queryByText(/build completed/)).not.toBeInTheDocument();
		await userEvent.click(
			canvas.getByRole("button", { name: "Expand process output" }),
		);
		await waitFor(() => {
			expect(canvas.getByText(/build completed/)).toBeVisible();
		});
	},
};

export const ProcessOutputAlwaysExpanded: Story = {
	args: {
		name: "process_output",
		status: "completed",
		shellToolDisplayMode: "always_expanded",
		result: {
			output: Array.from(
				{ length: 30 },
				(_, index) => `process output line ${index + 1}`,
			).join("\n"),
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/process output line 1/)).toBeVisible();
		expect(canvas.getByText(/process output line 30/)).toBeVisible();
		await waitFor(() => {
			expect(
				canvas.getByRole("button", {
					name: "Collapse full process output",
				}),
			).toHaveAttribute("aria-expanded", "true");
		});
	},
};

export const ProcessOutputStringError: Story = {
	args: {
		name: "process_output",
		status: "error",
		isError: true,
		result: "permission denied",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("img", { name: "Failed to read process output" }),
		).toBeVisible();
	},
};

export const ExecuteAuthRequired: Story = {
	args: {
		result: {
			auth_required: true,
			provider_display_name: "GitHub",
			authenticate_url: "https://coder.example.com/external-auth/github",
			output:
				"fatal: could not read Username for 'https://github.com': terminal prompts disabled",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const button = canvas.getByRole("button", {
			name: "Authenticate with GitHub",
		});
		expect(button).toBeInTheDocument();
		expect(
			canvas.getByRole("link", { name: "Open authentication link" }),
		).toHaveAttribute("href", "https://coder.example.com/external-auth/github");

		const openSpy = spyOn(window, "open").mockImplementation(() => null);
		await userEvent.click(button);
		expect(openSpy).toHaveBeenCalledWith(
			"https://coder.example.com/external-auth/github",
			"_blank",
			"width=900,height=600",
		);
		openSpy.mockRestore();
	},
};

// ---------------------------------------------------------------------------
// WaitForExternalAuth stories
// ---------------------------------------------------------------------------

export const WaitForExternalAuthRunning: Story = {
	args: {
		name: "wait_for_external_auth",
		status: "running",
		result: {
			provider_display_name: "GitHub",
			authenticated: false,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByText("Waiting for GitHub authentication..."),
		).toBeInTheDocument();
		expect(
			canvas.getByRole("img", { name: "Authentication in progress" }),
		).toBeVisible();
	},
};

export const WaitForExternalAuthAuthenticated: Story = {
	args: {
		name: "wait_for_external_auth",
		status: "completed",
		result: {
			provider_display_name: "GitHub",
			authenticated: true,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Authenticated with GitHub")).toBeInTheDocument();
	},
};

export const WaitForExternalAuthTimedOut: Story = {
	args: {
		name: "wait_for_external_auth",
		status: "completed",
		result: {
			provider_display_name: "GitHub",
			timed_out: true,
		},
	},
};

export const WaitForExternalAuthError: Story = {
	args: {
		name: "wait_for_external_auth",
		status: "error",
		isError: true,
		result: {
			provider_display_name: "GitHub",
			error: "Authentication failed: token exchange was rejected.",
		},
	},
};

// ---------------------------------------------------------------------------
// Subagent stories
// ---------------------------------------------------------------------------

export const SubagentRunning: Story = {
	args: {
		name: "spawn_agent",
		status: "running",
		args: {
			title: "Workspace diagnostics",
			prompt: "Collect logs and summarize why startup failed.",
		},
		result: {
			chat_id: "child-chat-id",
			title: "Workspace diagnostics",
			status: "pending",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByRole("link", { name: "View agent" })).toHaveAttribute(
			"href",
			"/agents/child-chat-id",
		);
	},
};

export const SubagentMalformedChatIdLinksToRecoverableChatId: Story = {
	args: {
		name: "spawn_agent",
		status: "completed",
		args: {
			title: "Workspace diagnostics",
			prompt: "Collect logs and summarize why startup failed.",
		},
		result: {
			chat_id: ["8f3a6131-1ce8-46f5-9", "b", "a8-4a36-beb2? no"].join(""),
			title: "Workspace diagnostics",
			status: "completed",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("button", { name: /Spawned Workspace diagnostics/ }),
		).toBeInTheDocument();
		expect(canvas.getByRole("link", { name: "View agent" })).toHaveAttribute(
			"href",
			["/agents/8f3a6131-1ce8-46f5-9", "b", "a8-4a36-beb2"].join(""),
		);
	},
};

const mockChatModelConfig = {
	...MockChatModelConfig,
	id: "8b29eba2-53a9-4c9a-95bb-b0326ac0a2fe",
	model: "claude-sonnet-4-6",
	display_name: "Claude Sonnet 4.6",
	model_config: {
		reasoning_effort: { default: "medium", max: "high" },
	},
};

export const SubagentSpawnWithModelAndEffort: Story = {
	args: {
		name: "spawn_agent",
		status: "completed",
		args: {
			title: "Workspace diagnostics",
			prompt: "Collect logs and summarize why startup failed.",
			model_config_id: "8b29eba2-53a9-4c9a-95bb-b0326ac0a2fe",
			reasoning_effort: "high",
		},
		result: {
			chat_id: "child-chat-id",
			title: "Workspace diagnostics",
			status: "completed",
		},
	},
	parameters: {
		queries: [{ key: chatModelConfigsKey, data: [mockChatModelConfig] }],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() =>
			expect(
				canvas.getByRole("button", {
					name: /Spawned Workspace diagnostics with Claude Sonnet 4.6, high thinking/,
				}),
			).toBeInTheDocument(),
		);
	},
};

export const SubagentSpawnWithModelDefaultEffort: Story = {
	args: {
		name: "spawn_agent",
		status: "completed",
		args: {
			title: "Workspace diagnostics",
			prompt: "Collect logs and summarize why startup failed.",
			model_config_id: "8b29eba2-53a9-4c9a-95bb-b0326ac0a2fe",
		},
		result: {
			chat_id: "child-chat-id",
			title: "Workspace diagnostics",
			status: "completed",
		},
	},
	parameters: {
		queries: [{ key: chatModelConfigsKey, data: [mockChatModelConfig] }],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() =>
			expect(
				canvas.getByRole("button", {
					name: /Spawned Workspace diagnostics with Claude Sonnet 4.6, medium thinking/,
				}),
			).toBeInTheDocument(),
		);
	},
};

// Effort-only spawns resolve against an inherited model whose effort
// bounds are unknown client-side, so no suffix is shown.
export const SubagentSpawnWithEffortOnly: Story = {
	args: {
		name: "spawn_agent",
		status: "completed",
		args: {
			title: "Workspace diagnostics",
			prompt: "Collect logs and summarize why startup failed.",
			reasoning_effort: "high",
		},
		result: {
			chat_id: "child-chat-id",
			title: "Workspace diagnostics",
			status: "completed",
		},
	},
	parameters: {
		queries: [{ key: chatModelConfigsKey, data: [mockChatModelConfig] }],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const label = canvas.getByRole("button", {
			name: /Spawned Workspace diagnostics/,
		});
		expect(label).toBeInTheDocument();
		expect(label).not.toHaveTextContent(/with/);
	},
};

export const SubagentSpawnWithUnknownModelConfig: Story = {
	args: {
		name: "spawn_agent",
		status: "completed",
		args: {
			title: "Workspace diagnostics",
			prompt: "Collect logs and summarize why startup failed.",
			model_config_id: "00000000-0000-0000-0000-000000000000",
		},
		result: {
			chat_id: "child-chat-id",
			title: "Workspace diagnostics",
			status: "completed",
		},
	},
	parameters: {
		queries: [{ key: chatModelConfigsKey, data: [mockChatModelConfig] }],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const label = canvas.getByRole("button", {
			name: /Spawned Workspace diagnostics/,
		});
		expect(label).toBeInTheDocument();
		expect(label).not.toHaveTextContent(/with/);
	},
};

export const ExploreSubagentRunning: Story = {
	args: {
		name: "spawn_explore_agent",
		status: "running",
		args: {
			prompt: "Read the repository and summarize the auth flow.",
		},
		result: {
			chat_id: "explore-chat-id",
			status: "pending",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByRole("link", { name: "View agent" })).toHaveAttribute(
			"href",
			"/agents/explore-chat-id",
		);
		expect(
			canvas.getByRole("button", { name: /Spawning Explore agent/ }),
		).toBeInTheDocument();
	},
};

export const SubagentAwaitLinkCard: Story = {
	args: {
		name: "wait_agent",
		args: { title: "Sub-agent" },
		result: { chat_id: "child-chat-id", status: "pending" },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByRole("link", { name: "View agent" })).toHaveAttribute(
			"href",
			"/agents/child-chat-id",
		);
	},
};

export const SubagentMessageLinkCard: Story = {
	args: {
		name: "message_agent",
		args: { title: "Sub-agent" },
		result: { chat_id: "child-chat-id", status: "pending" },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByRole("link", { name: "View agent" })).toHaveAttribute(
			"href",
			"/agents/child-chat-id",
		);
	},
};

export const SubagentCompletedDelegatedPending: Story = {
	args: {
		name: "spawn_agent",
		args: undefined,
		result: { chat_id: "child-chat-id", status: "pending" },
		status: "completed",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByRole("link", { name: "View agent" })).toHaveAttribute(
			"href",
			"/agents/child-chat-id",
		);
		expect(
			canvas.getByRole("button", { name: /Spawned Sub-agent/ }),
		).toBeInTheDocument();
		expect(canvasElement.querySelector(".animate-spin")).toBeNull();
	},
};

export const SubagentStreamOverrideStatus: Story = {
	args: {
		name: "spawn_agent",
		args: undefined,
		result: { chat_id: "child-chat-id", status: "pending" },
		status: "completed",
		subagentStatusOverrides: new Map([["child-chat-id", "completed"]]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("button", { name: /Spawned Sub-agent/ }),
		).toBeInTheDocument();
		expect(canvasElement.querySelector(".animate-spin")).toBeNull();
	},
};

export const SubagentNoErrorWhenCompleted: Story = {
	args: {
		name: "spawn_agent",
		args: undefined,
		result: {
			chat_id: "child-chat-id",
			status: "completed",
			error: "provider metadata noise",
		},
		status: "error",
		isError: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvasElement.querySelector(".animate-spin")).toBeNull();
		expect(canvasElement.querySelector(".lucide-circle-alert")).toBeNull();
		expect(
			canvas.getByRole("button", { name: /Spawned Sub-agent/ }),
		).toBeInTheDocument();
	},
};

export const SubagentAwaitPreferredTitle: Story = {
	args: {
		name: "wait_agent",
		args: { title: "Fallback title" },
		result: {
			chat_id: "child-chat-id",
			title: "Delegated child title",
			status: "completed",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Delegated child title")).toBeInTheDocument();
		expect(canvas.getByRole("link", { name: "View agent" })).toHaveAttribute(
			"href",
			"/agents/child-chat-id",
		);
		expect(canvas.queryByText("Fallback title")).toBeNull();
	},
};

export const SpawnSubagentGeneralRunning: Story = {
	args: {
		name: "spawn_agent",
		status: "running",
		args: {
			type: "general",
			title: "Workspace diagnostics",
			prompt: "Collect logs and summarize why startup failed.",
		},
		result: {
			chat_id: "spawn-general-child",
			type: "general",
			status: "pending",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("button", { name: /Spawning Workspace diagnostics/ }),
		).toBeInTheDocument();
	},
};

export const SpawnSubagentGeneralCompleted: Story = {
	args: {
		name: "spawn_agent",
		status: "completed",
		args: {
			type: "general",
			title: "Workspace diagnostics",
			prompt: "Collect logs and summarize why startup failed.",
		},
		result: {
			chat_id: "spawn-general-child",
			type: "general",
			title: "Workspace diagnostics",
			status: "completed",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("button", { name: /Spawned Workspace diagnostics/ }),
		).toBeInTheDocument();
	},
};

export const SpawnSubagentExploreRunning: Story = {
	args: {
		name: "spawn_agent",
		status: "running",
		args: {
			type: "explore",
			prompt: "Read the repository and summarize the auth flow.",
		},
		result: {
			chat_id: "spawn-explore-child",
			type: "explore",
			status: "pending",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("button", { name: /Spawning Explore agent/ }),
		).toBeInTheDocument();
	},
};

export const SpawnSubagentExploreCompleted: Story = {
	args: {
		name: "spawn_agent",
		status: "completed",
		args: {
			type: "explore",
			prompt: "Read the repository and summarize the auth flow.",
		},
		result: {
			chat_id: "spawn-explore-child",
			type: "explore",
			status: "completed",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("button", { name: /Spawned Explore agent/ }),
		).toBeInTheDocument();
	},
};

export const SpawnSubagentComputerUseRunning: Story = {
	args: {
		name: "spawn_agent",
		status: "running",
		args: {
			type: "computer_use",
			title: "Visual regression check",
			prompt:
				"Open the browser and check for visual regressions on the dashboard page.",
		},
		result: {
			chat_id: "spawn-desktop-child",
			type: "computer_use",
			status: "pending",
		},
	},
	decorators: [
		(Story) => (
			<DesktopPanelContext.Provider
				value={{
					desktopChatId: "spawn-desktop-child",
					onOpenDesktop: fn(),
				}}
			>
				<Story />
			</DesktopPanelContext.Provider>
		),
	],
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/Using the computer/)).toBeInTheDocument();
		expect(canvasElement.querySelector(".lucide-monitor")).not.toBeNull();
		await waitFor(() => {
			expect(
				canvas.getByRole("button", { name: "Open desktop tab" }),
			).toBeInTheDocument();
		});
	},
};

export const SpawnSubagentComputerUseCompleted: Story = {
	args: {
		name: "spawn_agent",
		status: "completed",
		args: {
			type: "computer_use",
			title: "Visual regression check",
			prompt:
				"Open the browser and check for visual regressions on the dashboard page.",
		},
		result: {
			chat_id: "spawn-desktop-child",
			type: "computer_use",
			title: "Visual regression check",
			status: "completed",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("button", { name: /Spawned Visual regression check/ }),
		).toBeInTheDocument();
	},
};

export const WaitAgentExploreStreamingFromHistory: Story = {
	args: {
		name: "wait_agent",
		status: "running",
		args: { chat_id: "explore-child" },
		result: { chat_id: "explore-child", status: "pending" },
		subagentVariants: new Map([["explore-child", "explore"]]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("button", { name: /Waiting for Explore agent/ }),
		).toBeInTheDocument();
		expect(canvas.queryByText("Waiting for sub-agent…")).toBeNull();
	},
};

export const MessageAgentExploreStreamingFromResult: Story = {
	args: {
		name: "message_agent",
		status: "running",
		args: { chat_id: "message-child", message: "continue" },
		result: {
			chat_id: "message-child",
			type: "explore",
			status: "pending",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("button", { name: /Messaging Explore agent/ }),
		).toBeInTheDocument();
		expect(canvas.queryByText("Messaging sub-agent…")).toBeNull();
	},
};

export const InterruptAgentRunningWithoutChatId: Story = {
	args: {
		name: "interrupt_agent",
		status: "running",
		args: {},
		result: { status: "running" },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			expect(canvasElement.textContent?.trim()).toBe("");
		});
		expect(canvasElement.querySelector("[data-transcript-row]")).toBeNull();
		expect(canvas.queryByRole("button")).toBeNull();
		expect(canvas.queryByRole("link", { name: "View agent" })).toBeNull();
	},
};

// interrupt_agent is the post-rename name for close_agent. The response
// carries `interrupted: true`.
export const InterruptAgentExploreCompleted: Story = {
	args: {
		name: "interrupt_agent",
		status: "completed",
		args: { chat_id: "interrupt-child" },
		result: {
			chat_id: "interrupt-child",
			type: "explore",
			status: "completed",
			interrupted: true,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("button", { name: /Interrupted Explore agent/ }),
		).toBeInTheDocument();
	},
};

// list_agents renders through ListAgentsTool, showing a count in the
// header and an expandable list of agents with links.
export const ListAgentsCompleted: Story = {
	args: {
		name: "list_agents",
		status: "completed",
		args: {},
		result: {
			agents: [
				{
					chat_id: "agent-1",
					title: "Repository review",
					type: "general",
					status: "completed",
					created_at: "2026-04-21T00:00:00.000Z",
					updated_at: "2026-04-21T00:05:00.000Z",
				},
				{
					chat_id: "agent-2",
					title: "Inspect repository",
					type: "explore",
					status: "running",
					created_at: "2026-04-21T00:01:00.000Z",
					updated_at: "2026-04-21T00:06:00.000Z",
				},
				{
					chat_id: "agent-3",
					title: "Drive the desktop",
					type: "computer_use",
					status: "pending",
					created_at: "2026-04-21T00:02:00.000Z",
					updated_at: "2026-04-21T00:07:00.000Z",
				},
			],
			total: 3,
			returned: 3,
			offset: 0,
			has_more: false,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const header = canvas.getByRole("button", { name: /Listed 3 of 3 agents/ });
		expect(header).toBeInTheDocument();
		// Expand to verify agent rows and links render.
		await userEvent.click(header);
		expect(
			canvas.getByText("Repository review (general, completed)"),
		).toBeInTheDocument();
		expect(
			canvas.getByText("Inspect repository (explore, running)"),
		).toBeInTheDocument();
	},
};

export const ListAgentsRunning: Story = {
	args: {
		name: "list_agents",
		status: "running",
		args: {},
		result: undefined,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Listing agents")).toBeInTheDocument();
	},
};

export const ListAgentsEmpty: Story = {
	args: {
		name: "list_agents",
		status: "completed",
		args: {},
		result: {
			agents: [],
			total: 0,
			has_more: false,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Listed 0 agents")).toBeInTheDocument();
	},
};

export const ListAgentsError: Story = {
	args: {
		name: "list_agents",
		status: "error",
		isError: true,
		args: {},
		result: "list_agents is only available on root chats",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Listed 0 agents")).toBeInTheDocument();
	},
};

// ---------------------------------------------------------------------------
// ListTemplates stories
// ---------------------------------------------------------------------------

export const ListTemplatesRunning: Story = {
	args: {
		name: "list_templates",
		status: "running",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Listing templates…")).toBeInTheDocument();
	},
};

export const ListTemplatesSuccess: Story = {
	args: {
		name: "list_templates",
		status: "completed",
		result: {
			templates: [
				{
					id: "template-1",
					name: "go-template",
					display_name: "Go Development",
					description: "A template for Go development with VS Code",
				},
				{
					id: "template-2",
					name: "python-template",
					description: "Python development environment",
				},
			],
			count: 2,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Listed 2 templates")).toBeInTheDocument();
		const toggle = canvas.getByRole("button");
		await userEvent.click(toggle);
		expect(canvas.getByText("Go Development")).toBeInTheDocument();
		expect(canvas.getByText("python-template")).toBeInTheDocument();
	},
};

export const ListTemplatesSingle: Story = {
	args: {
		name: "list_templates",
		status: "completed",
		result: {
			templates: [
				{
					id: "template-1",
					name: "go-template",
					description: "Go development template",
				},
			],
			count: 1,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Listed 1 template")).toBeInTheDocument();
	},
};

export const ListTemplatesEmpty: Story = {
	args: {
		name: "list_templates",
		status: "completed",
		result: {
			templates: [],
			count: 0,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Listing templates…")).toBeInTheDocument();
	},
};

// ---------------------------------------------------------------------------
// ChatSummarized stories
// ---------------------------------------------------------------------------

export const ChatSummarized: Story = {
	args: {
		name: "chat_summarized",
		args: undefined,
		result: { summary: "Compaction summary text." },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const toggle = canvas.getByRole("button", { name: "Summarized" });
		expect(toggle).toBeInTheDocument();
		expect(canvas.queryByText("Compaction summary text.")).toBeNull();

		await userEvent.click(toggle);

		expect(
			await canvas.findByText((text) =>
				text.includes("Compaction summary text."),
			),
		).toBeInTheDocument();
	},
};

// Automatic compactions include source: "automatic" but keep the
// plain "Summarized" label; only manual ones are called out.
export const ChatSummarizedAutomaticSource: Story = {
	args: {
		name: "chat_summarized",
		args: JSON.stringify({ source: "automatic" }),
		result: { summary: "Compaction summary text.", source: "automatic" },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("button", { name: "Summarized" }),
		).toBeInTheDocument();
	},
};

// A user-requested /compact renders with a distinct manual label.
export const ChatSummarizedManual: Story = {
	args: {
		name: "chat_summarized",
		args: JSON.stringify({ source: "manual" }),
		result: { summary: "Manual compaction summary text.", source: "manual" },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const toggle = canvas.getByRole("button", { name: "Summarized (manual)" });
		expect(toggle).toBeInTheDocument();

		await userEvent.click(toggle);

		expect(
			await canvas.findByText((text) =>
				text.includes("Manual compaction summary text."),
			),
		).toBeInTheDocument();
	},
};

// While the summary streams in, the manual source is only present in
// the call args; the header still shows the running label.
export const ChatSummarizedManualRunning: Story = {
	args: {
		name: "chat_summarized",
		args: JSON.stringify({ source: "manual" }),
		status: "running",
		result: undefined,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Summarizing…")).toBeInTheDocument();
	},
};

// ---------------------------------------------------------------------------
// SubagentInterrupt stories
// ---------------------------------------------------------------------------

export const SubagentInterrupt: Story = {
	args: {
		name: "interrupt_agent",
		args: undefined,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/Interrupted/)).toBeInTheDocument();
		expect(canvas.getByText("Sub-agent")).toBeInTheDocument();
	},
};

// ---------------------------------------------------------------------------
// Generic fallback stories
// ---------------------------------------------------------------------------

export const TaskNameGenericRendering: Story = {
	args: {
		name: "task",
		args: undefined,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("task")).toBeInTheDocument();
		expect(canvas.queryByRole("link", { name: "View agent" })).toBeNull();
	},
};

// ---------------------------------------------------------------------------
// MCP tool stories (generic renderer with MCP server context)
// ---------------------------------------------------------------------------

const sampleMCPServers = [
	{
		id: "mcp-server-1",
		slug: "linear",
		display_name: "Linear",
		description: "Project management",
		icon_url: "https://linear.app/favicon.ico",
		transport: "streamable_http",
		url: "https://mcp.linear.app",
		auth_type: "oauth2",
		has_oauth2_secret: false,
		has_api_key: false,
		has_custom_headers: false,
		tool_allow_list: [],
		tool_deny_list: [],
		availability: "default_on",
		enabled: true,
		model_intent: false,
		allow_in_plan_mode: false,
		forward_coder_headers: false,
		auth_connected: true,
		created_at: "2025-01-01T00:00:00Z",
		updated_at: "2025-01-01T00:00:00Z",
	},
] satisfies readonly import("#/api/typesGenerated").MCPServerConfig[];

export const MCPToolRunning: Story = {
	args: {
		name: "linear__list_issues",
		status: "running",
		args: { project: "backend" },
		mcpServerConfigId: "mcp-server-1",
		mcpServers: sampleMCPServers,
	},
	play: async ({ canvasElement }) => {
		// Spinner should be visible while running.
		expect(canvasElement.querySelector(".animate-spin")).not.toBeNull();
		// Icon should be monochrome (brightness-0 filter).
		const icon = canvasElement.querySelector(".brightness-0");
		expect(icon).not.toBeNull();
	},
};

export const MCPToolCompleted: Story = {
	args: {
		name: "linear__list_issues",
		status: "completed",
		args: { project: "backend" },
		result: {
			issues: [
				{ id: "LIN-123", title: "Fix auth flow" },
				{ id: "LIN-456", title: "Update dashboard" },
			],
		},
		mcpServerConfigId: "mcp-server-1",
		mcpServers: sampleMCPServers,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// No spinner when completed.
		expect(canvasElement.querySelector(".animate-spin")).toBeNull();
		// Icon should still be monochrome when completed.
		expect(canvasElement.querySelector(".brightness-0")).not.toBeNull();
		const toggle = canvas.getByRole("button");
		expect(toggle).toBeInTheDocument();
		await userEvent.click(toggle);
		expect(canvas.getByText("Input")).toBeVisible();
		expect(canvas.getByText("Output")).toBeVisible();
		await expectDiffText(canvasElement, "Fix auth flow");
	},
};

export const MCPToolError: Story = {
	args: {
		name: "linear__list_issues",
		status: "error",
		isError: true,
		args: { project: "backend" },
		result: { error: "Authentication token expired" },
		mcpServerConfigId: "mcp-server-1",
		mcpServers: sampleMCPServers,
	},
	play: async ({ canvasElement }) => {
		// Warning triangle icon should be present.
		expect(
			canvasElement.querySelector(".lucide-triangle-alert"),
		).not.toBeNull();
		// Label text should NOT use the destructive color.
		expect(canvasElement.querySelector(".text-content-destructive")).toBeNull();
	},
};

export const MCPToolNoResult: Story = {
	args: {
		name: "linear__create_issue",
		status: "completed",
		args: { title: "New issue" },
		mcpServerConfigId: "mcp-server-1",
		mcpServers: sampleMCPServers,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button"));
		expect(canvas.getByText("Input")).toBeVisible();
		await expectDiffText(canvasElement, "New issue");
	},
};

export const MCPToolSlackIcon: Story = {
	args: {
		name: "slack__post_message",
		status: "completed",
		result: { ok: true, channel: "#general" },
		mcpServerConfigId: "mcp-server-1",
		mcpServers: [
			{
				...sampleMCPServers[0],
				slug: "slack",
				display_name: "Slack",
				icon_url:
					"https://upload.wikimedia.org/wikipedia/commons/thumb/d/d5/Slack_icon_2019.svg/500px-Slack_icon_2019.svg.png",
			},
		],
	},
};

export const MCPToolGitHubIcon: Story = {
	args: {
		name: "github__list_prs",
		status: "completed",
		result: { prs: [{ id: 1, title: "Fix bug" }] },
		mcpServerConfigId: "mcp-server-1",
		mcpServers: [
			{
				...sampleMCPServers[0],
				slug: "github",
				display_name: "GitHub",
				icon_url:
					"https://upload.wikimedia.org/wikipedia/commons/9/91/Octicons-mark-github.svg",
			},
		],
	},
};

export const MCPToolFigmaIcon: Story = {
	args: {
		name: "figma__get_file",
		status: "completed",
		result: { file: "design.fig" },
		mcpServerConfigId: "mcp-server-1",
		mcpServers: [
			{
				...sampleMCPServers[0],
				slug: "figma",
				display_name: "Figma",
				icon_url:
					"https://upload.wikimedia.org/wikipedia/commons/3/33/Figma-logo.svg",
			},
		],
	},
};

export const MCPToolNoServer: Story = {
	args: {
		name: "some_custom_tool",
		status: "completed",
		result: { output: "Tool finished successfully" },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Falls through to generic wrench icon + raw tool name.
		expect(canvas.getByText("some_custom_tool")).toBeInTheDocument();
	},
};

export const WorkspaceMCPToolCompleted: Story = {
	args: {
		name: "workspace-mcp__echo",
		status: "completed",
		args: { message: "hello from workspace MCP" },
		result: { output: "hello from workspace MCP" },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("workspace-mcp__echo")).toBeInTheDocument();
		await userEvent.click(canvas.getByRole("button"));
		expect(canvas.getByText("Input")).toBeVisible();
		expect(canvas.getByText("Output")).toBeVisible();
		await expectDiffText(canvasElement, "message");
		await expectDiffText(canvasElement, "hello from workspace MCP");
	},
};

export const MCPToolModelIntentRunning: Story = {
	args: {
		name: "linear__list_issues",
		status: "running",
		args: { project: "backend" },
		modelIntent: "Fetching backend issues from Linear",
		mcpServerConfigId: "mcp-server-1",
		mcpServers: sampleMCPServers,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Should show the model intent label instead of the raw tool name.
		expect(
			canvas.getByText("Fetching backend issues from Linear"),
		).toBeInTheDocument();
		// Spinner should be visible while running.
		expect(canvasElement.querySelector(".animate-spin")).not.toBeNull();
	},
};

export const MCPToolModelIntentCompleted: Story = {
	args: {
		name: "github__create_pull_request",
		status: "completed",
		args: { title: "Fix auth flow", base: "main" },
		result: { url: "https://github.com/org/repo/pull/42" },
		modelIntent: "creating pull request for auth fix",
		mcpServerConfigId: "mcp-server-1",
		mcpServers: [
			{
				...sampleMCPServers[0],
				slug: "github",
				display_name: "GitHub",
				icon_url:
					"https://upload.wikimedia.org/wikipedia/commons/9/91/Octicons-mark-github.svg",
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Intent should be capitalized.
		expect(
			canvas.getByText("Creating pull request for auth fix"),
		).toBeInTheDocument();
	},
};

// ---------------------------------------------------------------------------
// WriteFile stories
// ---------------------------------------------------------------------------

export const WriteFileRunning: Story = {
	args: {
		name: "write_file",
		status: "running",
		args: {
			path: "src/utils/helpers.ts",
			content:
				"export function greet(name: string): string {\n  return `Hello, ${name}!`;\n}\n",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/Writing helpers\.ts/)).toBeInTheDocument();
	},
};

export const WriteFileSuccess: Story = {
	args: {
		name: "write_file",
		status: "completed",
		codeDiffDisplayMode: "auto",
		args: {
			path: "src/utils/helpers.ts",
			content:
				"export function greet(name: string): string {\n  return `Hello, ${name}!`;\n}\n",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/Wrote helpers\.ts/)).toBeInTheDocument();
		expect(canvas.queryByTestId("write-file-diff")).not.toBeInTheDocument();
		await userEvent.click(
			canvas.getByRole("button", { name: /Wrote helpers\.ts/ }),
		);
		await waitFor(() => {
			expect(canvas.getByTestId("write-file-diff")).toBeVisible();
		});
	},
};

export const WriteFileAlwaysExpanded: Story = {
	args: {
		name: "write_file",
		status: "completed",
		codeDiffDisplayMode: "always_expanded",
		args: {
			path: "src/utils/helpers.ts",
			content:
				"export function greet(name: string): string {\n  return `Hello, ${name}!`;\n}\n",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByTestId("write-file-diff")).toBeVisible();
	},
};

// ---------------------------------------------------------------------------
// EditFiles stories
// ---------------------------------------------------------------------------

export const EditFilesSingleRunning: Story = {
	args: {
		name: "edit_files",
		status: "running",
		args: {
			files: [
				{
					path: "src/config.ts",
					edits: [
						{
							search: "const timeout = 30;",
							replace: "const timeout = 60;",
						},
					],
				},
			],
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/Editing config\.ts/)).toBeInTheDocument();
	},
};

export const EditFilesSingleSuccess: Story = {
	args: {
		name: "edit_files",
		status: "completed",
		codeDiffDisplayMode: "auto",
		args: {
			files: [
				{
					path: "src/config.ts",
					edits: [
						{
							search: "const timeout = 30;",
							replace: "const timeout = 60;",
						},
					],
				},
			],
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/Edited config\.ts/)).toBeInTheDocument();
		expect(canvas.getAllByTestId("edit-file-diff")).toHaveLength(1);
	},
};

export const EditFilesAlwaysCollapsed: Story = {
	args: {
		name: "edit_files",
		status: "completed",
		codeDiffDisplayMode: "always_collapsed",
		args: {
			files: [
				{
					path: "src/config.ts",
					edits: [
						{
							search: "const timeout = 30;",
							replace: "const timeout = 60;",
						},
					],
				},
			],
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/Edited config\.ts/)).toBeVisible();
		expect(canvas.queryAllByTestId("edit-file-diff")).toHaveLength(0);
		await userEvent.click(
			canvas.getByRole("button", { name: /Edited config\.ts/ }),
		);
		await waitFor(() => {
			expect(canvas.getAllByTestId("edit-file-diff")).toHaveLength(1);
		});
	},
};

export const EditFilesMultipleSuccess: Story = {
	args: {
		name: "edit_files",
		status: "completed",
		args: {
			files: [
				{
					path: "src/config.ts",
					edits: [
						{
							search: "const timeout = 30;",
							replace: "const timeout = 60;",
						},
					],
				},
				{
					path: "src/server.ts",
					edits: [
						{
							search: 'const host = "localhost";',
							replace: 'const host = "0.0.0.0";',
						},
					],
				},
			],
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/Edited 2 files/)).toBeInTheDocument();
	},
};

/**
 * Exercises the LCS-based interleaved diff: only the first and last
 * lines change while the middle line stays the same, so the viewer
 * should show context around the modifications instead of removing
 * everything then re-adding everything.
 */
export const EditFilesInterleavedContext: Story = {
	args: {
		name: "edit_files",
		status: "completed",
		args: {
			files: [
				{
					path: "src/constants.ts",
					edits: [
						{
							search:
								'const API_URL = "http://localhost:3000";\nconst RETRY_COUNT = 3;\nconst TIMEOUT_MS = 5000;',
							replace:
								'const API_URL = "https://api.prod.example.com";\nconst RETRY_COUNT = 3;\nconst TIMEOUT_MS = 10000;',
						},
					],
				},
			],
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/Edited constants\.ts/)).toBeInTheDocument();
	},
};

export const EditFilesError: Story = {
	args: {
		name: "edit_files",
		status: "error",
		isError: true,
		args: {
			files: [
				{
					path: "src/missing.ts",
					edits: [
						{
							search: "old",
							replace: "new",
						},
					],
				},
			],
		},
		result: { error: "File not found" },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/Edited missing\.ts/)).toBeInTheDocument();
		// On error, no diff body: the synthetic fallback would
		// misrepresent a rejected edit as applied.
		expect(canvas.queryAllByTestId("edit-file-diff")).toHaveLength(0);
	},
};

export const EditFilesServerDiffMultiFile: Story = {
	args: {
		name: "edit_files",
		status: "completed",
		args: {
			files: [
				{
					path: "src/config.ts",
					edits: [
						{
							search: "const timeout = 30;",
							replace: "const timeout = 60;",
						},
					],
				},
				{
					path: "src/server.ts",
					edits: [
						{
							search: 'const host = "localhost";',
							replace: 'const host = "0.0.0.0";',
						},
					],
				},
			],
		},
		result: {
			ok: true,
			files: [
				{
					path: "src/config.ts",
					diff: "--- src/config.ts\n+++ src/config.ts\n@@ -1,3 +1,3 @@\n export const settings = {\n-\tconst timeout = 30;\n+\tconst timeout = 60;\n };\n",
				},
				{
					path: "src/server.ts",
					diff: '--- src/server.ts\n+++ src/server.ts\n@@ -1,3 +1,3 @@\n export const server = {\n-\tconst host = "localhost";\n+\tconst host = "0.0.0.0";\n };\n',
				},
			],
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/Edited 2 files/)).toBeInTheDocument();
		// Both server diffs must render. FileDiff's internals aren't
		// queryable from jsdom; count the testid'd wrappers instead.
		expect(canvas.getAllByTestId("edit-file-diff")).toHaveLength(2);
	},
};

export const EditFilesServerDiffNoOp: Story = {
	args: {
		name: "edit_files",
		status: "completed",
		args: {
			files: [
				{
					path: "src/unchanged.ts",
					edits: [
						{
							search: "same",
							replace: "same",
						},
					],
				},
			],
		},
		result: {
			ok: true,
			files: [{ path: "src/unchanged.ts", diff: "" }],
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/Edited unchanged\.ts/)).toBeInTheDocument();
		expect(canvas.queryAllByTestId("edit-file-diff")).toHaveLength(0);
	},
};

export const EditFilesFallbackToSynthetic: Story = {
	args: {
		name: "edit_files",
		status: "completed",
		args: {
			files: [
				{
					path: "src/legacy.ts",
					edits: [
						{
							search: "const timeout = 30;",
							replace: "const timeout = 60;",
						},
					],
				},
			],
		},
		result: { ok: true },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/Edited legacy\.ts/)).toBeInTheDocument();
		expect(canvas.getAllByTestId("edit-file-diff")).toHaveLength(1);
	},
};

export const EditFilesServerDiffPartialFallback: Story = {
	args: {
		name: "edit_files",
		status: "completed",
		args: {
			files: [
				{
					path: "src/config.ts",
					edits: [
						{
							search: "const timeout = 30;",
							replace: "const timeout = 60;",
						},
					],
				},
				{
					path: "src/server.ts",
					edits: [
						{
							search: 'const host = "localhost";',
							replace: 'const host = "0.0.0.0";',
						},
					],
				},
			],
		},
		result: {
			ok: true,
			files: [
				{
					path: "src/config.ts",
					diff: "--- src/config.ts\n+++ src/config.ts\n@@ -1,3 +1,3 @@\n export const settings = {\n-\tconst timeout = 30;\n+\tconst timeout = 60;\n };\n",
				},
			],
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/Edited 2 files/)).toBeInTheDocument();
		expect(canvas.getAllByTestId("edit-file-diff")).toHaveLength(2);
	},
};

// ---------------------------------------------------------------------------
// Computer tool stories
// ---------------------------------------------------------------------------

import { DESKTOP_SCREENSHOT_BASE64 } from "./__fixtures__/desktopScreenshot";

export const ComputerScreenshot: Story = {
	args: {
		name: "computer",
		status: "completed",
		result: {
			data: DESKTOP_SCREENSHOT_BASE64,
			text: "",
			mime_type: "image/jpeg",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Screenshot")).toBeInTheDocument();
		const img = canvas.getByRole("img", {
			name: "Screenshot from computer tool",
		});
		expect(img).toBeInTheDocument();
		expect(img.getAttribute("src")).toContain("data:image/jpeg;base64,");
		// Image should be wrapped in a button that opens the lightbox.
		const button = img.closest("button");
		expect(button).toBeInTheDocument();
	},
};

export const ComputerRunning: Story = {
	args: {
		name: "computer",
		status: "running",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Taking screenshot…")).toBeInTheDocument();
		expect(canvasElement.querySelector(".animate-spin")).not.toBeNull();
	},
};

export const ComputerTextFallback: Story = {
	args: {
		name: "computer",
		status: "completed",
		result: {
			data: "",
			text: "Screen resolution: 1920x1080\nActive window: Terminal",
			mime_type: "image/png",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Text-only results are collapsed by default (no image).
		const toggle = canvas.getByRole("button", { name: "Screenshot" });
		expect(toggle).toBeInTheDocument();
		expect(canvas.queryByRole("img")).toBeNull();

		await userEvent.click(toggle);
		expect(
			canvas.getByText(/Screen resolution: 1920x1080/),
		).toBeInTheDocument();
	},
};

export const ComputerError: Story = {
	args: {
		name: "computer",
		status: "error",
		isError: true,
		result: {
			data: "",
			text: "",
			mime_type: "image/png",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Screenshot")).toBeInTheDocument();
		// Warning icon should be present, not the old destructive style.
		expect(
			canvasElement.querySelector(".lucide-triangle-alert"),
		).not.toBeNull();
	},
};

export const ComputerArrayResult: Story = {
	args: {
		name: "computer",
		status: "completed",
		result: [
			{
				type: "image",
				data: DESKTOP_SCREENSHOT_BASE64,
				mime_type: "image/jpeg",
			},
			{ type: "text", text: "Clicked on button" },
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const img = canvas.getByRole("img", {
			name: "Screenshot from computer tool",
		});
		expect(img).toBeInTheDocument();
		expect(img.getAttribute("src")).toContain("data:image/jpeg;base64,");
	},
};

export const ComputerPromotedAttachmentArrayResult: Story = {
	args: {
		name: "computer",
		status: "completed",
		result: [
			{
				type: "image",
				data: DESKTOP_SCREENSHOT_BASE64,
				mime_type: "image/jpeg",
				attachment_file_id: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
				attachment_name: "screenshot-2026-04-21T00-00-00Z.png",
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const toggle = canvas.getByRole("button", { name: "Screenshot" });
		expect(toggle).toBeInTheDocument();
		expect(
			canvas.queryByRole("img", { name: "Screenshot from computer tool" }),
		).toBeNull();

		await userEvent.click(toggle);
		expect(
			canvas.getByText("Attached screenshot-2026-04-21T00-00-00Z.png"),
		).toBeInTheDocument();
	},
};

export const AttachFileLabelFallsBackToPathBasename: Story = {
	args: {
		name: "attach_file",
		status: "completed",
		args: {
			path: "docs/runbooks/incident.md",
		},
		result: {},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Attached incident.md")).toBeInTheDocument();
	},
};

// ---------------------------------------------------------------------------
// Tool failure display stories
// ---------------------------------------------------------------------------

export const GenericToolFailed: Story = {
	args: {
		name: "some_custom_tool",
		status: "error",
		isError: true,
		args: { input: "test data" },
		result: { error: "Connection refused: could not reach upstream service" },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Should show "Failed" badge instead of scary red alert.
		expect(
			canvasElement.querySelector(".lucide-triangle-alert"),
		).not.toBeNull();
		// Label should NOT have destructive color.
		const label = canvas.getByText("some_custom_tool");
		expect(label.className).not.toContain("text-content-destructive");
		// Error icon should not be present (replaced by warning triangle).
		expect(canvasElement.querySelector(".lucide-circle-alert")).toBeNull();
	},
};

export const GenericToolFailedNoResult: Story = {
	args: {
		name: "web_search",
		status: "error",
		isError: true,
	},
	play: async ({ canvasElement }) => {
		expect(
			canvasElement.querySelector(".lucide-triangle-alert"),
		).not.toBeNull();
	},
};

export const GenericToolStringError: Story = {
	args: {
		name: "web_search",
		status: "error",
		isError: true,
		result: "Network unreachable",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("img", { name: "Web search failed" }),
		).toBeVisible();
	},
};

export const GenericMCPToolStringError: Story = {
	args: {
		name: "linear__list_issues",
		status: "error",
		isError: true,
		result: "Authentication token expired",
		mcpServerConfigId: "mcp-server-1",
		mcpServers: sampleMCPServers,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByRole("img", { name: "List issues failed" }),
		).toBeVisible();
	},
};

const longCodeLine =
	'export const config = { apiUrl: "https://coder.example.com/api/v2/workspaces", token: "abcdefghijklmnopqrstuvwxyz0123456789_ABCDEFGHIJKLMNOPQRSTUVWXYZ_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", retries: 5 };';

const tallWideFileContent = [
	longCodeLine,
	...Array.from({ length: 40 }, (_, i) => `const line${i} = ${i};`),
].join("\n");

export const ReadFileLongLine: Story = {
	args: {
		name: "read_file",
		args: { path: "site/src/config.ts" },
		result: { content: longCodeLine },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: /Read config.ts/i }),
		);
		await expectDiffText(canvasElement, "apiUrl");
	},
};

export const ReadFileTallAndWide: Story = {
	args: {
		name: "read_file",
		args: { path: "site/src/config.ts" },
		result: { content: tallWideFileContent },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: /Read config.ts/i }),
		);
		await expectDiffText(canvasElement, "apiUrl");
		await waitFor(() => {
			const target = [
				...canvasElement.querySelectorAll<HTMLElement>(
					"[data-radix-scroll-area-viewport]",
				),
			].find(
				(v) => v.scrollWidth > v.clientWidth && v.scrollHeight > v.clientHeight,
			);
			if (!target) {
				throw new Error("Expected a viewport overflowing on both axes.");
			}
		});
	},
};

export const GenericToolLongOutput: Story = {
	args: {
		name: "some_custom_tool",
		args: { query: "lookup" },
		result: { value: longCodeLine },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: /some_custom_tool/i }),
		);
		await expectDiffText(canvasElement, "apiUrl");
	},
};

export const SubagentWaitTimedOut: Story = {
	args: {
		name: "wait_agent",
		status: "error",
		isError: true,
		args: { chat_id: "timed-out-child" },
		result: "timed out waiting for delegated subagent completion",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Should show clock icon for timeout.
		expect(canvasElement.querySelector(".lucide-clock")).not.toBeNull();
		// Should NOT show red alert icon.
		expect(canvasElement.querySelector(".lucide-circle-alert")).toBeNull();
		// Should show timeout verb.
		expect(canvas.getByText(/Timed out waiting for/)).toBeInTheDocument();
	},
};

export const SubagentWaitTimedOutWithTitle: Story = {
	args: {
		name: "wait_agent",
		status: "error",
		isError: true,
		args: { chat_id: "timed-out-child" },
		result: {
			chat_id: "timed-out-child",
			error: "timed out waiting for delegated subagent completion",
			title: "Fix login bug",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvasElement.querySelector(".lucide-clock")).not.toBeNull();
		expect(canvas.getByText(/Timed out waiting for/)).toBeInTheDocument();
		expect(canvas.getByText("Fix login bug")).toBeInTheDocument();
	},
};

export const SubagentWaitTimedOutTitleFromMap: Story = {
	args: {
		name: "wait_agent",
		status: "error",
		isError: true,
		args: { chat_id: "timed-out-child" },
		result: "timed out waiting for delegated subagent completion",
		subagentTitles: new Map([["timed-out-child", "Refactor auth module"]]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Refactor auth module")).toBeInTheDocument();
		expect(canvas.getByText(/Timed out waiting for/)).toBeInTheDocument();
	},
};

export const SubagentWaitTimedOutStructured: Story = {
	args: {
		name: "wait_agent",
		status: "completed",
		isError: false,
		args: { chat_id: "timed-out-child" },
		result: {
			chat_id: "timed-out-child",
			title: "Fix login bug",
			status: "running",
			timed_out: true,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Should show clock icon for timeout.
		expect(canvasElement.querySelector(".lucide-clock")).not.toBeNull();
		// Should NOT show red alert icon.
		expect(canvasElement.querySelector(".lucide-circle-alert")).toBeNull();
		// Should show timeout verb.
		expect(canvas.getByText(/Timed out waiting for/)).toBeInTheDocument();
		expect(canvas.getByText("Fix login bug")).toBeInTheDocument();
	},
};

export const SubagentSpawnError: Story = {
	args: {
		name: "spawn_agent",
		status: "error",
		isError: true,
		args: {
			title: "Database migration",
			prompt: "Run the pending migrations.",
		},
		result: {
			chat_id: "failed-child",
			error: "workspace not found",
			status: "error",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Should show the muted X icon, not the red alert.
		expect(canvasElement.querySelector(".lucide-circle-x")).not.toBeNull();
		expect(canvasElement.querySelector(".lucide-circle-alert")).toBeNull();
		// Should show error verb.
		expect(canvas.getByText(/Failed to spawn/)).toBeInTheDocument();
		expect(canvas.getByText("Database migration")).toBeInTheDocument();
	},
};

export const SubagentWaitError: Story = {
	args: {
		name: "wait_agent",
		status: "error",
		isError: true,
		args: { chat_id: "error-child" },
		result: {
			chat_id: "error-child",
			error: "subagent crashed unexpectedly",
			status: "error",
			title: "Lint codebase",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvasElement.querySelector(".lucide-circle-x")).not.toBeNull();
		expect(canvas.getByText(/Failed waiting for/)).toBeInTheDocument();
		expect(canvas.getByText("Lint codebase")).toBeInTheDocument();
	},
};

export const MCPToolFailedUnifiedStyle: Story = {
	args: {
		name: "linear__list_issues",
		status: "error",
		isError: true,
		args: { project: "backend" },
		result: { error: "Authentication token expired" },
		mcpServerConfigId: "mcp-server-1",
		mcpServers: sampleMCPServers,
	},
	play: async ({ canvasElement }) => {
		// Should show warning triangle icon.
		expect(
			canvasElement.querySelector(".lucide-triangle-alert"),
		).not.toBeNull();
		// Icon should NOT be red.
		expect(canvasElement.querySelector(".text-content-destructive")).toBeNull();
	},
};

// ---------------------------------------------------------------------------
// spawn_computer_use_agent stories
// ---------------------------------------------------------------------------

export const SpawnComputerUseAgentRunning: Story = {
	args: {
		name: "spawn_computer_use_agent",
		status: "running",
		args: {
			title: "Visual regression check",
			prompt:
				"Open the browser and check for visual regressions on the dashboard page.",
		},
		result: {
			chat_id: "desktop-child-1",
			title: "Visual regression check",
			status: "pending",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/Using the computer/)).toBeInTheDocument();
		expect(canvasElement.querySelector(".lucide-monitor")).not.toBeNull();
	},
};

export const SpawnComputerUseAgentCompleted: Story = {
	args: {
		name: "spawn_computer_use_agent",
		status: "completed",
		args: {
			title: "Visual regression check",
			prompt:
				"Open the browser and check for visual regressions on the dashboard page.",
		},
		result: {
			chat_id: "desktop-child-1",
			title: "Visual regression check",
			status: "completed",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/Spawned/)).toBeInTheDocument();
		expect(canvas.getByText(/Visual regression check/)).toBeInTheDocument();
		expect(canvas.getByRole("link", { name: "View agent" })).toHaveAttribute(
			"href",
			"/agents/desktop-child-1",
		);
	},
};

export const SpawnComputerUseAgentError: Story = {
	args: {
		name: "spawn_computer_use_agent",
		status: "error",
		isError: true,
		result: {
			chat_id: "desktop-child-1",
			status: "error",
		},
	},
	play: async ({ canvasElement }) => {
		expect(canvasElement.querySelector(".lucide-circle-x")).not.toBeNull();
	},
};

// ---------------------------------------------------------------------------
// wait_agent with computer-use subagent stories
// ---------------------------------------------------------------------------

export const WaitAgentComputerUseRunning: Story = {
	args: {
		name: "wait_agent",
		status: "running",
		args: {
			chat_id: "desktop-child-1",
		},
		result: {
			chat_id: "desktop-child-1",
			status: "pending",
		},
		subagentVariants: new Map([["desktop-child-1", "computer_use"]]),
	},
	decorators: [
		(Story) => (
			<DesktopPanelContext.Provider
				value={{
					desktopChatId: "desktop-child-1",
					onOpenDesktop: fn(),
				}}
			>
				<Story />
			</DesktopPanelContext.Provider>
		),
	],
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/Using the computer/)).toBeInTheDocument();
		// Running state shows the monitor icon instead of a spinner.
		expect(canvasElement.querySelector(".lucide-monitor")).not.toBeNull();
		// The VNC preview container should mount (the connection will
		// stay in "connecting" state without a real WebSocket, which
		// is expected; we only verify the container renders).
		await waitFor(() => {
			expect(
				canvas.getByRole("button", { name: "Open desktop tab" }),
			).toBeInTheDocument();
		});
	},
};

export const WaitAgentComputerUseCompletedNoRecording: Story = {
	args: {
		name: "wait_agent",
		status: "completed",
		args: { chat_id: "desktop-child-1" },
		result: {
			chat_id: "desktop-child-1",
			title: "Set up environment",
			status: "waiting",
			report: "Configured the dev environment.",
		},
		subagentVariants: new Map([["desktop-child-1", "computer_use"]]),
	},
	decorators: [
		(Story) => (
			<DesktopPanelContext.Provider
				value={{
					desktopChatId: "desktop-child-1",
					onOpenDesktop: fn(),
				}}
			>
				<Story />
			</DesktopPanelContext.Provider>
		),
	],
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.queryByRole("button", { name: "View recording" })).toBeNull();
		expect(
			canvas.queryByRole("button", { name: "Open desktop tab" }),
		).toBeNull();
	},
};

export const WaitAgentComputerUseTimedOutNoRecording: Story = {
	args: {
		name: "wait_agent",
		status: "error",
		isError: true,
		args: { chat_id: "desktop-child-1" },
		result: {
			chat_id: "desktop-child-1",
			title: "Set up environment",
			status: "pending",
			error: "timed out waiting for agent",
		},
		subagentVariants: new Map([["desktop-child-1", "computer_use"]]),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.queryByRole("button", { name: "View recording" })).toBeNull();
	},
};

// ---------------------------------------------------------------------------
// read_skill stories
// ---------------------------------------------------------------------------

export const ReadSkillRunning: Story = {
	args: {
		name: "read_skill",
		status: "running",
		args: { name: "deep-review" },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/Reading skill deep-review/)).toBeInTheDocument();
	},
};

export const ReadSkillCompleted: Story = {
	args: {
		name: "read_skill",
		status: "completed",
		args: { name: "deep-review" },
		result: {
			name: "deep-review",
			body: "## Deep Review Skill\n\nReview the code changes thoroughly.\n\n1. Check for correctness\n2. Verify tests\n3. Ensure style consistency",
			files: ["roles/security-reviewer.md", "templates/review-checklist.md"],
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/Read skill deep-review/)).toBeInTheDocument();
		// Expand the collapsible to verify markdown body renders.
		const toggle = canvas.getByRole("button");
		await userEvent.click(toggle);
		await waitFor(() => {
			expect(canvas.getByText("Deep Review Skill")).toBeInTheDocument();
		});
	},
};

export const ReadSkillError: Story = {
	args: {
		name: "read_skill",
		status: "error",
		isError: true,
		args: { name: "nonexistent-skill" },
		result: { error: 'skill "nonexistent-skill" not found' },
	},
	play: async ({ canvasElement }) => {
		expect(
			canvasElement.querySelector(".lucide-triangle-alert"),
		).not.toBeNull();
	},
};

// ---------------------------------------------------------------------------
// read_skill_file stories
// ---------------------------------------------------------------------------

export const ReadSkillFileRunning: Story = {
	args: {
		name: "read_skill_file",
		status: "running",
		args: { name: "deep-review", path: "roles/security-reviewer.md" },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByText(/Reading deep-review\/roles\/security-reviewer\.md/),
		).toBeInTheDocument();
	},
};

export const ReadSkillFileCompleted: Story = {
	args: {
		name: "read_skill_file",
		status: "completed",
		args: { name: "deep-review", path: "roles/security-reviewer.md" },
		result: {
			content:
				"# Security Reviewer Role\n\nFocus on authentication, authorization, and input validation.\n\n## Checklist\n- [ ] Verify auth middleware\n- [ ] Check for SQL injection\n- [ ] Validate user inputs",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByText(/Read deep-review\/roles\/security-reviewer\.md/),
		).toBeInTheDocument();
		// Expand the collapsible to verify markdown content renders.
		const toggle = canvas.getByRole("button");
		await userEvent.click(toggle);
		await waitFor(() => {
			expect(canvas.getByText("Security Reviewer Role")).toBeInTheDocument();
		});
	},
};

export const ReadSkillFileError: Story = {
	args: {
		name: "read_skill_file",
		status: "error",
		isError: true,
		args: { name: "deep-review", path: "missing-file.md" },
		result: { error: "file not found" },
	},
};

// ---------------------------------------------------------------------------
// start_workspace stories
// ---------------------------------------------------------------------------

export const StartWorkspaceRunning: Story = {
	args: {
		name: "start_workspace",
		status: "running",
	},
	decorators: [
		(Story) => (
			<ChatWorkspaceContext value={{ workspaceId: "test-workspace-id" }}>
				<Story />
			</ChatWorkspaceContext>
		),
	],
	parameters: {
		queries: [
			{
				key: ["workspace", "test-workspace-id"],
				data: {
					id: "test-workspace-id",
					latest_build: {
						id: "test-build-id",
						status: "starting",
					},
				},
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Starting workspace…")).toBeInTheDocument();
	},
};

export const StartWorkspaceCompleted: Story = {
	args: {
		name: "start_workspace",
		status: "completed",
		result: {
			started: true,
			workspace_name: "my-project",
			agent_status: "ready",
			build_id: "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
		},
	},
	parameters: {
		queries: [
			{
				key: [
					"workspaceBuilds",
					"a1b2c3d4-e5f6-7890-abcd-ef1234567890",
					"logs",
				],
				data: [],
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Started my-project")).toBeInTheDocument();
	},
};

export const StartWorkspaceLegacy: Story = {
	args: {
		name: "start_workspace",
		status: "completed",
		result: {
			started: true,
			workspace_name: "legacy-workspace",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Started legacy-workspace")).toBeInTheDocument();
	},
};

export const StartWorkspaceError: Story = {
	args: {
		name: "start_workspace",
		status: "error",
		isError: true,
		result: {
			error: "workspace was deleted; use create_workspace to make a new one",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Failed to start workspace")).toBeInTheDocument();
	},
};

export const StartWorkspaceBuildFailed: Story = {
	args: {
		name: "start_workspace",
		status: "completed",
		result: {
			error: "workspace start build failed: terraform apply failed",
			build_id: "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
		},
	},
	parameters: {
		queries: [
			{
				key: [
					"workspaceBuilds",
					"a1b2c3d4-e5f6-7890-abcd-ef1234567890",
					"logs",
				],
				data: [],
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Failed to start workspace")).toBeInTheDocument();
	},
};

export const StartWorkspaceQuotaReached: Story = {
	args: {
		name: "start_workspace",
		status: "completed",
		result: {
			error_code: "INSUFFICIENT_QUOTA",
			error: "workspace start build failed: insufficient quota",
			title: "Workspace quota reached",
			message:
				"Coder could not start this workspace because your workspace quota is full.",
			build_id: "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
			quota: {
				credits_consumed: 40,
				budget: 40,
			},
		},
	},
	parameters: {
		queries: [
			{
				key: [
					"workspaceBuilds",
					"a1b2c3d4-e5f6-7890-abcd-ef1234567890",
					"logs",
				],
				data: [],
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Workspace quota reached")).toBeInTheDocument();
	},
};

// ---------------------------------------------------------------------------
// create_workspace stories
// ---------------------------------------------------------------------------

export const CreateWorkspaceRunning: Story = {
	args: {
		name: "create_workspace",
		status: "running",
	},
	decorators: [
		(Story) => (
			<ChatWorkspaceContext value={{ workspaceId: "test-workspace-id" }}>
				<Story />
			</ChatWorkspaceContext>
		),
	],
	parameters: {
		queries: [
			{
				key: ["workspace", "test-workspace-id"],
				data: {
					id: "test-workspace-id",
					latest_build: {
						id: "test-build-id",
						status: "starting",
					},
				},
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Creating workspace…")).toBeInTheDocument();
	},
};

export const CreateWorkspaceCompleted: Story = {
	args: {
		name: "create_workspace",
		status: "completed",
		result: {
			created: true,
			workspace_name: "my-project",
			build_id: "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
		},
	},
	parameters: {
		queries: [
			{
				key: [
					"workspaceBuilds",
					"a1b2c3d4-e5f6-7890-abcd-ef1234567890",
					"logs",
				],
				data: [],
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Created my-project")).toBeInTheDocument();
	},
};

export const CreateWorkspaceQuotaReached: Story = {
	args: {
		name: "create_workspace",
		status: "completed",
		result: {
			error_code: "INSUFFICIENT_QUOTA",
			error: "workspace build failed: insufficient quota",
			title: "Workspace quota reached",
			message:
				"Coder could not create this workspace because your workspace quota is full.",
			build_id: "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
			quota: {
				credits_consumed: 40,
				budget: 40,
			},
		},
	},
	parameters: {
		queries: [
			{
				key: [
					"workspaceBuilds",
					"a1b2c3d4-e5f6-7890-abcd-ef1234567890",
					"logs",
				],
				data: [],
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Workspace quota reached")).toBeInTheDocument();
	},
};

export const CreateWorkspaceLegacy: Story = {
	args: {
		name: "create_workspace",
		status: "completed",
		result: {
			created: true,
			workspace_name: "legacy-workspace",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Created legacy-workspace")).toBeInTheDocument();
	},
};

export const CreateWorkspaceAlreadyExists: Story = {
	args: {
		name: "create_workspace",
		status: "completed",
		result: {
			created: false,
			workspace_name: "my-project",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(
			canvas.getByText("Workspace my-project already exists"),
		).toBeInTheDocument();
	},
};

export const CreateWorkspaceError: Story = {
	args: {
		name: "create_workspace",
		status: "error",
		isError: true,
		result: {
			error: "template not found",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Failed to create workspace")).toBeInTheDocument();
	},
};

export const CreateWorkspaceBuildFailed: Story = {
	args: {
		name: "create_workspace",
		status: "completed",
		result: {
			error: "workspace build failed: terraform apply failed",
			build_id: "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
		},
	},
	parameters: {
		queries: [
			{
				key: [
					"workspaceBuilds",
					"a1b2c3d4-e5f6-7890-abcd-ef1234567890",
					"logs",
				],
				data: [],
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Failed to create workspace")).toBeInTheDocument();
	},
};

export const AllToolIconsTranscript: Story = {
	render: () => (
		<ChatWorkspaceContext value={{ workspaceId: "test-workspace-id" }}>
			<DesktopPanelContext.Provider
				value={{ desktopChatId: "desktop-child", onOpenDesktop: fn() }}
			>
				<div className="flex flex-col gap-2">
					<BlockList
						blocks={[
							{
								type: "thinking",
								text: "Thinking\nReviewing the available tools and grouping them by category.",
							},
						]}
						tools={[]}
						keyPrefix="all-tool-icons-thinking"
					/>
					{allToolShowcaseItems.map((tool, index) => (
						<Tool
							key={`${tool.name}-${index}`}
							name={tool.name}
							status={tool.status ?? "completed"}
							args={tool.args}
							result={tool.result}
							isError={tool.isError}
							killedBySignal={tool.killedBySignal}
							modelIntent={tool.modelIntent}
							parsedCommands={tool.parsedCommands}
							subagentVariants={tool.subagentVariants}
							shellToolDisplayMode="always_collapsed"
							codeDiffDisplayMode="always_collapsed"
							showDesktopPreviews={false}
						/>
					))}
				</div>
			</DesktopPanelContext.Provider>
		</ChatWorkspaceContext>
	),
	parameters: {
		queries: [
			{
				key: ["workspace", "test-workspace-id"],
				data: {
					id: "test-workspace-id",
					latest_build: {
						id: "test-build-id",
						status: "running",
					},
				},
			},
			{
				key: [
					"workspaceBuilds",
					"a1b2c3d4-e5f6-7890-abcd-ef1234567890",
					"logs",
				],
				data: [],
			},
		],
	},
};
