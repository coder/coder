"use client";

// App Router error boundary, shared with apps/docs via @coder/docs-ui so the
// two copies cannot drift. Note the coverage gap: `app/error.tsx` does not
// catch errors thrown by the root layout itself — that would need
// `app/global-error.tsx` — but the known trigger (fumadocs Tabs' storage access
// when the browser blocks site data) throws inside the page tree, which this
// covers.
export { ErrorPage as default } from "@coder/docs-ui";
