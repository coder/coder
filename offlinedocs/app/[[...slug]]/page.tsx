import { createDocsPage } from "@coder/docs-next";
import { source } from "@/lib/source";

const { Page, generateStaticParams, generateMetadata } = createDocsPage(source, {
  // An air-gapped install keeps Copy Markdown (the twin is right here in the
  // export) but not the "Open in <assistant>" dropdown or the "Edit on GitHub"
  // link, whose every link is to a service on the public internet.
  pageActions: { viewOptions: false },
  editLink: false,
});

export default Page;
export { generateStaticParams, generateMetadata };
// Route-segment config must be statically analyzable (a re-exported binding is
// not); createDocsPage always returns false here, pinned by the engine's tests.
export const dynamicParams = false;
