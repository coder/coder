import type * as TypesGen from "#/api/typesGenerated";

/** Reports whether the user must sign in before a chat can use the server. */
export const needsSignIn = (server: TypesGen.MCPServerConfig) =>
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
