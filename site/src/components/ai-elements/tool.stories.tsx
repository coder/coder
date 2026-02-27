import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { Tool } from "./tool";

const executeCommand = "git fetch origin";
const meta: Meta<typeof Tool> = {
	title: "components/ai-elements/Tool",
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
