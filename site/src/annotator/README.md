# Annotator

The click-to-annotate overlay behind the `chat-ui-annotations`
experiment. When a port preview in an agent chat is opened with
`?coder_annotate=1`, the workspace app proxy injects `annotator.js` into
the app's HTML. The overlay lets the user pick elements and leave
comments; each comment is posted to the dashboard over `postMessage` and
sent to the agent with the element's selector, test id, React component
names, an allowlisted opening tag, and the visible text.

The overlay runs inside pages we do not control, so it is plain
TypeScript with no framework and no dependencies: every import in this
directory is relative. `vite.annotator.config.mts` builds `main.ts` into
a single self-contained `out/annotator.js` next to the dashboard's own
output, and that build fails on any import that is not relative.

The dashboard side lives in `src/pages/AgentsPage` and imports
`#/annotator/protocol` (message types and the bounded parser) and
`#/annotator/formatAnnotations` (markdown output). Nothing in this
directory imports from the rest of the site. The Go side is
`coderd/workspaceapps/annotation.go`.
