import { createFromSource } from "fumadocs-core/search/server";
import { source } from "@/lib/source";

// The bundle is a fully static export with no server at runtime, so instead of
// a live search endpoint we emit a build-time search index. `staticGET` writes
// the Orama index as a static file that the client-side search (see
// provider.tsx) fetches and queries entirely in the browser, so search works
// with no server and fully offline. `revalidate = false` keeps the route
// static under `output: export`.
export const revalidate = false;

export const { staticGET: GET } = createFromSource(source);
