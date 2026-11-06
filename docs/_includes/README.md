# Shared docs fragments

This folder holds Markdown fragments that more than one docs page shares.
A fragment is not a page: it has no route in `docs/manifest.json`, and readers only see it inside the pages that include it.

## Include a fragment

Put an include directive on its own line where the fragment's content belongs:

```md
<include>../../_includes/agent-relay/beta-notice.md</include>
```

- Write the path relative to the page that holds the directive.
- Keep the directive alone on its line.
  The build does not expand a directive that shares its line with other text.
- Indent the directive to place the fragment inside a list item.
  Every line of the fragment takes the directive's indentation.
- Keep the directive outside fenced code blocks and HTML comments.

A missing or circular include fails the docs build, and names the page and line.

## Write a fragment

- Group fragments in a folder named for the feature or page family, such as `port-forwarding/` or `sizing/`.
- Write links and images relative to the fragment file.
  The build rewrites them to resolve from each page that includes the fragment.
- Keep headings in the including pages, so each page's outline stays readable in its own source.
- Do not add front matter.
  The build drops it, because a fragment has no title of its own.
- Follow the [style guide](../.style/style-guide/README.md), as you would for a page.

## Edit a fragment

A change to a fragment changes every page that includes it.
Before you edit one, list those pages:

```sh
grep -rl --include='*.md' '_includes/agent-relay/beta-notice.md' docs
```

The docs preview comment on a pull request only lists the pages that the pull request changes.
Open the including pages in the preview to review a fragment change.
