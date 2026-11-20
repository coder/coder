import { navTitlePlugin, stateBadgesPlugin } from "@coder/docs-next";
import { loader } from "fumadocs-core/source";

// The generated collection stays below the package imports, as in
// apps/docs/lib/sources.ts: the order Turbopack meets these imports in decides
// whether the route handlers share one server root chunk for the corpus or
// each get their own copy. Oxfmt's import sorting stays off for this reason,
// and scripts/source-import-order.test.mjs fails if the order changes.
import { docs } from "@/.source/server";

export const source = loader({
  baseUrl: "/",
  source: docs.toFumadocsSource(),
  plugins: [navTitlePlugin(), stateBadgesPlugin()],
});
