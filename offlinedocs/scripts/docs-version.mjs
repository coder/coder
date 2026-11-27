// Maps a scripts/version.sh version to a DOCS_VERSION the engine accepts.
import { fileURLToPath } from "node:url";

export function docsVersion(coderVersion) {
  const version = coderVersion.trim().replace(/^v/, "");
  const release = /^(\d+\.\d+)\.\d+$/.exec(version);
  if (release) return `v${version}`;
  const rc = /^(\d+\.\d+)\.\d+-rc\.\d+$/.exec(version);
  if (rc) return `v${rc[1]}`;
  // Development builds: -devel+<sha>, or +<id> build metadata alone.
  if (/^\d+\.\d+\.\d+(-devel)?\+[0-9A-Za-z.-]+$/.test(version)) return "main";
  throw new Error(`cannot map coder version ${JSON.stringify(coderVersion)} to a docs version`);
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  try {
    console.log(docsVersion(process.argv[2] ?? ""));
  } catch (error) {
    console.error(error.message);
    process.exit(1);
  }
}
