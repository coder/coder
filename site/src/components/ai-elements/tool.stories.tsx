import type { Meta, StoryObj } from "@storybook/react-vite";
import { Tool } from "./tool";

const executeCommand = "git fetch origin";
const subagentReport = `
## Workspace startup report

1. Agent connected after network retries.
2. \`docker pull\` failed due to expired auth token.
3. Re-authentication fixed image pulls and startup completed.
`;

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
};

export default meta;
type Story = StoryObj<typeof Tool>;

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
};

export const WaitForExternalAuthRunning: Story = {
	args: {
		name: "wait_for_external_auth",
		status: "running",
		result: {
			provider_display_name: "GitHub",
			authenticated: false,
		},
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

export const SubagentRunning: Story = {
	args: {
		name: "subagent",
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
};

export const SubagentReport: Story = {
	args: {
		name: "subagent_report",
		status: "completed",
		args: {
			report: subagentReport,
		},
		result: {
			title: "Sub-agent report",
		},
	},
};
