import { toast } from "sonner";
import type {
	Workspace,
	WorkspaceAgent,
	WorkspaceApp,
} from "#/api/typesGenerated";

// This is a magic undocumented string that is replaced
// with a brand-new session token from the backend.
// This only exists for external URLs, and should only
// be used internally, and is highly subject to break.
export const SESSION_TOKEN_PLACEHOLDER = "$SESSION_TOKEN";

// This is a list of external app protocols that we
// allow to be opened in a new window. This is
// used to prevent phishing attacks where a user
// is tricked into clicking a link that opens
// a malicious app using the Coder session token.
const ALLOWED_EXTERNAL_APP_PROTOCOLS = [
	"vscode:",
	"vscode-insiders:",
	"windsurf:",
	"cursor:",
	"jetbrains-gateway:",
	"jetbrains:",
	"kiro:",
	"positron:",
	"antigravity:",
];

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
		const appProtocol = new URL(app.url).protocol;
		const isAllowedProtocol =
			ALLOWED_EXTERNAL_APP_PROTOCOLS.includes(appProtocol);

		return needsSessionToken(app) && isAllowedProtocol
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
