import { STATIC_DOCS_RENDER_CONTEXT, parseReleaseSchedule } from "@coder/docs-config";
import { coderPageSchema, createCoderMdxOptions } from "@coder/docs-transforms/mdx-config";
import { replaceSource } from "fumadocs-core/mdx-plugins/stringifier";
import { defineConfig, defineDocs } from "fumadocs-mdx/config";
import imageSizes from "./.generated/image-sizes.json" with { type: "json" };
import releaseScheduleJson from "./.generated/release-schedule.json" with { type: "json" };

// No refresh URL: the export has no router, so it keeps the schedule it was built with.
const releaseSchedule =
  releaseScheduleJson === null ? null : parseReleaseSchedule(releaseScheduleJson);

// `docs.schema` is `coderPageSchema` (not fumadocs' default `pageSchema`) so a
// page's `premium_callout`/`experiments` frontmatter survives fumadocs-mdx's
// frontmatter validation instead of being silently stripped.
// `includeProcessedMarkdown` keeps each page's post-rendered Markdown
// (`page.data.getText`), which the routes under app/llms.mdx/ emit as that
// page's Markdown twin and llms-full.txt concatenates.
export const docs = defineDocs({
  dir: "content/docs",
  docs: { schema: coderPageSchema, postprocess: { includeProcessedMarkdown: true } },
});

// A static export has no deployment to ask, so premium and experimental
// blocks are resolved here: every block renders, labelled for an OSS reader.
export default defineConfig({
  mdxOptions: createCoderMdxOptions({
    renderContext: STATIC_DOCS_RENDER_CONTEXT,
    imageSizes,
    releaseSchedule,
    replaceSource,
  }),
});
