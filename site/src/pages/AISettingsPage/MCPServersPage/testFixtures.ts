import type * as TypesGen from "#/api/typesGenerated";
import { MockMCPServerConfig } from "#/testHelpers/chatEntities";

const now = "2026-03-19T12:00:00.000Z";

const createMCPServerConfig = (
	overrides: Partial<TypesGen.MCPServerConfig> &
		Pick<TypesGen.MCPServerConfig, "id" | "display_name" | "slug">,
): TypesGen.MCPServerConfig => ({
	...MockMCPServerConfig,
	created_at: now,
	updated_at: now,
	...overrides,
});

export const mockCoderMCPServer = createMCPServerConfig({
	id: "mcp-coder",
	display_name: "Coder",
	slug: "coder",
	icon_url: "/icon/coder.svg",
	url: "https://dev.coder.com/api/experimental/mcp/http",
	transport: "streamable_http",
	auth_type: "oauth2",
	has_oauth2_secret: true,
	availability: "default_off",
	enabled: true,
});

export const mockGitHubMCPServer = createMCPServerConfig({
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
});

export const mockImageMCPServer = createMCPServerConfig({
	id: "mcp-image",
	display_name: "Image",
	slug: "image",
	url: "https://mcp.example.com/image",
	transport: "streamable_http",
	auth_type: "api_key",
	has_api_key: true,
	availability: "default_off",
	enabled: false,
});

export const mockMemoryMCPServer = createMCPServerConfig({
	id: "mcp-memory",
	display_name: "Memory",
	slug: "memory",
	url: "https://mcp.example.com/memory",
	transport: "streamable_http",
	auth_type: "oauth2",
	availability: "force_on",
	enabled: true,
});
