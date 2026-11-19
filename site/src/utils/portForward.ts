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

const subdomainURL = (
	host: string,
	subdomain: string,
	pathname?: string,
	search?: string,
): string => {
	const { location } = window;
	const baseUrl = `${location.protocol}//${host.replace(/\*/g, subdomain)}`;
	try {
		const url = new URL(baseUrl);
		if (pathname) {
			url.pathname = pathname;
		}
		if (search) {
			url.search = search;
		}
		return url.toString();
	} catch {
		// When the proxy host is empty or invalid, return a do-nothing anchor
		// so the link renders without navigating anywhere.
		return "#";
	}
};

export const portForwardURL = (
	host: string,
	port: number,
	agentName: string,
	workspaceName: string,
	username: string,
	protocol: WorkspaceAgentPortShareProtocol,
	pathname?: string,
	search?: string,
): string => {
	const suffix = protocol === "https" ? "s" : "";
	return subdomainURL(
		host,
		`${port}${suffix}--${agentName}--${workspaceName}--${username}`,
		pathname,
		search,
	);
};

/** Reports whether value is a TCP port number, from 1 to 65535. */
export const isValidPort = (value: unknown): value is number =>
	typeof value === "number" &&
	Number.isInteger(value) &&
	value >= 1 &&
	value <= 65535;

/**
 * Reports whether value is at most 63 lowercase letters, digits, and interior
 * hyphens, so it cannot add a dot, slash, or port to a hostname it goes into.
 */
export const isHostnameLabel = (value: unknown): value is string =>
	typeof value === "string" &&
	value.length <= 63 &&
	/^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/.test(value);

/** The URL the workspace agent serves a file preview at, from its slug. */
export const filePreviewURL = (
	host: string,
	slug: string,
	agentName: string,
	workspaceName: string,
	username: string,
	pathname?: string,
	search?: string,
): string => {
	return subdomainURL(
		host,
		`${slug}--${agentName}--${workspaceName}--${username}`,
		pathname,
		search,
	);
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

	if (!agentName || !workspaceName || !username || !proxyHost) {
		open(uri);
		return;
	}

	open(rewriteLocalhostURL(uri, proxyHost, agentName, workspaceName, username));
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
