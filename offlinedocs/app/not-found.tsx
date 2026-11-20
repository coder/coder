import { NotFoundDocsPage } from "@coder/docs-next";
import { NOT_FOUND_ARTWORK_PATH } from "@coder/docs-ui";
import type { Metadata } from "next";

// The exported `_not-found/index.html` and `404.html`. The root layout already
// wraps everything in the docs shell, so this is the page body alone; the
// offline export serves at the origin root, so the paths carry no base path.
export const metadata: Metadata = { title: "Page not found" };

export default function NotFound() {
  return <NotFoundDocsPage homeHref="/" imageSrc={NOT_FOUND_ARTWORK_PATH} />;
}
