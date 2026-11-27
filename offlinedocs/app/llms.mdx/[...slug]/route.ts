import { llms } from "@/lib/llms";

// One Markdown twin per page. This is the staging directory: `pnpm build`
// relocates `/llms.mdx/admin/users.md` to `/admin/users.md`, the URL a twin is
// served at, because no route segment can match on the extension. Plain text,
// never HTML — refer to packages/docs-next/src/llms.ts.
export const { GET, generateStaticParams } = llms.markdown;
// Statically analyzable segment config (Turbopack rejects re-exported
// bindings); a static export prerenders every twin it generates.
export const dynamic = "force-static";
