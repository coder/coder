# Annotator

The click-to-annotate overlay behind the `chat-ui-annotations`
experiment. When a port preview in an agent chat is opened with
`?coder_annotate=1`, the workspace app proxy injects `annotator.js` into
the app's HTML. The overlay lets the user pick elements and leave
comments; each comment is posted to the dashboard over `postMessage` and
sent to the agent with the element's selector, test id, React component
names, an allowlisted opening tag, and the visible text.

The dashboard side lives in `site/src/pages/AgentsPage` and consumes
`@coder/annotator/protocol` (message types and the bounded parser) and
`@coder/annotator/formatAnnotations` (markdown output) through a path
alias onto `src/`. The Go side is
`coderd/workspaceapps/annotation.go`.
