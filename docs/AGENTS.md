# Documentation Guidelines

Read the [style guide](.style/style-guide/README.md) before you write docs prose.
It is the canonical source; this page is the short list of rules that get missed most often.

The surrounding page is not a reliable guide to house style.
Most of the corpus predates these rules, so match the rules, not the neighbouring lines.

When the style guide and the [content guidelines](.style/content-guidelines.md) conflict, the content guidelines govern scope and routing.

## Docs contract

- **DOC1:** One sentence per source line.
  Don't split a sentence across lines, and don't wrap to a fixed column width.
  `MD013` is disabled on purpose.
- **DOC2:** Headings are sentence case, don't lead with an `-ing` word, and carry no trailing punctuation.
- **DOC3:** No em-dashes, en-dashes, or ` -- ` as punctuation, in prose, code, comments, or strings.
  Use commas, semicolons, periods, or a restructured sentence.
- **DOC4:** Address the reader as "you".
  No first-person singular.
  Reserve "we" for Coder Technologies as a company.
- **DOC5:** Active voice, present tense, contractions by default.
  Keep sentences short.
- **DOC6:** Use the canonical term: `select` not `click`, `stop` not `kill`, `turn off` not `disable`, `refer to` or `visit` not `see`, `Learn more` not `Next steps`, `tutorial` not `walkthrough`.
- **DOC7:** Use canonical product and brand casing, and one term per concept.
- **DOC8:** One instruction per step.
  Put the condition before the instruction.
  Callouts inform, steps instruct, and a warning states its consequence.
- **DOC9:** Link text and alt text describe the destination or the image.
  Never `here` or `click here`.
- **DOC10:** One audience and one outcome per page, declared up front.
- **DOC11:** Digits for 6 and higher, a non-breaking space between a number and its unit, and the documented date and time formats.

## Commands

| Task              | Command                |
|-------------------|------------------------|
| Prose lint (Vale) | `make lint/prose`      |
| Markdown lint     | `pnpm run check-docs`  |
| Markdown fix      | `pnpm run lint-docs`   |
| Table format      | `pnpm run format-docs` |

Prettier does not own Markdown in this repository.
Don't run it over `docs/`.

## Where the rules live

| Topic                                        | Page                                                                                      |
|----------------------------------------------|-------------------------------------------------------------------------------------------|
| Line breaks, text formatting, block elements | [formatting.md](.style/style-guide/formatting.md)                                         |
| Headings, dashes, commas, quotes             | [capitalization-and-punctuation.md](.style/style-guide/capitalization-and-punctuation.md) |
| Person, voice, tense, sentence length        | [voice-and-tone.md](.style/style-guide/voice-and-tone.md)                                 |
| Terminology and banned words                 | [word-choice.md](.style/style-guide/word-choice.md)                                       |
| Steps, conditions, callouts                  | [procedural-writing.md](.style/style-guide/procedural-writing.md)                         |
| Link text, alt text, plain English           | [accessibility-and-inclusion.md](.style/style-guide/accessibility-and-inclusion.md)       |
| Audience, scope, personas                    | [audience-and-scope.md](.style/style-guide/audience-and-scope.md)                         |
| Numbers, units, dates                        | [numbers-units-and-dates.md](.style/style-guide/numbers-units-and-dates.md)               |
| What belongs in `docs/` at all               | [content-guidelines.md](.style/content-guidelines.md)                                     |

Vale enforces a subset of these rules and reports the rest as advisory annotations.
A clean `make lint/prose` run does not mean the prose follows the style guide.
