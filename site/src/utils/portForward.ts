import type { WorkspaceAgentPortShareProtocol } from "api/typesGenerated";

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

	const baseUrl = `${location.protocol}//${host.replace("*", subdomain)}`;
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
		const localHosts = ["0.0.0.0", "127.0.0.1", "localhost"];
		if (!localHosts.includes(url.hostname)) {
			open(uri);
			return;
		}
		open(
			portForwardURL(
				proxyHost,
				Number.parseInt(url.port),
				agentName,
				workspaceName,
				username,
				url.protocol.replace(":", "") as WorkspaceAgentPortShareProtocol,
				url.pathname,
				url.search,
			),
		);
	} catch (ex) {
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
