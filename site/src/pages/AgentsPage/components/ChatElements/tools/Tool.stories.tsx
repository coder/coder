import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, spyOn, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { DesktopPanelContext } from "./DesktopPanelContext";
import { Tool } from "./Tool";

const executeCommand = "git fetch origin";
const meta: Meta<typeof Tool> = {
	title: "pages/AgentsPage/ChatElements/tools/Tool",
	component: Tool,
	decorators: [
		(Story) => (
			<div className="max-w-3xl rounded-lg border border-solid border-border-default bg-surface-primary p-4">
				<Story />
			</div>
		),
	],
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

export const ExecuteSuccess: Story = {
	args: {
		result: {
			output:
				"From github.com:coder/coder\n * [new branch]      feature/agent-ui -> origin/feature/agent-ui",
		},
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

export const SubagentRequestMetadata: Story = {
	args: {
		name: "spawn_agent",
		args: undefined,
		result: {
			chat_id: "child-chat-id",
			status: "completed",
			request_id: "request-123",
			duration_ms: 1530,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Worked for 2s")).toBeInTheDocument();
	},
};

export const SubagentAwaitRequestMetadata: Story = {
	args: {
		name: "wait_agent",
		args: undefined,
		result: {
			chat_id: "child-chat-id",
			status: "completed",
			request_id: "request-123",
			duration_ms: 1530,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Worked for 2s")).toBeInTheDocument();
	},
};

export const SubagentMessageRequestMetadata: Story = {
	args: {
		name: "message_agent",
		args: undefined,
		result: {
			chat_id: "child-chat-id",
			status: "completed",
			request_id: "request-123",
			duration_ms: 1530,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Worked for 2s")).toBeInTheDocument();
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

// ---------------------------------------------------------------------------
// SubagentTerminate stories
// ---------------------------------------------------------------------------

export const SubagentTerminate: Story = {
	args: {
		name: "close_agent",
		args: undefined,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/Terminated/)).toBeInTheDocument();
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
		// Result should be collapsed by default.
		const toggle = canvas.getByRole("button");
		expect(toggle).toBeInTheDocument();
		// Expand to see result content.
		await userEvent.click(toggle);
		// @pierre/diffs renders inside a Shadow DOM (<diffs-container>)
		// so textContent on the host element can't see the content.
		// Query into the shadow root to verify the JSON rendered.
		await waitFor(() => {
			const shadow = canvasElement.querySelector("diffs-container")?.shadowRoot;
			expect(shadow?.textContent).toContain("Fix auth flow");
		});
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
		// No toggle button when there is no result content.
		expect(canvasElement.querySelector("button")).toBeNull();
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
		args: {
			path: "src/utils/helpers.ts",
			content:
				"export function greet(name: string): string {\n  return `Hello, ${name}!`;\n}\n",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/Wrote helpers\.ts/)).toBeInTheDocument();
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
		const toggle = canvas.getByRole("button", { name: /Screenshot/ });
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
		expect(canvas.getByText(/Spawning/)).toBeInTheDocument();
		expect(canvasElement.querySelector(".animate-spin")).not.toBeNull();
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
			duration_ms: "12400",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText(/Spawned/)).toBeInTheDocument();
		expect(canvas.getByText(/Visual regression check/)).toBeInTheDocument();
		expect(canvas.getByText("Worked for 12s")).toBeInTheDocument();
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
		computerUseSubagentIds: new Set(["desktop-child-1"]),
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
		// is expected — we only verify the container renders).
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
		computerUseSubagentIds: new Set(["desktop-child-1"]),
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
		computerUseSubagentIds: new Set(["desktop-child-1"]),
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
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Started my-project")).toBeInTheDocument();
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
};
