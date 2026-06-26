import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import { MockMCPServerConfig } from "#/testHelpers/chatEntities";
import { getDefaultMCPSelection, MCPServerPicker } from "./MCPServerPicker";

// ── Helpers ────────────────────────────────────────────────────

const now = "2026-03-19T12:00:00.000Z";

const createServerConfig = (
	overrides: Partial<TypesGen.MCPServerConfig> &
		Pick<TypesGen.MCPServerConfig, "id" | "display_name" | "slug">,
): TypesGen.MCPServerConfig => ({
	...MockMCPServerConfig,
	created_at: now,
	updated_at: now,
	...overrides,
});

// ── Fixtures ───────────────────────────────────────────────────

const sentryServer = createServerConfig({
	id: "mcp-sentry",
	display_name: "Sentry",
	slug: "sentry",
	description: "Error tracking and monitoring",
	icon_url: "/icon/widgets.svg",
	url: "https://mcp.sentry.io/sse",
	transport: "sse",
	auth_type: "oauth2",
	has_oauth2_secret: true,
	availability: "force_on",
	enabled: true,
	auth_connected: true,
});

const linearServer = createServerConfig({
	id: "mcp-linear",
	display_name: "Linear",
	slug: "linear",
	description: "Project management and issue tracking",
	url: "https://mcp.linear.app/v1",
	transport: "streamable_http",
	auth_type: "api_key",
	has_api_key: true,
	availability: "default_on",
	enabled: true,
	auth_connected: false,
});

const githubServer = createServerConfig({
	id: "mcp-github",
	display_name: "GitHub",
	slug: "github",
	description: "Code hosting and collaboration",
	icon_url: "/icon/github.svg",
	url: "https://api.githubcopilot.com/mcp/",
	transport: "streamable_http",
	auth_type: "oauth2",
	has_oauth2_secret: true,
	availability: "default_on",
	enabled: true,
	auth_connected: false,
});

const githubServerConnected = {
	...githubServer,
	auth_connected: true,
};

const slackServer = createServerConfig({
	id: "mcp-slack",
	display_name: "Slack",
	slug: "slack",
	description: "Team messaging and notifications",
	url: "https://mcp.slack.com/v1",
	transport: "streamable_http",
	auth_type: "oauth2",
	has_oauth2_secret: true,
	availability: "default_off",
	enabled: true,
	auth_connected: false,
});

const datadogServer = createServerConfig({
	id: "mcp-datadog",
	display_name: "Datadog",
	slug: "datadog",
	description: "Infrastructure monitoring and APM",
	url: "https://mcp.datadog.com/v1",
	transport: "streamable_http",
	auth_type: "none",
	availability: "default_off",
	enabled: true,
	auth_connected: false,
});

const disabledServer = createServerConfig({
	id: "mcp-disabled",
	display_name: "Disabled Server",
	slug: "disabled",
	url: "https://mcp.disabled.com/v1",
	enabled: false,
});

const allServers = [
	sentryServer,
	linearServer,
	githubServer,
	slackServer,
	datadogServer,
];

// ── Meta ───────────────────────────────────────────────────────

const meta: Meta<typeof MCPServerPicker> = {
	title: "pages/AgentsPage/MCPServerPicker",
	component: MCPServerPicker,
	args: {
		onSelectionChange: fn(),
		onAuthComplete: fn(),
	},
	decorators: [
		(Story) => (
			<div className="p-10">
				<Story />
			</div>
		),
	],
};

export default meta;
type Story = StoryObj<typeof MCPServerPicker>;

// ── Stories ────────────────────────────────────────────────────

/** No servers available — picker should not render. */
export const NoServers: Story = {
	args: {
		servers: [],
		selectedServerIds: [],
	},
};

/** All disabled servers — picker should not render. */
export const AllDisabled: Story = {
	args: {
		servers: [disabledServer],
		selectedServerIds: [],
	},
};

/** Single force-on server. Toggle should be disabled. */
export const SingleForceOn: Story = {
	args: {
		servers: [sentryServer],
		selectedServerIds: [sentryServer.id],
	},
};

/** Single default-on server with no auth required. */
export const SingleDefaultOnNoAuth: Story = {
	args: {
		servers: [linearServer],
		selectedServerIds: [linearServer.id],
	},
};

/** Single default-off optional server. */
export const SingleDefaultOff: Story = {
	args: {
		servers: [datadogServer],
		selectedServerIds: [],
	},
};

/** OAuth2 server needing authentication — shows Auth button. */
export const OAuthNeedsAuth: Story = {
	args: {
		servers: [githubServer],
		selectedServerIds: [githubServer.id],
	},
};

/** OAuth2 server already authenticated — shows check icon. */
export const OAuthConnected: Story = {
	args: {
		servers: [githubServerConnected],
		selectedServerIds: [githubServerConnected.id],
	},
};

/** Multiple servers with mixed availability and auth states. */
export const MixedServers: Story = {
	args: {
		servers: allServers,
		selectedServerIds: getDefaultMCPSelection(allServers),
	},
};

/** All servers with connected OAuth2 (GitHub connected). */
export const AllConnected: Story = {
	args: {
		servers: [sentryServer, linearServer, githubServerConnected, datadogServer],
		selectedServerIds: getDefaultMCPSelection([
			sentryServer,
			linearServer,
			githubServerConnected,
			datadogServer,
		]),
	},
};

/** Disabled state — all toggles disabled. */
export const Disabled: Story = {
	args: {
		servers: allServers,
		selectedServerIds: getDefaultMCPSelection(allServers),
		disabled: true,
	},
};

/** Servers including a disabled one — disabled should be filtered out. */
export const WithDisabledServer: Story = {
	args: {
		servers: [...allServers, disabledServer],
		selectedServerIds: getDefaultMCPSelection(allServers),
	},
};

/** All servers opted out — only force_on remains active. */
export const AllOptedOut: Story = {
	args: {
		servers: allServers,
		selectedServerIds: [sentryServer.id],
	},
};

/** Default-off server with OAuth needing auth (opted in, Auth button shows). */
export const OptionalOAuthNeedsAuth: Story = {
	args: {
		servers: [slackServer],
		selectedServerIds: [slackServer.id],
	},
};

/** Trigger shows overlapping icon stack when multiple servers are active. */
export const MultipleActiveIcons: Story = {
	args: {
		servers: [sentryServer, linearServer, githubServerConnected, datadogServer],
		selectedServerIds: [
			sentryServer.id,
			linearServer.id,
			githubServerConnected.id,
			datadogServer.id,
		],
	},
};

/** More than 3 active servers shows +N overflow badge. */
export const IconStackOverflow: Story = {
	args: {
		servers: allServers,
		selectedServerIds: allServers.map((s) => s.id),
	},
};
