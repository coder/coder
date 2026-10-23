import { describe, expect, it } from "vitest";
import type { ChatDiffStatus } from "#/api/typesGenerated";
import {
	changeRequestLabel,
	findDiffStatus,
	getBlobUrl,
	getChangeRequestFilesUrl,
	getFileDiffAnchor,
	getFileWebLinks,
	getGitWebRepository,
} from "./gitWebUrl";

const diffStatus = (overrides: Partial<ChatDiffStatus>): ChatDiffStatus => ({
	chat_id: "chat-1",
	pull_request_title: "",
	pull_request_draft: false,
	changes_requested: false,
	additions: 0,
	deletions: 0,
	changed_files: 0,
	...overrides,
});

describe("getGitWebRepository", () => {
	it.each([
		["https://github.com/coder/coder.git", "https://github.com/coder/coder"],
		["https://github.com/coder/coder/", "https://github.com/coder/coder"],
		["https://token@github.com/coder/coder", "https://github.com/coder/coder"],
		[
			"ssh://git@github.com:22/coder/coder.git",
			"https://github.com/coder/coder",
		],
		["git@github.com:coder/coder.git", "https://github.com/coder/coder"],
		["git@GitHub.com:coder/coder", "https://github.com/coder/coder"],
	])("links %s on GitHub", (origin, url) => {
		expect(getGitWebRepository(origin, [])).toEqual({
			provider: "github",
			url,
		});
	});

	it("keeps GitLab subgroups in the repository path", () => {
		expect(
			getGitWebRepository("git@gitlab.com:group/sub/repo.git", []),
		).toEqual({ provider: "gitlab", url: "https://gitlab.com/group/sub/repo" });
	});

	it.each([
		["https://bitbucket.org/team/repo.git"],
		["https://git.example.com/team/repo.git"],
		["/home/coder/repo"],
		["https://github.com/coder"],
		["file:///srv/git/coder/coder.git"],
	])("does not link %s", (origin) => {
		expect(getGitWebRepository(origin, [])).toBeUndefined();
	});

	it.each([
		[
			"git@ghe.example.com:coder/coder.git",
			"https://ghe.example.com/coder/coder.git",
			"https://ghe.example.com/coder/coder/pull/12",
			{ provider: "github", url: "https://ghe.example.com/coder/coder" },
		],
		[
			"https://ghe.example.com/coder/coder",
			"https://ghe.example.com/coder/coder.git",
			"https://ghe.example.com/coder/coder/tree/feat/x",
			{ provider: "github", url: "https://ghe.example.com/coder/coder" },
		],
		[
			"git@git.example.com:group/sub/repo.git",
			"https://git.example.com/group/sub/repo",
			"https://git.example.com/group/sub/repo/-/merge_requests/7",
			{ provider: "gitlab", url: "https://git.example.com/group/sub/repo" },
		],
		[
			"https://git.example.com/gitlab/group/repo.git",
			"https://git.example.com/gitlab/group/repo.git",
			"https://git.example.com/gitlab/group/repo/-/tree/main",
			{ provider: "gitlab", url: "https://git.example.com/gitlab/group/repo" },
		],
	])(
		"recognizes the self-hosted %s from %s's URL %s",
		(origin, remoteOrigin, url, repository) => {
			expect(
				getGitWebRepository(origin, [
					diffStatus({ remote_origin: remoteOrigin, url }),
				]),
			).toEqual(repository);
		},
	);

	it("ignores the URLs of other repositories", () => {
		expect(
			getGitWebRepository("https://git.example.com/team/repo.git", [
				diffStatus({
					remote_origin: "https://git.example.com/team/other.git",
					url: "https://git.example.com/team/other/pull/1",
				}),
			]),
		).toBeUndefined();
	});
});

describe("findDiffStatus", () => {
	it("matches the repository whatever form its origin takes", () => {
		const status = diffStatus({
			remote_origin: "https://github.com/coder/coder",
			git_branch: "feat",
		});
		const statuses = [
			diffStatus({ remote_origin: status.remote_origin, git_branch: "main" }),
			status,
		];
		expect(
			findDiffStatus(statuses, "git@github.com:coder/coder.git", "feat"),
		).toBe(status);
		expect(
			findDiffStatus(statuses, "git@github.com:coder/other.git", "feat"),
		).toBeUndefined();
	});
});

