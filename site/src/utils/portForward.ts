import type { WorkspaceAgentPortShareProtocol } from "#/api/typesGenerated";

const localHosts = new Set(["localhost", "127.0.0.1", "0.0.0.0"]);

/**
 * Parse a port string from a URL, falling back to the protocol default
 * (80 for http, 443 for https) when the port is empty (i.e. not
 * specified).
 *
 * @param portStr - The port string from `URL.port` (empty when the URL
 *   uses the protocol's default port).
 * @param protocol - The protocol string from `URL.protocol`, which
 *   always includes a trailing colon (e.g. `"https:"`).
 */
export const resolveLocalhostPort = (
	portStr: string,
	protocol: string,
): number => {
	if (portStr) {
		return Number.parseInt(portStr, 10);
	}
	return protocol === "https:" ? 443 : 80;
};

// These values and algorithms must match coderd/workspaceapps/appurl.
const maxLabelLength = 63;
const longestPort = "65535s";

/**
 * Returns the 32-bit FNV-1a hash of the agent name in base36, padded to
 * 7 characters. Short form port URLs identify the agent with it.
 */
const agentNameHash = (agentName: string): string => {
	let hash = 0x811c9dc5;
	for (const byte of new TextEncoder().encode(agentName)) {
		hash = Math.imul(hash ^ byte, 0x01000193) >>> 0;
	}
	return hash.toString(36).padStart(7, "0");
};

/**
 * Returns the subdomain for a port. It uses the long form when the longest
 * port fits in one DNS label next to the fixed part of the wildcard host, and
 * the short form, which identifies the workspace by ID, otherwise.
 */
const portSubdomain = (
	host: string,
	port: string,
	agentName: string,
	workspaceName: string,
	username: string,
	workspaceId: string,
): string => {
	const fixedLength = host.split(".")[0].replace("*", "").length;
	const longForm = (p: string) =>
		`${p}--${agentName}--${workspaceName}--${username}`;
	if (longForm(longestPort).length + fixedLength <= maxLabelLength) {
		return longForm(port);
	}
	const encodedWorkspaceId = BigInt(`0x${workspaceId.replaceAll("-", "")}`)
		.toString(36)
		.padStart(25, "0");
	return `${port}--${encodedWorkspaceId}--${agentNameHash(agentName)}`;
};

export const portForwardURL = (
	host: string,
	port: number,
	agentName: string,
	workspaceName: string,
	username: string,
	workspaceId: string,
	protocol: WorkspaceAgentPortShareProtocol,
	pathname?: string,
	search?: string,
): string => {
	const { location } = window;
	const suffix = protocol === "https" ? "s" : "";

	try {
		const subdomain = portSubdomain(
			host,
			`${port}${suffix}`,
			agentName,
			workspaceName,
			username,
			workspaceId,
		);
		const url = new URL(
			`${location.protocol}//${host.replace(/\*/g, subdomain)}`,
		);
		if (pathname) {
			url.pathname = pathname;
		}
		if (search) {
			url.search = search;
		}
		return url.toString();
	} catch {
		// When the proxy host or workspace ID is empty or invalid, return a
		// do-nothing anchor so the link renders without navigating anywhere.
		return "#";
	}
};

/**
 * Rewrite a localhost URL to use the workspace port-forward subdomain.
 * Returns the original URL unchanged when it is not a recognized
 * localhost address or when parsing fails.
 */
export const rewriteLocalhostURL = (
	url: string,
	proxyHost: string,
	agentName: string,
	workspaceName: string,
	username: string,
	workspaceId: string,
): string => {
	try {
		const parsed = new URL(url);
		if (!localHosts.has(parsed.hostname)) {
			return url;
		}
		const protocol = parsed.protocol.replace(
			":",
			"",
		) as WorkspaceAgentPortShareProtocol;
		return portForwardURL(
			proxyHost,
			resolveLocalhostPort(parsed.port, parsed.protocol),
			agentName,
			workspaceName,
			username,
			workspaceId,
			protocol,
			parsed.pathname,
			parsed.search,
		);
	} catch {
		return url;
	}
};

// openMaybePortForwardedURL tries to open the provided URI through the
// port-forwarded URL if it is localhost, otherwise opens it normally.
export const openMaybePortForwardedURL = (
	uri: string,
	proxyHost?: string,
	agentName?: string,
	workspaceName?: string,
	username?: string,
	workspaceId?: string,
) => {
	const open = (uri: string) => {
		// Copied from: https://github.com/xtermjs/xterm.js/blob/master/addons/xterm-addon-web-links/src/WebLinksAddon.ts#L23
		const newWindow = window.open();
		if (newWindow) {
			try {
				newWindow.opener = null;
			} catch {
				// no-op, Electron can throw
			}
			newWindow.location.href = uri;
		} else {
			console.warn("Opening link blocked as opener could not be cleared");
		}
	};

	if (!agentName || !workspaceName || !username || !workspaceId || !proxyHost) {
		open(uri);
		return;
	}

	open(
		rewriteLocalhostURL(
			uri,
			proxyHost,
			agentName,
			workspaceName,
			username,
			workspaceId,
		),
	);
};

export const saveWorkspaceListeningPortsProtocol = (
	workspaceID: string,
	protocol: WorkspaceAgentPortShareProtocol,
) => {
	localStorage.setItem(
		`listening-ports-protocol-workspace-${workspaceID}`,
		protocol,
	);
};

export const getWorkspaceListeningPortsProtocol = (
	workspaceID: string,
): WorkspaceAgentPortShareProtocol => {
	return (localStorage.getItem(
		`listening-ports-protocol-workspace-${workspaceID}`,
	) || "http") as WorkspaceAgentPortShareProtocol;
};
