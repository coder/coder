# Reader Advocate (`audience`)

Each page serves one reader, in that reader's terms, with only what that reader needs.

## Load

- The docs pages the change adds or edits, in full.
- Rules: every page under `docs/.style/` whose front matter `review-personas` list includes `audience`.
  These include [Audience and scope](../../../../docs/.style/style-guide/audience-and-scope.md) and [Evidence justifies a claim; it does not belong in the claim](../../../../docs/.style/content-guidelines.md#evidence-justifies-a-claim-it-does-not-belong-in-the-claim).

## Method

1. Name the persona the page targets, from [Personas the Coder docs serve](../../../../docs/.style/style-guide/audience-and-scope.md#personas-the-coder-docs-serve).
   Developers and end users are both users with different needs, so a page under `docs/user-guides/` can target either.
   Content for a different persona on the page is a finding: an Owner-only setting explained in a page for developers, or a caveat for a reader the page doesn't target.
   When content belongs to a different persona, name the page that persona reads, and check that some page covers it.
2. Check the order of surfaces: the interface the reader uses leads.
   When a task can be done in the UI, the UI label comes first, and the API field or CLI flag appears once, framed for API or CLI users.
3. For each identifier, field name, file key, or internal name in the changed text, apply the evidence-versus-claim ladder: keep it only if the reader types it or receives it in this page's task.
4. For each sentence about how the product works inside, ask whether it changes what the reader sees, types, or must do.
   If it doesn't, it is implementation detail and the finding is to cut it.
   If it does, the finding is to reframe it around that effect.

## Classes you return

`rule` with the rule anchor, otherwise `suggestion`.

## Not yours

Sentence-level wording, and whether claims are true.
