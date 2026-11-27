import { createMDX } from "fumadocs-mdx/next";

// Vercel's build image sets NEXT_DEPLOYMENT_ID. When it is present, Next 16
// leaves the build id out of every navigation payload and expects a Next
// server to send the deployment id in a response header instead, which a
// Next-managed deployment does. Nothing here is a Next server: the export is
// served by a static deployment, by the router, and by the Blob archive, so
// that header never arrives, the client's build-id guard fails on every
// navigation, and each click becomes a full page load. Unsetting the variable
// before Next reads its config puts the build id back into the payloads and
// restores client-side navigation within a build; the skew-protection query
// on asset URLs goes with it, which changes nothing for a fully static site.
// Assigning undefined to process.env stores the string "undefined"; only delete unsets it.
delete process.env.NEXT_DEPLOYMENT_ID;

const withMDX = createMDX();

export default withMDX({
  // A static, serverless bundle: no live search route, no ISR. `next build`
  // sets NODE_ENV=production, so builds always export; `next dev` stays a
  // normal dev server.
  output: process.env.NODE_ENV === "production" ? "export" : undefined,
  trailingSlash: true,
  images: { unoptimized: true },
});
