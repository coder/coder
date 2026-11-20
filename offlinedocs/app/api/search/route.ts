import { createDocsSearchRoute } from "@coder/docs-next";
import { source } from "@/lib/source";

export const { GET, revalidate } = createDocsSearchRoute(source);
