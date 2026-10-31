import type * as TypesGen from "#/api/typesGenerated";
import { MockMCPServerConfig as BaseMockMCPServerConfig } from "#/testHelpers/chatEntities";

const now = "2026-03-19T12:00:00.000Z";

const MockMCPServerConfig: TypesGen.MCPServerConfig = {
	...BaseMockMCPServerConfig,
	created_at: now,
	updated_at: now,
};

export const MockCoderMCPServer: TypesGen.MCPServerConfig = {
	...MockMCPServerConfig,
	id: "mcp-coder",
	display_name: "Coder",
	slug: "coder",
	icon_url: "/icon/coder.svg",
	url: "https://dev.coder.com/api/experimental/mcp/http",
	transport: "streamable_http",
	auth_type: "oauth2",
	oauth2_client_registration: "manual",
	oauth2_client_id: "coder-mcp-client",
	oauth2_auth_url: "https://idp.example.com/oauth2/authorize",
	oauth2_token_url: "https://idp.example.com/oauth2/tokens",
	has_oauth2_secret: true,
	has_signing_secret: true,
	forward_coder_headers: true,
	availability: "default_off",
	enabled: true,
};

export const MockDynamicMCPServer: TypesGen.MCPServerConfig = {
	...MockCoderMCPServer,
	oauth2_client_registration: "dynamic",
	oauth2_client_id:
		"18ea34fc0ff4136437145221f81860922852d7a801db15c598cc89ce3a516b8d7a30ce78077d6dad4dcf903a537fe65e5310e94bec06f729861fa04d1527616ce7a286166f276d3f9e786208fd23e0be91161427ff3efdb25c97b06a9cb13eee",
	oauth2_scopes: "openid profile",
	oauth2_resource: "https://mcp.example.com/mcp",
};

export const MockGitHubMCPServer: TypesGen.MCPServerConfig = {
	...MockMCPServerConfig,
	id: "mcp-github",
	display_name: "GitHub",
	slug: "github",
	icon_url: "/icon/github.svg",
	url: "https://api.githubcopilot.com/mcp/",
	transport: "streamable_http",
	auth_type: "oauth2",
	has_oauth2_secret: true,
	availability: "default_off",
	enabled: true,
};

export const MockImageMCPServer: TypesGen.MCPServerConfig = {
	...MockMCPServerConfig,
	id: "mcp-image",
	display_name: "Image",
	slug: "image",
	url: "https://mcp.example.com/image",
	transport: "streamable_http",
	auth_type: "api_key",
	has_api_key: true,
	availability: "default_off",
	enabled: false,
};

export const MockMemoryMCPServer: TypesGen.MCPServerConfig = {
	...MockMCPServerConfig,
	id: "mcp-memory",
	display_name: "Memory",
	slug: "memory",
	url: "https://mcp.example.com/memory",
	transport: "streamable_http",
	auth_type: "oauth2",
	availability: "force_on",
	enabled: true,
};
