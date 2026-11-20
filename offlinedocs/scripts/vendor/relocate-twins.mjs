// Move a static export's markdown twins from the staging directory the twin
// routes generate into to the page URLs they are served at, in place:
//
//   out/llms.mdx/admin/users.md -> out/admin/users.md
//   out/llms.mdx/index.md       -> out/index.md
//
// Every build mode ends with this, or the exports drift apart. The stitched
// build calls `relocateMarkdownTwins` itself, after its own overlay and
// alias steps; this is the entry point for a build whose last step is a
// plain `next build` (apps/offlinedocs, which runs it across the two apps
// the same way apps/docs runs offlinedocs' csp-check).
import { stat } from "node:fs/promises";
import path from "node:path";
import { relocateMarkdownTwins } from "./archive-layout-helpers.mjs";

const dir = path.resolve(process.argv[2] ?? "out");
const exported = await stat(dir).catch(() => undefined);
if (!exported?.isDirectory()) {
  throw new Error(`relocate-twins: ${dir} is missing; run the build first`);
}
const relocated = await relocateMarkdownTwins(dir);
console.log(`[relocate-twins] ${relocated} markdown twin(s) moved to their page URLs`);
