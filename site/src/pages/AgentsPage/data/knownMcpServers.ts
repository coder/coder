/**
 * Known MCP server registry.
 *
 * Each entry maps one or more hostname patterns to a suggested display
 * name, slug, and bundled icon URL. The MCP server admin form uses this
 * to pre-fill empty fields when an administrator pastes a recognised
 * Server URL. The admin can still edit or clear any pre-filled value;
 * the form never overwrites fields that already contain user input.
 *
 * The registry intentionally lives in the frontend so it can ship with
 * every release without a database migration. Add new entries as
 * popular MCP servers gain bundled brand icons under
 * `site/static/icon/`. Keep each `hostPatterns` regex anchored with `^`
 * and `$` to avoid loose matches on overlapping vendors.
 */
interface KnownMcpServer {
	readonly displayName: string;
	readonly slug: string;
	readonly iconUrl: string;
	readonly hostPatterns: readonly RegExp[];
}

const KNOWN_MCP_SERVERS: readonly KnownMcpServer[] = [
	{
		displayName: "Bitbucket",
		slug: "bitbucket",
		iconUrl: "/icon/bitbucket.svg",
		hostPatterns: [/^mcp\.bitbucket\.org$/i, /^api\.bitbucket\.org$/i],
	},
	{
		displayName: "Discord",
		slug: "discord",
		iconUrl: "/icon/discord.svg",
		hostPatterns: [/^mcp\.discord\.com$/i, /^discord\.com$/i],
	},
	{
		displayName: "Figma",
		slug: "figma",
		iconUrl: "/icon/figma-black.svg",
		hostPatterns: [/^mcp\.figma\.com$/i, /^api\.figma\.com$/i],
	},
	{
		displayName: "GitHub",
		slug: "github",
		iconUrl: "/icon/github.svg",
		hostPatterns: [
			/^api\.githubcopilot\.com$/i,
			/^mcp\.github\.com$/i,
			/^api\.github\.com$/i,
		],
	},
	{
		displayName: "GitLab",
		slug: "gitlab",
		iconUrl: "/icon/gitlab.svg",
		hostPatterns: [/^mcp\.gitlab\.com$/i, /^gitlab\.com$/i],
	},
	{
		displayName: "Linear",
		slug: "linear",
		iconUrl: "/icon/linear.svg",
		hostPatterns: [/^mcp\.linear\.app$/i, /^linear\.app$/i],
	},
	{
		displayName: "Notion",
		slug: "notion",
		iconUrl: "/icon/notion.svg",
		hostPatterns: [
			/^mcp\.notion\.com$/i,
			/^(api\.)?notion\.com$/i,
			/^(api\.)?notion\.so$/i,
		],
	},
	{
		displayName: "Slack",
		slug: "slack",
		iconUrl: "/icon/slack.svg",
		hostPatterns: [/^mcp\.slack\.com$/i, /^slack\.com$/i],
	},
];

/**
 * Look up a known MCP server entry by Server URL.
 *
 * The match is keyed on the URL's hostname against each entry's
 * `hostPatterns`. Returns the first matching entry or null when the
 * URL is malformed or unrecognised. Callers that pre-fill form fields
 * from the result must only fill empty fields so users can override.
 */
export function findKnownMcpServerByUrl(url: string): KnownMcpServer | null {
	const trimmed = url.trim();
	if (trimmed === "") {
		return null;
	}
	let parsed: URL;
	try {
		parsed = new URL(trimmed);
	} catch {
		return null;
	}
	const host = parsed.hostname;
	for (const entry of KNOWN_MCP_SERVERS) {
		if (entry.hostPatterns.some((re) => re.test(host))) {
			return entry;
		}
	}
	return null;
}
