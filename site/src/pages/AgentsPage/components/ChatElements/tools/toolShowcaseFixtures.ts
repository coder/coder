import type { Tool } from "./Tool";

// 1x1 solid coral (#FF6B6B) PNG encoded as base64.
const TEST_PNG_B64 =
	"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR4nGP4n539HwAHFwLVF8kc1wAAAABJRU5ErkJggg==";

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

export const allToolShowcaseItems: ToolShowcaseItem[] = [
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
		name: "list_agents",
		result: {
			agents: [{ id: "agent-1", title: "Workspace diagnostics" }],
			total: 1,
		},
	},
	{
		name: "list_subagent_models",
		result: {
			models: [
				{
					model_config_id: "model-1",
					display_name: "Fast Model",
					model: "fast-1",
					provider: "openai",
					is_default: true,
				},
			],
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
		name: "chat_cleared",
		args: { source: "manual" },
		result: { source: "manual" },
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
		name: "find_tools",
		args: { queries: ["github issues"] },
		result: {
			matches: [
				{
					name: "github__list_issues",
					description: "List issues in a GitHub repository.",
				},
			],
			activated: ["github__list_issues"],
			total_deferred: 12,
		},
	},
	{
		name: "unknown_tool",
		args: { example: true },
		result: { ok: true },
	},
];
