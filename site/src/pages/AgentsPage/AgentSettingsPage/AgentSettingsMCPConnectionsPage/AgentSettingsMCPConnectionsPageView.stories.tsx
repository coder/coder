import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import { MockMCPServerConfig } from "#/testHelpers/chatEntities";
import {
	MockDefaultOrganization,
	MockOrganization2,
	mockApiError,
} from "#/testHelpers/entities";
import { AgentSettingsMCPConnectionsPageView } from "./AgentSettingsMCPConnectionsPageView";

const buildServer = (
	overrides: Partial<TypesGen.MCPServerConfig> &
		Pick<TypesGen.MCPServerConfig, "id" | "display_name">,
): TypesGen.MCPServerConfig => ({
	...MockMCPServerConfig,
	auth_type: "oauth2",
	...overrides,
});

const servers = [
	buildServer({
		id: "mcp-coder",
		display_name: "Coder",
		description: "Workspaces, templates and tasks",
	}),
	buildServer({
		id: "mcp-github",
		display_name: "GitHub",
		description: "Repositories, pull requests and issues",
		icon_url: "/icon/github.svg",
		auth_connected: true,
	}),
	buildServer({
		id: "mcp-granola",
		display_name: "Granola",
		availability: "force_on",
	}),
	buildServer({
		id: "mcp-linear",
		display_name: "Linear",
		description: "Issues, projects and cycles",
		auth_connected: true,
	}),
	buildServer({
		id: "mcp-image",
		display_name: "Image",
		auth_type: "none",
		auth_connected: true,
	}),
	buildServer({
		id: "mcp-registry",
		display_name: "Registry",
		auth_type: "api_key",
		auth_connected: true,
	}),
	buildServer({
		id: "mcp-slackbot",
		display_name: "Slackbot",
		auth_type: "none",
		auth_connected: true,
	}),
];

const meta = {
	title: "pages/AgentsPage/AgentSettingsMCPConnectionsPageView",
	component: AgentSettingsMCPConnectionsPageView,
	args: {
		organizations: [MockDefaultOrganization],
		organization: MockDefaultOrganization,
		onOrganizationChange: fn(),
		servers,
		error: undefined,
		isLoading: false,
		isRetrying: false,
		onRetry: fn(),
		connectingServerId: null,
		onConnect: fn(),
		onDisconnect: fn(),
		disconnectTarget: null,
		isDisconnecting: false,
		onConfirmDisconnect: fn(),
		onCancelDisconnect: fn(),
	},
} satisfies Meta<typeof AgentSettingsMCPConnectionsPageView>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const MultipleOrganizations: Story = {
	args: {
		organizations: [MockDefaultOrganization, MockOrganization2],
	},
};

export const Connecting: Story = {
	args: {
		connectingServerId: "mcp-coder",
	},
};

export const Loading: Story = {
	args: {
		servers: undefined,
		isLoading: true,
	},
};

export const LoadError: Story = {
	args: {
		servers: undefined,
		error: mockApiError({ message: "Failed to load MCP servers." }),
	},
};

export const Empty: Story = {
	args: {
		servers: servers.filter((server) => server.auth_type !== "oauth2"),
	},
};

export const DisconnectDialogOpen: Story = {
	args: {
		disconnectTarget: servers[1],
	},
};
