import type { ChatDiffStatus } from "#/api/typesGenerated";
import { prNumber } from "./pullRequest";

/** The git hosts coderd's git integration supports (coderd/externalauth/gitprovider). */
export type GitProvider = "github" | "gitlab";

/** A repository's web page on a git host. */
export type GitWebRepository = {
	provider: GitProvider;
	/** Such as "https://github.com/coder/coder". */
	url: string;
};

export const gitProviderNames: Record<GitProvider, string> = {
	github: "GitHub",
	gitlab: "GitLab",
};

/** How each host names a pull request, such as "PR #12" or "MR !12". */
export const changeRequestLabel = (provider: GitProvider, number: string) =>
	provider === "github" ? `PR #${number}` : `MR !${number}`;

type Remote = { host: string; path: string };

const urlSchemePattern = /^[a-z][a-z0-9+.-]*:\/\//i;
const remoteProtocols = new Set([
	"https:",
	"http:",
	"ssh:",
	"git:",
	"git+ssh:",
]);

/**
 * The host and repository path of a remote origin, such as "github.com" and
 * "coder/coder" from "git@github.com:coder/coder.git". The port is dropped:
 * an SSH port says nothing about the web host's.
 */
const parseRemote = (origin: string): Remote | undefined => {
	const trimmed = origin.trim();
	let host: string;
	let path: string;
	if (urlSchemePattern.test(trimmed)) {
		let url: URL;
		try {
			url = new URL(trimmed);
		} catch {
			return undefined;
		}
		if (!remoteProtocols.has(url.protocol)) {
			return undefined;
		}
		host = url.hostname;
		path = url.pathname;
	} else {
		// Git's scp-like syntax, [user@]host:path.
		const match = /^(?:[^@/]+@)?([^/:]+):(.+)$/.exec(trimmed);
		if (!match) {
			return undefined;
		}
		[, host, path] = match;
	}
	path = path
		.split("/")
		.filter(Boolean)
		.join("/")
		.replace(/\.git$/, "");
	if (!host || !path.includes("/")) {
		return undefined;
	}
	return { host: host.toLowerCase(), path };
};

const isSameRepository = (origin: string, remote: Remote) => {
	const other = parseRemote(origin);
	return (
		other !== undefined &&
		other.host === remote.host &&
		other.path.toLowerCase() === remote.path.toLowerCase()
	);
};

/**
 * The repository URL and provider in a pull request, merge request or branch
 * URL, in the shapes coderd's providers build (BuildPullRequestURL and
 * BuildBranchURL). GitLab's come first: its "/-/tree/" also contains
 * GitHub's "/tree/".
 */
const hostedUrlPatterns: ReadonlyArray<readonly [GitProvider, RegExp]> = [
	["gitlab", /^(https?:\/\/.+?)\/-\/(?:merge_requests\/\d+|tree\/)/],
	["github", /^(https?:\/\/.+?)\/(?:pull\/\d+|tree\/)/],
];

const parseHostedUrl = (url: string): GitWebRepository | undefined => {
	for (const [provider, pattern] of hostedUrlPatterns) {
		const match = pattern.exec(url);
		if (match) {
			return { provider, url: match[1] };
		}
	}
	return undefined;
};

const providersByHost: Readonly<Record<string, GitProvider>> = {
	"github.com": "github",
	"gitlab.com": "gitlab",
};

/**
 * The web page of the repository at `origin`. The host names the provider
 * for github.com and gitlab.com. A self-hosted instance is recognized by the
 * shape of a pull request, merge request or branch URL that coderd
 * reported for the same repository. Undefined for any other host.
 */
export const getGitWebRepository = (
	origin: string,
	diffStatuses: readonly ChatDiffStatus[],
): GitWebRepository | undefined => {
	const remote = parseRemote(origin);
	if (!remote) {
		return undefined;
	}
	for (const status of diffStatuses) {
		if (
			status.url &&
			status.remote_origin &&
			isSameRepository(status.remote_origin, remote)
		) {
			const hosted = parseHostedUrl(status.url);
			if (hosted) {
				return hosted;
			}
		}
	}
	const provider = providersByHost[remote.host];
	return provider && { provider, url: `https://${remote.host}/${remote.path}` };
};

/**
 * The chat's diff status for `branch` of the repository at `origin`. One
 * exists once the branch is pushed, and carries the pull request, if any.
 */
