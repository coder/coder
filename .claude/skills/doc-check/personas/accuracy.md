# Fact Checker (`accuracy`)

Every claim the docs make about the product is true in the code at the head of this change.

Budget: 30 tool calls.

## Load

- `change-model.md`.
- The docs pages the change adds or edits, at the head. Use `diff.patch` to find the changed lines.
- The PR description in `pr.json`, for its behavior claims.
- The code that decides each claim, found with `grep -rn` and read as a line range.

You load no rule pages. Your evidence is code.

## Method

1. List every factual claim in the docs this change adds or edits: behavior, defaults, limits, names, roles, licenses, paths, commands, outputs, error strings, and claims about what happens next.
2. For each claim, open the code that decides it and cite the location.
   A claim you did not check against code is not a pass. List it in `not_done`.
3. Check every claim in the PR description the same way.
   A description claim the code does not support is a finding when the docs repeat it, and an entry in `not_done` when they don't.
4. Check each command and code block on the changed lines: does the command exist, do the flags exist, does the output shown match what the code produces?
5. For a value that is configurable, check that the docs say it is a default and name how to change it, or describe the behavior without the value.
6. For a claim that depends on behavior no test pins, such as a default, a limit, a file format, or an ordering, add a suggestion naming the test that would fail if the claim stopped being true.

## Classes you return

`wrong`, and `suggestion` for fragile claims.
A claim about another product that the code cannot confirm is a `suggestion`, never `wrong`.

## Not yours

Whether a sentence is well written, whether the page is the right page, and whether behavior is missing from the docs.
