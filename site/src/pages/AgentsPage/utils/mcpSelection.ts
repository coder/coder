import type * as TypesGen from "#/api/typesGenerated";

/**
 * Compute the default selection based on server availability policies.
 * force_on and default_on servers are selected by default.
 */
export const getDefaultMCPSelection = (
	servers: readonly TypesGen.MCPServerConfig[],
): string[] => {
	const ids: string[] = [];
	for (const server of servers) {
		if (
			server.enabled &&
			(server.availability === "force_on" ||
				server.availability === "default_on")
		) {
			ids.push(server.id);
		}
	}
	return ids;
};

const legacyMCPSelectionStorageKey = "agents.selected-mcp-server-ids";

/** localStorage key for persisting the user's MCP server selection. */
export const mcpSelectionStorageKey = (organizationId: string) =>
	`${legacyMCPSelectionStorageKey}.${organizationId}`;

/**
 * Read the persisted MCP selection from localStorage, filtered to only
 * include IDs that still exist in the current server list.
 * Returns `null` when nothing is stored (caller should fall back to defaults).
 *
 * When `readLegacy` is set (the default organization inherits selections
 * that predate organization scoping), a selection found under the legacy
 * unscoped key is rewritten under the organization-scoped key, so the
 * storage schema heals itself on first successful read.
 */
export const getSavedMCPSelection = (
	organizationId: string,
	servers: readonly TypesGen.MCPServerConfig[],
	readLegacy = false,
): string[] | null => {
	let raw = localStorage.getItem(mcpSelectionStorageKey(organizationId));
	let fromLegacy = false;
	if (raw === null && readLegacy) {
		raw = localStorage.getItem(legacyMCPSelectionStorageKey);
		fromLegacy = raw !== null;
	}
	if (raw === null) {
		return null;
	}
	// If the server list is empty (e.g. the query hasn't loaded yet),
	// we can't validate any IDs so signal "unknown" rather than
	// returning an empty array that would be mistaken for "user
	// deliberately deselected everything".
	if (servers.length === 0) {
		return null;
	}
	try {
		const parsed: unknown = JSON.parse(raw);
		if (!Array.isArray(parsed)) {
			return null;
		}
		const enabledIds = new Set<string>();
		const forceOnIds: string[] = [];
		for (const server of servers) {
			if (!server.enabled) continue;
			enabledIds.add(server.id);
			if (server.availability === "force_on") {
				forceOnIds.push(server.id);
			}
		}
		const restored = parsed.filter(
			(id): id is string => typeof id === "string" && enabledIds.has(id),
		);
		// Merge force_on servers that might not be in the saved list.
		for (const id of forceOnIds) {
			if (!restored.includes(id)) {
				restored.push(id);
			}
		}
		if (fromLegacy) {
			saveMCPSelection(organizationId, restored);
			localStorage.removeItem(legacyMCPSelectionStorageKey);
		}
		return restored;
	} catch {
		return null;
	}
};

export const saveMCPSelection = (
	organizationId: string,
	ids: readonly string[],
): void => {
	localStorage.setItem(
		mcpSelectionStorageKey(organizationId),
		JSON.stringify(ids),
	);
};

const needsSignIn = (server: TypesGen.MCPServerConfig) =>
	server.auth_type === "oauth2" && !server.auth_connected;

/**
 * Servers the chat's MCP menu lists: those usable right now, plus those
 * the chat selected (or an admin forces on) whose sign-in has lapsed, so
 * the user can see why they stopped working.
 */
export const getMCPMenuServers = (
	servers: readonly TypesGen.MCPServerConfig[],
	selectedIds: readonly string[] | undefined,
): {
	server: TypesGen.MCPServerConfig;
	isSelected: boolean;
	isDisconnected: boolean;
}[] =>
	servers.flatMap((server) => {
		const isDisconnected = needsSignIn(server);
		const isSelected =
			server.availability === "force_on" ||
			(selectedIds?.includes(server.id) ?? false);
		return !isDisconnected || isSelected
			? [{ server, isSelected, isDisconnected }]
			: [];
	});

/**
 * Counts OAuth2 servers the user has no connected account for, whatever
 * their availability; callers pass only enabled servers.
 */
export const countMCPServersNeedingSignIn = (
	servers: readonly TypesGen.MCPServerConfig[],
): number => servers.filter(needsSignIn).length;
