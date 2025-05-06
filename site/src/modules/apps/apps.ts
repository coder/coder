import { generateRandomString } from "utils/random";

type GetVSCodeHrefParams = {
	owner: string;
	workspace: string;
	token: string;
	agent?: string;
	folder?: string;
};

export const getVSCodeHref = (
	app: "vscode" | "vscode-insiders",
	{ owner, workspace, token, agent, folder }: GetVSCodeHrefParams,
) => {
	const query = new URLSearchParams({
		owner,
		workspace,
		url: location.origin,
		token,
		openRecent: "true",
	});
	if (agent) {
		query.set("agent", agent);
	}
	if (folder) {
		query.set("folder", folder);
	}
	return `${app}://coder.coder-remote/open?${query}`;
};

type GetTerminalHrefParams = {
	username: string;
	workspace: string;
	agent?: string;
	container?: string;
};

export const getTerminalHref = ({
	username,
	workspace,
	agent,
	container,
}: GetTerminalHrefParams) => {
	const params = new URLSearchParams();
	if (container) {
		params.append("container", container);
	}
	// Always use the primary for the terminal link. This is a relative link.
	return `/@${username}/${workspace}${
		agent ? `.${agent}` : ""
	}/terminal?${params}`;
};

export const openAppInNewWindow = (name: string, href: string) => {
	window.open(
		href,
		`${name} - ${generateRandomString(12)}`,
		"width=900,height=600",
	);
};
