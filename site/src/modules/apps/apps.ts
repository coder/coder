import type {
	Workspace,
	WorkspaceAgent,
	WorkspaceApp,
} from "api/typesGenerated";

// This is a magic undocumented string that is replaced
// with a brand-new session token from the backend.
// This only exists for external URLs, and should only
// be used internally, and is highly subject to break.
const SESSION_TOKEN_PLACEHOLDER = "$SESSION_TOKEN";

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

export const openAppInNewWindow = (href: string) => {
	window.open(href, "_blank", "width=900,height=600");
};

type CreateAppHrefParams = {
	path: string;
	host: string;
	workspace: Workspace;
	agent: WorkspaceAgent;
	token?: string;
};

export const createAppHref = (
	app: WorkspaceApp,
	{ path, token, workspace, agent, host }: CreateAppHrefParams,
): string => {
	if (isExternalApp(app)) {
		return needsSessionToken(app)
			? app.url.replaceAll(SESSION_TOKEN_PLACEHOLDER, token ?? "")
			: app.url;
	}

	// The backend redirects if the trailing slash isn't included, so we add it
	// here to avoid extra roundtrips.
	let href = `${path}/@${workspace.owner_name}/${workspace.name}.${
		agent.name
	}/apps/${encodeURIComponent(app.slug)}/`;

	if (app.command) {
		// Terminal links are relative. The terminal page knows how
		// to select the correct workspace proxy for the websocket
		// connection.
		href = `/@${workspace.owner_name}/${workspace.name}.${
			agent.name
		}/terminal?command=${encodeURIComponent(app.command)}`;
	}

	if (host && app.subdomain && app.subdomain_name) {
		const baseUrl = `${window.location.protocol}//${host.replace(/\*/g, app.subdomain_name)}`;
		const url = new URL(baseUrl);
		url.pathname = "/";
		href = url.toString();
	}

	return href;
};

export const needsSessionToken = (app: WorkspaceApp) => {
	if (!isExternalApp(app)) {
		return false;
	}

	// HTTP links should never need the session token, since Cookies
	// handle sharing it when you access the Coder Dashboard. We should
	// never be forwarding the bare session token to other domains!
	const isHttp = app.url.startsWith("http");
	const requiresSessionToken = app.url.includes(SESSION_TOKEN_PLACEHOLDER);
	return requiresSessionToken && !isHttp;
};

type ExternalWorkspaceApp = WorkspaceApp & {
	external: true;
	url: string;
};

export const isExternalApp = (
	app: WorkspaceApp,
): app is ExternalWorkspaceApp => {
	return app.external && app.url !== undefined;
};
