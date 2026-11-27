import { createLlmsRoutes } from "@coder/docs-next";
import { source } from "@/lib/source";

/**
 * The plain-text channel's route modules, shared by the three route files
 * under app/: the per-page Markdown twins, staged under `llms.mdx/`,
 * `llms.txt`, and `llms-full.txt`. One version, served at the root, so there
 * are no version-prefixed variants.
 */
export const llms = createLlmsRoutes(source);
