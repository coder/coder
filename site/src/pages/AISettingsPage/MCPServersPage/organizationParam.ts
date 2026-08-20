import type { Organization } from "#/api/typesGenerated";

export const orgSearchParam = "org";

export const selectOrganization = (
	organizations: readonly Organization[],
	name: string | null,
): Organization => {
	const organization =
		organizations.find((org) => org.name === name) ??
		organizations.find((org) => org.is_default) ??
		organizations[0];
	if (!organization) {
		throw new Error("Expected at least one organization");
	}
	return organization;
};

export const mcpServersPath = (organization: Organization) =>
	`/ai/settings/mcp-servers?${orgSearchParam}=${encodeURIComponent(organization.name)}`;

export const addMCPServerPath = (organization: Organization) =>
	`/ai/settings/mcp-servers/add?${orgSearchParam}=${encodeURIComponent(organization.name)}`;

export const updateMCPServerPath = (
	serverId: string,
	organization: Organization,
) =>
	`/ai/settings/mcp-servers/${serverId}?${orgSearchParam}=${encodeURIComponent(organization.name)}`;
