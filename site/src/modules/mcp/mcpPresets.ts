/** The MCP servers that coderd serves itself, offered as presets. */
export type BuiltInMCPServerId = "slack" | "github";

/** What the person adding an MCP server chose to start from. */
export type MCPServerPresetId = BuiltInMCPServerId | "custom";

export const slackDefaultAPIBaseURL = "https://slack.com/api";
export const githubDefaultAPIBaseURL = "https://api.github.com";

/** The GitHub MCP server toolset IDs, plus "all". */
export const githubToolsets = [
	{ id: "context", label: "Context" },
	{ id: "repos", label: "Repositories" },
	{ id: "issues", label: "Issues" },
	{ id: "pull_requests", label: "Pull requests" },
	{ id: "actions", label: "Actions" },
	{ id: "code_security", label: "Code security" },
	{ id: "discussions", label: "Discussions" },
	{ id: "notifications", label: "Notifications" },
	{ id: "users", label: "Users" },
	{ id: "orgs", label: "Organizations" },
	{ id: "gists", label: "Gists" },
	{ id: "projects", label: "Projects" },
	{ id: "labels", label: "Labels" },
	{ id: "all", label: "All toolsets" },
] as const;

const defaultGitHubToolsets: readonly string[] = [
	"context",
	"repos",
	"issues",
	"pull_requests",
];

/** The settings that end up in the query string of a built-in server's URL. */
export type MCPPresetOptions = {
	readonly slackAPIBaseURL: string;
	readonly slackPinThread: boolean;
	readonly githubAPIBaseURL: string;
	readonly githubToolsets: readonly string[];
	readonly githubReadOnly: boolean;
};

export const defaultMCPPresetOptions: MCPPresetOptions = {
	slackAPIBaseURL: slackDefaultAPIBaseURL,
	slackPinThread: false,
	githubAPIBaseURL: githubDefaultAPIBaseURL,
	githubToolsets: defaultGitHubToolsets,
	githubReadOnly: false,
};

type BuiltInMCPPreset = {
	readonly title: string;
	readonly slug: string;
	readonly description: string;
	readonly iconURL: string;
	/** What the person adding the server sees on the preset's card. */
	readonly summary: string;
};

export const builtInMCPPresets: Record<BuiltInMCPServerId, BuiltInMCPPreset> = {
	slack: {
		title: "Slack",
		slug: "slack",
		description:
			"Reply in Slack threads, react, read threads, look up users, and search messages with your Slack app's token.",
		iconURL: "/icon/slack.svg",
		summary: "Reply, react, read threads, and search messages.",
	},
	github: {
		title: "GitHub",
		slug: "github",
		description:
			"GitHub's official MCP tools for repositories, issues, and pull requests, using your GitHub token.",
		iconURL: "/icon/github.svg",
		summary: "Work with repositories, issues, and pull requests.",
	},
};

/** Whether `value` is the slug of a built-in server. */
const isBuiltInMCPServerId = (value: string): value is BuiltInMCPServerId =>
	Object.hasOwn(builtInMCPPresets, value);

const builtinPath = "/api/experimental/mcp/builtin/";

const trimTrailingSlash = (value: string) => value.replace(/\/+$/, "");

/** The URL of a built-in server on the deployment at `origin`. */
export const buildPresetURL = (
	preset: BuiltInMCPServerId,
	origin: string,
	options: MCPPresetOptions,
): string => {
	const params: string[] = [];
	if (preset === "slack") {
		const base = options.slackAPIBaseURL.trim();
		if (base !== "" && base !== slackDefaultAPIBaseURL) {
			params.push(`api_base_url=${encodeURIComponent(base)}`);
		}
		if (options.slackPinThread) {
			params.push("pin_thread=true");
		}
	} else {
		const toolsets = options.githubToolsets.includes("all")
			? ["all"]
			: options.githubToolsets;
		if (toolsets.length > 0) {
			params.push(`toolsets=${toolsets.join(",")}`);
		}
		const base = options.githubAPIBaseURL.trim();
		if (base !== "" && base !== githubDefaultAPIBaseURL) {
			params.push(`api_base_url=${encodeURIComponent(base)}`);
		}
		if (options.githubReadOnly) {
			params.push("read_only=true");
		}
	}
	const query = params.length > 0 ? `?${params.join("&")}` : "";
	return `${trimTrailingSlash(origin)}${builtinPath}${preset}${query}`;
};

type ParsedPresetURL = {
	readonly preset: BuiltInMCPServerId;
	/** The deployment URL the built-in path hangs off. */
	readonly origin: string;
	readonly options: MCPPresetOptions;
};

/**
 * Recognizes a built-in server by the path of its URL, so editing one shows
 * its options again. Returns undefined for any other URL.
 */
export const parsePresetURL = (url: string): ParsedPresetURL | undefined => {
	let parsed: URL;
	try {
		parsed = new URL(url);
	} catch {
		return undefined;
	}
	const match = parsed.pathname.match(
		/^(.*)\/api\/experimental\/mcp\/builtin\/(slack|github)\/?$/,
	);
	if (!match) {
		return undefined;
	}
	const preset = match[2] as BuiltInMCPServerId;
	const params = parsed.searchParams;
	const toolsets = params.get("toolsets");
	return {
		preset,
		origin: `${parsed.origin}${match[1]}`,
		options: {
			slackAPIBaseURL:
				preset === "slack"
					? (params.get("api_base_url") ?? slackDefaultAPIBaseURL)
					: slackDefaultAPIBaseURL,
			slackPinThread: preset === "slack" && params.get("pin_thread") === "true",
			githubAPIBaseURL:
				preset === "github"
					? (params.get("api_base_url") ?? githubDefaultAPIBaseURL)
					: githubDefaultAPIBaseURL,
			githubToolsets:
				preset === "github" && toolsets !== null
					? toolsets
							.split(",")
							.map((id) => id.trim())
							.filter(Boolean)
					: defaultGitHubToolsets,
			githubReadOnly: preset === "github" && params.get("read_only") === "true",
		},
	};
};

/** Where a person finds the token a built-in server asks for. */
export const builtInTokenHelp: Record<
	BuiltInMCPServerId,
	{ readonly label: string; readonly url: string }
> = {
	slack: {
		label: "Bot User OAuth Token (xoxb-...)",
		url: "https://api.slack.com/authentication/token-types#bot",
	},
	github: {
		label: "personal access token",
		url: "https://github.com/settings/personal-access-tokens",
	},
};

/**
 * Tells which built-in server an MCP server is: the one its URL points at, or
 * the one its slug names. Returns undefined for any other server.
 */
export const detectBuiltInMCPServer = (server: {
	readonly slug: string;
	readonly url?: string;
}): BuiltInMCPServerId | undefined => {
	const fromURL = server.url ? parsePresetURL(server.url)?.preset : undefined;
	if (fromURL) {
		return fromURL;
	}
	return isBuiltInMCPServerId(server.slug) ? server.slug : undefined;
};
