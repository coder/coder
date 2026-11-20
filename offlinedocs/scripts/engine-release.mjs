// The docs-engine release this app builds with. Change ENGINE_RELEASE_TAG, run
// `pnpm engine:update`, then `pnpm install --lockfile-only`.
import { readFileSync, writeFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

export const ENGINE_RELEASE_TAG = "v0.1.0";

export const ENGINE_PACKAGES = [
  "docs-build",
  "docs-config",
  "docs-manifest",
  "docs-next",
  "docs-sources",
  "docs-theme",
  "docs-transforms",
  "docs-ui",
];

export function engineReleaseBaseUrl(tag) {
  if (!/^v\d+\.\d+\.\d+$/.test(tag)) throw new Error(`not a release tag: ${JSON.stringify(tag)}`);
  return `https://github.com/coder/docs-engine/releases/download/${tag}/`;
}

export function engineDependencies(tag) {
  const base = engineReleaseBaseUrl(tag);
  const version = tag.slice(1);
  return Object.fromEntries(
    ENGINE_PACKAGES.map((pkg) => [`@coder/${pkg}`, `${base}coder-${pkg}-${version}.tgz`]),
  );
}

// Every @coder/ dependency must be exactly one of the release's tarballs.
export function engineDependencyProblems(manifest, tag) {
  const want = engineDependencies(tag);
  const have = Object.fromEntries(
    Object.entries(manifest.dependencies ?? {}).filter(([name]) => name.startsWith("@coder/")),
  );
  const problems = [];
  for (const [name, url] of Object.entries(want)) {
    if (have[name] !== url)
      problems.push(`${name} should be ${url}, is ${have[name] ?? "missing"}`);
  }
  for (const name of Object.keys(have)) {
    if (!(name in want)) problems.push(`${name} is not a docs-engine release package`);
  }
  for (const field of ["devDependencies", "peerDependencies", "optionalDependencies"]) {
    for (const name of Object.keys(manifest[field] ?? {})) {
      if (name.startsWith("@coder/")) problems.push(`${field}.${name} belongs in dependencies`);
    }
  }
  return problems;
}

export function withEngineDependencies(manifest, tag) {
  const dependencies = { ...manifest.dependencies, ...engineDependencies(tag) };
  return {
    ...manifest,
    dependencies: Object.fromEntries(
      Object.keys(dependencies)
        .sort()
        .map((name) => [name, dependencies[name]]),
    ),
  };
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const manifestUrl = new URL("../package.json", import.meta.url);
  const manifest = JSON.parse(readFileSync(manifestUrl, "utf8"));
  if (process.argv[2] === "--write") {
    writeFileSync(
      manifestUrl,
      `${JSON.stringify(withEngineDependencies(manifest, ENGINE_RELEASE_TAG), null, 2)}\n`,
    );
  } else {
    const problems = engineDependencyProblems(manifest, ENGINE_RELEASE_TAG);
    if (problems.length) {
      console.error(
        `package.json does not match docs-engine ${ENGINE_RELEASE_TAG}; run pnpm engine:update:\n  ${problems.join("\n  ")}`,
      );
      process.exit(1);
    }
  }
}
