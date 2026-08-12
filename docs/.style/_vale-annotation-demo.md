# Vale annotation rendering demo

> [!NOTE]
> This page exists only to verify how GitHub renders Vale annotations
> at each severity level.
> The three `Coder.Demo*` rules under
> [`docs/.style/styles/Coder/`](styles/Coder/) fire on the marker
> strings below.
> This page and its rules disappear in a follow-up commit on the same
> PR once the rendering check completes; see DOCS-426.

## Markers

Each marker fires exactly one Vale annotation when this page lints:

- Suggestion (rendered as GitHub `notice`):
  vale-demo-suggestion-marker.
- Warning (rendered as GitHub `warning`):
  vale-demo-warning-marker.
- Error (rendered as GitHub `error`):
  vale-demo-error-marker.

The rules use Vale's `existence` extension type and target a single
literal token each, so each marker produces a single annotation at its
exact location.
