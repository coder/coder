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

const mockCoderMCP: TypesGen.MCPServerConfig = {
	...MockMCPServerConfig,
	id: "mcp-coder",
	display_name: "Coder",
	description: "Workspaces, templates and tasks",
	auth_type: "oauth2",
};
const mockGitHubMCP: TypesGen.MCPServerConfig = {
	...MockMCPServerConfig,
	id: "mcp-github",
	display_name: "GitHub",
	description: "Repositories, pull requests and issues",
	icon_url: "/icon/github.svg",
	auth_type: "oauth2",
	auth_connected: true,
};
const mockGranolaMCP: TypesGen.MCPServerConfig = {
	...MockMCPServerConfig,
	id: "mcp-granola",
	display_name: "Granola",
	availability: "force_on",
	auth_type: "oauth2",
};
const mockImageMCP: TypesGen.MCPServerConfig = {
	...MockMCPServerConfig,
	id: "mcp-image",
	display_name: "Image",
	auth_connected: true,
};
const mockRegistryMCP: TypesGen.MCPServerConfig = {
	...MockMCPServerConfig,
	id: "mcp-registry",
	display_name: "Registry",
	auth_type: "api_key",
	auth_connected: true,
};
const mockServers = [
	mockCoderMCP,
	mockGitHubMCP,
	mockGranolaMCP,
	mockImageMCP,
	mockRegistryMCP,
];

const meta = {
	title: "pages/AgentsPage/AgentSettingsMCPConnectionsPageView",
	component: AgentSettingsMCPConnectionsPageView,
	args: {
		organizations: [MockDefaultOrganization],
		organization: MockDefaultOrganization,
		onOrganizationChange: fn(),
		servers: mockServers,
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

export const Mobile: Story = {
	parameters: {
		viewport: { defaultViewport: "mobile1" },
		pixel: { matrix: { viewports: ["phone"] } },
	},
};

// Between the @lg and @2xl container widths: Description shows, Status does not.
export const NarrowContainer: Story = {
	decorators: [
		(Story) => (
			<div className="w-[580px]">
				<Story />
			</div>
		),
	],
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
		servers: [mockImageMCP, mockRegistryMCP],
	},
};

export const NoOrganization: Story = {
	args: {
		organizations: [],
		organization: undefined,
		servers: undefined,
	},
};

export const DisconnectDialogOpen: Story = {
	args: {
		disconnectTarget: mockGitHubMCP,
	},
};

export const Disconnecting: Story = {
	args: {
		disconnectTarget: mockGitHubMCP,
		isDisconnecting: true,
	},
};

/** A failed refetch keeps the last list below the error. */
export const RefetchError: Story = {
	args: {
		error: mockApiError({ message: "Failed to load MCP servers." }),
	},
};

export const Retrying: Story = {
	args: {
		error: mockApiError({ message: "Failed to load MCP servers." }),
		isRetrying: true,
	},
};

export const OneServerWithoutSignIn: Story = {
	args: {
		servers: [mockCoderMCP, mockGitHubMCP, mockImageMCP],
	},
};
