import { docs } from "collections/server";
import { loader } from "fumadocs-core/source";

// The offline bundle serves the docs at the site root (baseUrl "/"), so the
// tarball can be extracted and served directly. See
// https://fumadocs.dev/docs/headless/source-api for more info.
export const source = loader({
	baseUrl: "/",
	source: docs.toFumadocsSource(),
});
