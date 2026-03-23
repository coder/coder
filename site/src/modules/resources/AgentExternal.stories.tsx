import { chromatic } from "testHelpers/chromatic";
import { MockWorkspace, MockWorkspaceAgent } from "testHelpers/entities";
import { withDashboardProvider } from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { AgentExternal } from "./AgentExternal";

const mockCredentials = {
	command:
		'curl -fsSL "https://coder.example.com/api/v2/init-script/linux/amd64" | CODER_AGENT_TOKEN="test-token" sh',
	agent_token: "test-token",
	init_script_base_url: "https://coder.example.com/api/v2/init-script",
};

const credentialsQueryKey = [
	"workspaces",
	MockWorkspace.id,
	"agents",
	MockWorkspaceAgent.name,
	"credentials",
];

const meta: Meta<typeof AgentExternal> = {
	title: "modules/resources/AgentExternal",
	component: AgentExternal,
	args: {
		agent: {
			...MockWorkspaceAgent,
			status: "connecting",
			operating_system: "linux",
			architecture: "amd64",
		},
		workspace: MockWorkspace,
	},
	decorators: [withDashboardProvider],
	parameters: {
		chromatic,
		queries: [{ key: credentialsQueryKey, data: mockCredentials }],
	},
};

export default meta;
type Story = StoryObj<typeof AgentExternal>;

export const Linux: Story = {
	args: {
		agent: {
			...MockWorkspaceAgent,
			status: "connecting",
			operating_system: "linux",
			architecture: "amd64",
		},
	},
};

export const macOS: Story = {
	args: {
		agent: {
			...MockWorkspaceAgent,
			status: "connecting",
			operating_system: "darwin",
			architecture: "arm64",
		},
	},
};

export const Windows: Story = {
	args: {
		agent: {
			...MockWorkspaceAgent,
			status: "connecting",
			operating_system: "windows",
			architecture: "amd64",
		},
	},
};

export const Timeout: Story = {
	args: {
		agent: {
			...MockWorkspaceAgent,
			status: "timeout",
			operating_system: "linux",
			architecture: "amd64",
		},
	},
};
