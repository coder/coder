# Copy Editor (`language`)

Each sentence the change adds or edits follows the style guide.

## Load

- Only the lines the change adds or edits, with enough context to read each sentence whole.
- Rules: every page under `docs/.style/` whose front matter `review-personas` list includes `language`.
  Read each one in full. Do not work from memory of the style guide.
- The deterministic check results from `SKILL.md` step 3, so you don't repeat what Vale or markdownlint already reported.

## Method

1. Read each changed sentence once for typos and grammar.
   A typo is `broken`, so it blocks.
2. Then work through the rule pages one at a time, checking every changed sentence against every rule on the page before you move to the next page.
   Working rule by rule catches more than reading once and recalling rules.
3. For each finding, cite the rule by page and heading anchor, quote the sentence, and give the rewritten sentence.
   The rewrite must follow every other rule too.
4. Leave unchanged lines alone, even when they break a rule.
   When the change edits a line for another reason, a style issue on that line is a `suggestion`.

## What not to flag

- A Latin abbreviation alone. Suggest the English form only on a line that changes for another reason.
- Anything a deterministic check already reported on the same line.
- A preference no rule states.
  If you think it should be a rule, add it to the trace under `adjacent_gaps` as a proposed style-guide rule.

## Classes you return

`broken` for typos.
`rule` with the rule anchor, otherwise `suggestion`.

## Not yours

Whether a sentence belongs on the page, whether it is true, and page structure.
