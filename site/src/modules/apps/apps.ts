import type {
	Workspace,
	WorkspaceAgent,
	WorkspaceApp,
} from "#/api/typesGenerated";
import { toast } from "#/components/Toaster/toast";

// This is a magic undocumented string that is replaced
// with a brand-new session token from the backend.
// This only exists for external URLs, and should only
// be used internally, and is highly subject to break.
export const SESSION_TOKEN_PLACEHOLDER = "$SESSION_TOKEN";

type SessionTokenHandler = {
	/** Scheme and authority, matched case-insensitively. */
	base: string;
	/** Path through the first required param, matched exactly. */
	path: string;
};

// The only deep-link handlers that may receive the session token.
const SESSION_TOKEN_HANDLERS: SessionTokenHandler[] = [
	...[
		"vscode",
		"vscode-insiders",
		"windsurf",
		"devin",
		"cursor",
		"kiro",
		"positron",
		"antigravity",
		"antigravity-ide",
	].flatMap((scheme) =>
		["/open?", "/openDevContainer?"].map((path) => ({
			base: `${scheme}://coder.coder-remote`,
			path,
		})),
	),
	{ base: "jetbrains://gateway", path: "/coder?" },
	{ base: "jetbrains://gateway", path: "/com.coder.toolbox?" },
	// Gateway also handles non-Coder providers, such as `type=ssh`.
	{ base: "jetbrains-gateway://connect", path: "#type=coder&" },
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
	accessUrl?: string;
};

export const getAppHref = (
	app: WorkspaceApp,
	{ path, token, workspace, agent, host, accessUrl }: GetAppHrefParams,
): string => {
	if (isExternalApp(app)) {
		// Sub-agent apps never get a token; `coder open app` refuses them too.
		return needsSessionToken(app) && !isSessionTokenLinkBlocked(app, accessUrl)
			? app.url.replaceAll(
					SESSION_TOKEN_PLACEHOLDER,
					agent.parent_id ? "" : (token ?? ""),
				)
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

/** Shown when `isSessionTokenLinkBlocked` blocks an app. */
export const SESSION_TOKEN_LINK_BLOCKED_MESSAGE =
	"This app's URL isn't trusted to sign you in to Coder.";

/** True when an app needs a session token but its link isn't trusted with one. */
export const isSessionTokenLinkBlocked = (
	app: WorkspaceApp,
	accessUrl: string | undefined,
): boolean =>
	isExternalApp(app) &&
	needsSessionToken(app) &&
	!isSessionTokenDestinationTrusted(app.url, accessUrl);

/**
 * True when the link opens a known Coder handler, its only `url` param is this
 * deployment (the current origin or `accessUrl`), and the placeholder is its
 * only `token`.
 */
export const isSessionTokenDestinationTrusted = (
	appUrl: string,
	accessUrl: string | undefined,
): boolean => {
	// Browsers strip tab, LF and CR before the IDE sees the link, so `ur\tl`
	// would become a second `url` there.
	if (
		/\p{Cc}/u.test(appUrl) ||
		appUrl.split(SESSION_TOKEN_PLACEHOLDER).length !== 2
	) {
		return false;
	}
	const handler = getHandler(appUrl);
	if (handler === undefined) {
		return false;
	}
	const params = appUrl.slice(
		handler.base.length + handler.path.search(/[?#]/) + 1,
	);
	const entries = params.split("&");
	const decoded = decodeOnce(params);
	// `getAppHref` replaces the raw placeholder, so it must be the raw `token`
	// entry. JetBrains Gateway doesn't decode names, so they must be literal.
	// IDEs decode values twice, so check that view too and reject leftovers.
	// A later `#` would hide params from IDEs that read only the query.
	return (
		!params.includes("#") &&
		entries.includes(`token=${SESSION_TOKEN_PLACEHOLDER}`) &&
		entries.every((entry) => !entry.split("=")[0].includes("%")) &&
		decoded !== undefined &&
		!decoded.includes("%") &&
		[params, decoded].every((p) =>
			hasTrustedParams(new URLSearchParams(p), accessUrl),
		)
	);
};

/** The known handler the link starts with, if any. Case folding is ASCII only. */
const getHandler = (appUrl: string): SessionTokenHandler | undefined =>
	SESSION_TOKEN_HANDLERS.find(
		({ base, path }) =>
			appUrl.slice(0, base.length).replace(/[A-Z]/g, (c) => c.toLowerCase()) ===
				base && appUrl.startsWith(path, base.length),
	);

const decodeOnce = (value: string): string | undefined => {
	try {
		return decodeURIComponent(value);
	} catch {
		return undefined;
	}
};

/** Every name appears once, `token` is the placeholder, `url` is this deployment. */
const hasTrustedParams = (
	params: URLSearchParams,
	accessUrl: string | undefined,
): boolean => {
	const names = [...params.keys()];
	return (
		new Set(names).size === names.length &&
		params.get("token") === SESSION_TOKEN_PLACEHOLDER &&
		isThisDeployment(params.get("url") ?? "", accessUrl)
	);
};

const isThisDeployment = (
	url: string,
	accessUrl: string | undefined,
): boolean => {
	if (!isBareOrigin(url)) {
		return false;
	}
	const { origin } = new URL(url);
	return (
		origin === location.origin ||
		(accessUrl !== undefined &&
			URL.canParse(accessUrl) &&
			origin === new URL(accessUrl).origin)
	);
};

/**
 * True when `url` is only an origin, optionally with a trailing slash. A path
 * could reach a workspace app on this deployment, and userinfo can hide a
 * different host from IDEs that decode it again. Matched on the raw string,
 * since parsing normalizes `/.` and drops an empty `?`.
 */
export const isBareOrigin = (url: string): boolean =>
	/^[a-z][a-z\d+.-]*:\/\/[^/\\?#@\s]+\/?$/i.test(url) && URL.canParse(url);

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
 * True when an app is not an external app, or is an external app whose URL can
 * be parsed by the URL constructor. External apps with an unparsable URL
 * cannot be launched. Template authors sometimes set a bare string with no
 * scheme, which would otherwise crash the page during render.
 */
export const isAppUrlValid = (app: WorkspaceApp): boolean => {
	if (!isExternalApp(app)) {
		return true;
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
