import type { WorkspaceAgentPortShareProtocol } from "api/typesGenerated";

export const localHosts = new Set(["localhost", "127.0.0.1", "0.0.0.0"]);

/**
 * Parse a port string from a URL, falling back to the protocol default
 * (80 for http, 443 for https) when the port is empty (i.e. not specified).
 */
export const resolveLocalhostPort = (
	portStr: string,
	protocol: string,
): number => {
	if (portStr) {
		return Number.parseInt(portStr, 10);
	}
	return protocol === "https:" || protocol === "https" ? 443 : 80;
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
	const { location } = window;
	const suffix = protocol === "https" ? "s" : "";

	const subdomain = `${port}${suffix}--${agentName}--${workspaceName}--${username}`;

	const baseUrl = `${location.protocol}//${host.replace(/\*/g, subdomain)}`;
	const url = new URL(baseUrl);
	if (pathname) {
		url.pathname = pathname;
	}
	if (search) {
		url.search = search;
	}
	return url.toString();
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

	try {
		const url = new URL(uri);
		if (!localHosts.has(url.hostname)) {
			open(uri);
			return;
		}
		const protocol = url.protocol.replace(
			":",
			"",
		) as WorkspaceAgentPortShareProtocol;
		open(
			portForwardURL(
				proxyHost,
				resolveLocalhostPort(url.port, url.protocol),
				agentName,
				workspaceName,
				username,
				protocol,
				url.pathname,
				url.search,
			),
		);
	} catch (_ex) {
		open(uri);
	}
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
