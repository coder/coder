import {
	ExternalAppSessionTokenSchemes,
	type Workspace,
	type WorkspaceAgent,
	type WorkspaceApp,
} from "#/api/typesGenerated";
import { toast } from "#/components/Toaster/toast";

// This is a magic undocumented string that is replaced
// with a brand-new session token from the backend.
// This only exists for external URLs, and should only
// be used internally, and is highly subject to break.
export const SESSION_TOKEN_PLACEHOLDER = "$SESSION_TOKEN";

// Only these protocols may receive the session token. This prevents phishing
// attacks where a user is tricked into clicking a link that hands the Coder
// session token to a malicious app or unsupported protocol. The list is generated from codersdk so the
// server-side validation uses the same schemes.
const isSessionTokenProtocol = (protocol: string): boolean =>
	ExternalAppSessionTokenSchemes.some((scheme) => `${scheme}:` === protocol);

// The query or fragment parameter IDE handlers read the deployment URL from.
const SESSION_TOKEN_DESTINATION_PARAM = "url";

const WEB_PROTOCOLS = ["http:", "https:"];

const isDeploymentOrigin = (value: string): boolean =>
	URL.canParse(value) && new URL(value).origin === location.origin;

const isWebUrl = (value: string): boolean =>
	URL.canParse(value) && WEB_PROTOCOLS.includes(new URL(value).protocol);

/**
 * True when the session token can be safely substituted into an external app url.
 * Both the query and the fragment are checked because some handlers (such as JetBrains Gateway)
 * read their parameters from the fragment.
 */
export const canReceiveSessionToken = (rawUrl: string): boolean => {
	if (!URL.canParse(rawUrl)) {
		return false;
	}
	const url = new URL(rawUrl);
	if (!isSessionTokenProtocol(url.protocol)) {
		return false;
	}

	const params = [
		...url.searchParams,
		...new URLSearchParams(url.hash.slice(1)),
	];
	const destinations = params.filter(
		([key]) => key === SESSION_TOKEN_DESTINATION_PARAM,
	);
	if (destinations.length === 0) {
		return false;
	}

	return params.every(([key, value]) =>
		key === SESSION_TOKEN_DESTINATION_PARAM || isWebUrl(value)
			? isDeploymentOrigin(value)
			: true,
	);
};

type GetVSCodeHrefParams = {
	owner: string;
	workspace: string;
	token: string;
	agent?: string;
	folder?: string;
	chatId?: string;
};

export const getVSCodeHref = (
	app: "vscode" | "vscode-insiders" | "cursor",
	{ owner, workspace, token, agent, folder, chatId }: GetVSCodeHrefParams,
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
	if (chatId) {
		query.set("chatId", chatId);
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

// Open `about:blank` first to detect a popup blocker. If it opens, we
// null out `opener` (durable on the opened window); and navigate `popup`
// to the target URL. The Coder UI keeps access to `popup`s handle
export const openAppInNewWindow = (href: string) => {
	const popup = window.open("about:blank", "_blank", "width=900,height=600");
	if (!popup) {
		toast.error("Failed to open app in new window.", {
			description: "Popup blocked. Allow popups to open this app.",
		});
		return;
	}
	try {
		// Setting the opener to null persists in the `popup` window over refresh
		// and navigation. The opening window retains its connection to `popup`
		popup.opener = null;
	} catch {
		// Electron can throw
	}
	popup.location.href = href;
};

type GetAppHrefParams = {
	path: string;
	host: string;
	workspace: Workspace;
	agent: WorkspaceAgent;
	token?: string;
};

export const getAppHref = (
	app: WorkspaceApp,
	{ path, token, workspace, agent, host }: GetAppHrefParams,
): string => {
	if (isExternalApp(app)) {
		// Unparsable or untrusted URLs are returned raw. Consumers disable the button via isAppUrlValid, so the href is never followed.
		return needsSessionToken(app) && canReceiveSessionToken(app.url)
			? app.url.replaceAll(SESSION_TOKEN_PLACEHOLDER, token ?? "")
			: app.url;
	}

	if (app.command) {
		// Pass the app slug instead of the raw command. The terminal
		// page resolves the command from the workspace agent's app
		// list, which avoids exposing the command in the URL and
		// lets us skip the confirmation dialog for trusted,
		// admin-configured template apps.
		return `/@${workspace.owner_name}/${workspace.name}.${
			agent.name
		}/terminal?app=${encodeURIComponent(app.slug)}`;
	}

	if (host && app.subdomain && app.subdomain_name) {
		const baseUrl = `${location.protocol}//${host.replace(/\*/g, app.subdomain_name)}`;
		const url = new URL(baseUrl);
		url.pathname = "/";
		return url.toString();
	}

	// The backend redirects if the trailing slash isn't included, so we add it
	// here to avoid extra roundtrips.
	return `${path}/@${workspace.owner_name}/${workspace.name}.${
		agent.name
	}/apps/${encodeURIComponent(app.slug)}/`;
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

export const needsSessionToken = (app: ExternalWorkspaceApp) => {
	// HTTP links should never need the session token, since Cookies
	// handle sharing it when you access the Coder Dashboard. We should
	// never be forwarding the bare session token to other domains!
	const isHttp = app.url.startsWith("http");
	const requiresSessionToken = app.url.includes(SESSION_TOKEN_PLACEHOLDER);
	return requiresSessionToken && !isHttp;
};

/**
 * True for apps that can be rendered inside a dashboard iframe. Command apps
 * open in terminal tabs instead.
 */
export const isWorkspaceAppEmbeddable = (app: WorkspaceApp): boolean => {
	return !app.hidden && !isExternalApp(app) && !app.command;
};

export const AGENT_BROWSER_APP_SLUG = "agent-browser";

export const getAgentBrowserApp = (
	agent: WorkspaceAgent | undefined,
): WorkspaceApp | undefined => {
	const app = agent?.apps.find(
		(agentApp) => agentApp.slug === AGENT_BROWSER_APP_SLUG,
	);
	// "disabled" means the template does not configure a health check.
	if (
		app &&
		isWorkspaceAppEmbeddable(app) &&
		(app.health === "healthy" || app.health === "disabled")
	) {
		return app;
	}
	return undefined;
};

/**
 * An app is considered valid to launch if it is either internal or an external app with a properly formatted, parsable URL.
 * Any app requesting a session token remains invalid unless it restricts sending that token exclusively to the current deployment.
 */
export const isAppUrlValid = (app: WorkspaceApp): boolean => {
	if (!isExternalApp(app)) {
		return true;
	}
	if (needsSessionToken(app)) {
		return canReceiveSessionToken(app.url);
	}
	return URL.canParse(app.url);
};

/**
 * True when an app requires subdomain access but the deployment has no wildcard
 * access URL configured, so the app cannot be launched or embedded.
 */
export const isAppBlockedByMissingWildcard = (
	app: WorkspaceApp,
	wildcardHostname: string | undefined,
): boolean => {
	return app.subdomain && !wildcardHostname;
};