export const findDiffStatus = (
	diffStatuses: readonly ChatDiffStatus[],
	origin: string,
	branch: string,
): ChatDiffStatus | undefined => {
	const remote = parseRemote(origin);
	return remote
		? diffStatuses.find(
				(status) =>
					status.git_branch === branch &&
					status.remote_origin !== undefined &&
					isSameRepository(status.remote_origin, remote),
			)
		: undefined;
};

const encodePath = (path: string) =>
	path.split("/").map(encodeURIComponent).join("/");

/** The page of `relativePath` at `ref`, scrolled to `line` when given. */
export const getBlobUrl = (
	repository: GitWebRepository,
	ref: string,
	relativePath: string,
	line?: number,
) => {
	const route = repository.provider === "gitlab" ? "-/blob" : "blob";
	const anchor = line === undefined ? "" : `#L${line}`;
	return `${repository.url}/${route}/${encodePath(ref)}/${encodePath(relativePath)}${anchor}`;
};

const hexDigest = async (algorithm: string, text: string) => {
	const digest = await crypto.subtle.digest(
		algorithm,
		new TextEncoder().encode(text),
	);
	return Array.from(new Uint8Array(digest), (byte) =>
		byte.toString(16).padStart(2, "0"),
	).join("");
};

/**
 * The anchor of `relativePath` in a pull or merge request's diff: GitHub's
 * "diff-" and the SHA-256 of the path, and GitLab's SHA-1 of the path
 * (Gitlab::Diff::File.file_hash, the id of each file in the diffs page).
 * Undefined outside secure contexts, which have no crypto.subtle.
 */
export const getFileDiffAnchor = async (
	provider: GitProvider,
	relativePath: string,
): Promise<string | undefined> => {
	if (!globalThis.crypto?.subtle) {
		return undefined;
	}
	return provider === "github"
		? `diff-${await hexDigest("SHA-256", relativePath)}`
		: hexDigest("SHA-1", relativePath);
};

/**
 * The changed files page of the pull or merge request at `url`, scrolled to
 * the file at `anchor` when given.
 */
export const getChangeRequestFilesUrl = (
	provider: GitProvider,
	url: string,
	anchor?: string,
): string => {
	const base =
		/^.*?\/(?:pull|-\/merge_requests)\/\d+/.exec(url)?.[0] ??
		url.replace(/[?#].*$/, "").replace(/\/+$/, "");
	const route = provider === "github" ? "files" : "diffs";
	return `${base}/${route}${anchor ? `#${anchor}` : ""}`;
};

type FileWebLinksOptions = {
	/** The repository's remote origin. */
	origin: string;
	/** The checked-out branch, empty on a detached HEAD. */
	branch: string;
	/** The full SHA of HEAD, empty when HEAD is unborn or unknown. */
	headCommit: string;
	/** The file's path in the repository. */
	relativePath: string;
	/** The 1-based line to scroll to. */
	line?: number;
	diffStatuses: readonly ChatDiffStatus[];
};

export type FileWebLinks = {
	repository: GitWebRepository;
	/**
	 * The file on the pushed branch, or else at HEAD. `commit` is set for
	 * HEAD, whose page is missing until the commit is pushed.
	 */
	blob?: { url: string; commit?: string };
	/** The pull or merge request of the branch, and its number. */
	changeRequest?: { url: string; number: string };
};

/**
 * Links to a repository file on its git host. A diff status for the branch
 * means coderd saw it on the remote, so the file links at the branch.
 */
export const getFileWebLinks = ({
	origin,
	branch,
	headCommit,
	relativePath,
	line,
	diffStatuses,
}: FileWebLinksOptions): FileWebLinks | undefined => {
	const repository = getGitWebRepository(origin, diffStatuses);
	if (!repository) {
		return undefined;
	}
	const status = branch
		? findDiffStatus(diffStatuses, origin, branch)
		: undefined;
	let blob: FileWebLinks["blob"];
	if (status) {
		blob = { url: getBlobUrl(repository, branch, relativePath, line) };
	} else if (headCommit) {
		blob = {
			url: getBlobUrl(repository, headCommit, relativePath, line),
			commit: headCommit,
		};
	}
	// A branch's status without a pull request carries the branch's URL.
	const number = status && prNumber(status);
	const changeRequest =
		status?.url && number ? { url: status.url, number } : undefined;
	return { repository, blob, changeRequest };
};
