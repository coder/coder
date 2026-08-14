import type { Organization } from "#/api/typesGenerated";

export const orgSearchParam = "org";

export const selectOrganization = (
	organizations: readonly Organization[],
	name: string | null,
): Organization | undefined =>
	organizations.find((org) => org.name === name) ??
	organizations.find((org) => org.is_default) ??
	organizations[0];

export const mcpServersPath = (organization: Organization | undefined) =>
	organization
		? `/ai/settings/mcp-servers?${orgSearchParam}=${encodeURIComponent(organization.name)}`
		: "/ai/settings/mcp-servers";

export const addMCPServerPath = (organization: Organization | undefined) =>
	organization
		? `/ai/settings/mcp-servers/add?${orgSearchParam}=${encodeURIComponent(organization.name)}`
		: "/ai/settings/mcp-servers/add";

export const updateMCPServerPath = (
	serverId: string,
	organization: Organization | undefined,
) =>
	organization
		? `/ai/settings/mcp-servers/${serverId}?${orgSearchParam}=${encodeURIComponent(organization.name)}`
		: `/ai/settings/mcp-servers/${serverId}`;
