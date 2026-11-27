// Sync the docs corpus into content/docs + public/ before `next build`.
//
// Corpus selection, in order:
//   1. DOCS_ROOT       — a docs tree on disk (local dev against a coder/coder
//                        checkout, and offline/air-gapped builds).
//   2. DOCS_REPO       — a git URL/path; clones once into .cache/docs-clone and
//                        loads DOCS_REF (default `main`).
//   3. fixtures/docs   — the committed fixture corpus, so CI and a fresh clone
//                        build without network or a coder/coder checkout.
//
// TODO: switch the DOCS_REPO path to
// `createGitRefsProviderFromContent` once coder/coder ships docs/versions.json;
// until then the ref is named explicitly.
import { cp, mkdir, readFile, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { syncDocSource, writeReleaseSchedule } from "@coder/docs-build";
import { parseReleaseSchedule } from "@coder/docs-config";
import { createGitRefsProvider, createLocalTreeProvider } from "@coder/docs-sources";
import {
  assertDeployHasCorpus,
  exportVersionFromEnv,
  githubBlobSourceLink,
  redactRepoUrl,
} from "./sync-helpers.mjs";

const appDir = path.dirname(path.dirname(fileURLToPath(import.meta.url)));

// A deploy must never silently publish the fixture corpus.
assertDeployHasCorpus(process.env);

const version = exportVersionFromEnv(process.env);

let provider;
let sourceLink;
if (process.env.DOCS_ROOT) {
  provider = createLocalTreeProvider({ docsRoot: process.env.DOCS_ROOT, version });
  console.log(`[sync] corpus: local tree at ${process.env.DOCS_ROOT}`);
} else if (process.env.DOCS_REPO) {
  provider = createGitRefsProvider({
    repo: process.env.DOCS_REPO,
    versions: [version],
    cacheDir: path.join(appDir, ".cache", "docs-clone"),
  });
  sourceLink = githubBlobSourceLink(process.env.DOCS_REPO, version.ref);
  console.log(`[sync] corpus: git ${redactRepoUrl(process.env.DOCS_REPO)} @ ${version.ref}`);
} else {
  provider = createLocalTreeProvider({
    docsRoot: path.join(appDir, "fixtures", "docs"),
    version,
  });
  console.log("[sync] corpus: committed fixture (set DOCS_ROOT or DOCS_REPO for the real corpus)");
}

const source = await provider.loadVersion(version);
const result = await syncDocSource(source, {
  contentDir: path.join(appDir, "content", "docs"),
  publicDir: path.join(appDir, "public"),
  rootTitle: "Home",
  sourceLink,
});
// source.config.ts imports this as ./.generated/image-sizes.json.
await mkdir(path.join(appDir, ".generated"), { recursive: true });
await writeFile(
  path.join(appDir, ".generated", "image-sizes.json"),
  `${JSON.stringify(result.imageSizes)}\n`,
);
// The export ships the schedule as of its build, with no refresh; the fixture
// corpus has no Releases page and needs no network.
// A schedule fetched earlier by a step that held the token, so the build never needs one.
const scheduleFile = process.env.DOCS_RELEASE_SCHEDULE_FILE;
const schedule = await writeReleaseSchedule({
  outFile: path.join(appDir, ".generated", "release-schedule.json"),
  token: process.env.DOCS_GITHUB_TOKEN || undefined,
  schedule: scheduleFile
    ? parseReleaseSchedule(JSON.parse(await readFile(scheduleFile, "utf8")))
    : process.env.DOCS_ROOT || process.env.DOCS_REPO
      ? undefined
      : null,
});
console.log(`[sync] release schedule: ${schedule ? schedule.generatedAt : "unavailable"}`);
console.log(
  `[sync] ${result.pagesWritten} pages, ${result.metaFilesWritten} meta files, ${result.imagesCopied} images (${result.skippedNonManifest} non-manifest files skipped)`,
);

// The favicon files and the supergraphic screenshot backdrops ship in
// @coder/docs-theme (the root layout references the favicons via
// coderFaviconIcons; brand.css references /brand/screenshot-bg-*.webp); copy
// them beside the synced images so they serve from the site root.
const themeAssets = path.dirname(
  path.dirname(
    fileURLToPath(import.meta.resolve("@coder/docs-theme/assets/favicons/favicon-light.svg")),
  ),
);
await cp(path.join(themeAssets, "favicons"), path.join(appDir, "public"), { recursive: true });
await cp(path.join(themeAssets, "brand"), path.join(appDir, "public", "brand"), {
  recursive: true,
});