describe("getBlobUrl", () => {
	it("links a file at a ref, on its line when given", () => {
		expect(
			getBlobUrl(
				{ provider: "github", url: "https://github.com/coder/coder" },
				"feat/x",
				"site/a b#1.ts",
				6,
			),
		).toBe("https://github.com/coder/coder/blob/feat/x/site/a%20b%231.ts#L6");
		expect(
			getBlobUrl(
				{ provider: "gitlab", url: "https://gitlab.com/group/repo" },
				"0123abc",
				"README.md",
			),
		).toBe("https://gitlab.com/group/repo/-/blob/0123abc/README.md");
	});
});

describe("getChangeRequestFilesUrl", () => {
	it("links the changed files of a pull or merge request", async () => {
		expect(
			getChangeRequestFilesUrl(
				"github",
				"https://github.com/coder/coder/pull/12",
				await getFileDiffAnchor("github", "README.md"),
			),
		).toBe(
			"https://github.com/coder/coder/pull/12/files#diff-b335630551682c19a781afebcf4d07bf978fb1f8ac04c6bf87428ed5106870f5",
		);
		expect(
			getChangeRequestFilesUrl(
				"gitlab",
				"https://gitlab.com/group/repo/-/merge_requests/7/diffs?view=inline",
				await getFileDiffAnchor("gitlab", "README.md"),
			),
		).toBe(
			"https://gitlab.com/group/repo/-/merge_requests/7/diffs#8ec9a00bfd09b3190ac6b22251dbb1aa95a0579d",
		);
		expect(
			getChangeRequestFilesUrl(
				"github",
				"https://github.com/coder/coder/pull/12",
			),
		).toBe("https://github.com/coder/coder/pull/12/files");
	});
});

describe("getFileWebLinks", () => {
	const origin = "git@github.com:coder/coder.git";
	const headCommit = "0123456789abcdef0123456789abcdef01234567";
	const links = (diffStatuses: ChatDiffStatus[], branch = "feat") =>
		getFileWebLinks({
			origin,
			branch,
			headCommit,
			relativePath: "site/main.go",
			line: 6,
			diffStatuses,
		});

	it("links the pushed branch and its pull request", () => {
		expect(
			links([
				diffStatus({
					remote_origin: "https://github.com/coder/coder",
					git_branch: "feat",
					url: "https://github.com/coder/coder/pull/12",
					pr_number: 12,
				}),
			]),
		).toEqual({
			repository: { provider: "github", url: "https://github.com/coder/coder" },
			blob: {
				url: "https://github.com/coder/coder/blob/feat/site/main.go#L6",
			},
			changeRequest: {
				url: "https://github.com/coder/coder/pull/12",
				number: "12",
			},
		});
	});

	it("links a pushed branch without a pull request at the branch", () => {
		const result = links([
			diffStatus({
				remote_origin: "https://github.com/coder/coder",
				git_branch: "feat",
				url: "https://github.com/coder/coder/tree/feat",
			}),
		]);
		expect(result?.blob).toEqual({
			url: "https://github.com/coder/coder/blob/feat/site/main.go#L6",
		});
		expect(result?.changeRequest).toBeUndefined();
	});

	it("links HEAD when the branch is not known to be pushed", () => {
		const result = links([
			diffStatus({
				remote_origin: "https://github.com/coder/coder",
				git_branch: "other",
				url: "https://github.com/coder/coder/pull/3",
				pr_number: 3,
			}),
		]);
		expect(result?.blob).toEqual({
			url: `https://github.com/coder/coder/blob/${headCommit}/site/main.go#L6`,
			commit: headCommit,
		});
		expect(result?.changeRequest).toBeUndefined();
		expect(
			getFileWebLinks({
				origin,
				branch: "feat",
				headCommit: "",
				relativePath: "site/main.go",
				diffStatuses: [],
			})?.blob,
		).toBeUndefined();
	});
});

describe("changeRequestLabel", () => {
	it("names pull and merge requests the way their hosts do", () => {
		expect(changeRequestLabel("github", "12")).toBe("PR #12");
		expect(changeRequestLabel("gitlab", "7")).toBe("MR !7");
	});
});
