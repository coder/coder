// Pure helpers for scripts/sync-docs.mjs, split out so they can be unit
// tested without running a sync. Kept app-local (duplicated in offlinedocs)
// until @coder/docs-sources exports shared equivalents.

/**
 * Strip URL userinfo (`user:token@`) from a repo URL before it reaches any
 * console output or error message, so a token-in-URL setup never leaks to
 * build logs.
 *
 * @param {string} repo - a git URL or a local path
 * @returns {string} the same value with any credentials removed
 */
export function redactRepoUrl(repo) {
  try {
    const url = new URL(repo);
    url.username = "";
    url.password = "";
    return url.href;
  } catch {
    // Not a WHATWG URL (a local path, or an scp-like address such as
    // `git@host:owner/name`): drop any userinfo-looking prefix.
    return repo.replace(/^[^@/\s]+@/, "");
  }
}

/**
 * Derive a `sourceLink` callback from the actual DOCS_REPO value instead of
 * hardcoding github.com/coder/coder. Only github.com repo URLs
 * (https or scp-like ssh) produce blob links; anything else — a fork host, a
 * local clone path, the fixture repo — returns undefined so the sync omits
 * sourceLink, matching the DOCS_ROOT branch.
 *
 * @param {string | undefined} repo - the DOCS_REPO value
 * @param {string} ref - the git ref the version syncs from
 * @returns {((repoRel: string) => string) | undefined}
 */
export function githubBlobSourceLink(repo, ref) {
  if (!repo) return undefined;
  const match =
    /^https:\/\/github\.com\/([\w.-]+)\/([\w.-]+?)(?:\.git)?\/?$/.exec(repo) ??
    /^(?:ssh:\/\/)?git@github\.com[:/]([\w.-]+)\/([\w.-]+?)(?:\.git)?$/.exec(repo);
  if (!match) return undefined;
  const [, owner, name] = match;
  return (repoRel) => `https://github.com/${owner}/${name}/blob/${ref}/${repoRel}`;
}

/**
 * Refuse to build the fixture corpus on a deploy: with VERCEL set, a missing
 * DOCS_REPO/DOCS_ROOT must fail the build instead of silently publishing the
 * placeholder site.
 *
 * @param {Record<string, string | undefined>} env - process.env
 */
export function assertDeployHasCorpus(env) {
  if (env.VERCEL && !env.DOCS_REPO && !env.DOCS_ROOT) {
    throw new Error(
      "VERCEL is set but neither DOCS_REPO nor DOCS_ROOT is: refusing to deploy the fixture corpus. " +
        "Set DOCS_REPO (or DOCS_ROOT) in the Vercel build environment.",
    );
  }
}

const VERSION = /^v?(\d+\.\d+(?:\.\d+)?)$/;

/**
 * The single version an offline export renders, from DOCS_VERSION (`v2.38.1`, `2.38`, or `main`).
 *
 * @param {Record<string, string | undefined>} env - process.env
 */
export function exportVersionFromEnv(env) {
  const ref = env.DOCS_REF ?? "main";
  const value = env.DOCS_VERSION ?? "";
  if (value === "" || value === "main") return { id: "main", label: "main", ref, latest: true };
  const match = VERSION.exec(value);
  if (!match)
    throw new Error(
      `DOCS_VERSION must look like v2.38.1, v2.38 or main, got ${JSON.stringify(value)}`,
    );
  return { id: match[1], label: `v${match[1]}`, ref, latest: true };
}
