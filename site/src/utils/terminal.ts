import { API } from "api/api";

export const terminalWebsocketUrl = async (
	baseUrl: string | undefined,
	reconnect: string,
	agentId: string,
	command: string | undefined,
	height: number,
	width: number,
	containerName: string | undefined,
	containerUser: string | undefined,
): Promise<string> => {
	const query = new URLSearchParams({ reconnect });
	if (command) {
		query.set("command", command);
	}
	query.set("height", height.toString());
	query.set("width", width.toString());
	if (containerName) {
		query.set("container", containerName);
	}
	if (containerName && containerUser) {
		query.set("container_user", containerUser);
	}


	const url = new URL(baseUrl || `${location.protocol}//${location.host}`);
	url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
	if (!url.pathname.endsWith("/")) {
		`${url.pathname}/`;
	}
	url.pathname += `api/v2/workspaceagents/${agentId}/pty`;
	url.search = `?${query.toString()}`;

	// If the URL is just the primary API, we don't need a signed token to
	// connect.
	if (!baseUrl) {
		return url.toString();
	}

	// Do ticket issuance and set the query parameter.
	const tokenRes = await API.issueReconnectingPTYSignedToken({
		url: url.toString(),
		agentID: agentId,
	});
	query.set("coder_signed_app_token_23db1dde", tokenRes.signed_token);
	url.search = `?${query.toString()}`;

	return url.toString();
};
