Get the diff for the review target specified in your prompt, then review it.

Write all findings to the output file specified in your prompt. Create the directory if it doesn’t exist. The file is your deliverable — the orchestrator reads it, not your chat output. Your final message should just confirm the file path and how many findings it contains (or that you found nothing).

- **PR:** `gh pr diff {number}`
- **Branch:** `git diff origin/main...{branch}`
- **Commit range:** `git diff {base}..{tip}`

You can report two kinds of things:

**Findings** — concrete problems with evidence.

**Observations** — things that work but are fragile, work by coincidence, or are worth knowing about for future changes. These aren’t bugs, they’re context. Mark them with `Obs`.

Use this structure in the file for each finding:

---

**P{n}** `file.go:42` — One-sentence finding.

Evidence: what you see in the code, and what goes wrong.

---

For observations:

---

**Obs** `file.go:42` — One-sentence observation.

Why it matters: brief explanation.

---

Rules:

- **Severity**: P0 (blocks merge), P1 (should fix before merge), P2 (consider fixing), P3 (minor), P4 (out of scope, cosmetic).
- Severity comes from **consequences**, not mechanism. “setState on unmounted component” is a mechanism. “Dialog opens in wrong view” is a consequence. “Attacker can upload active content” is a consequence. “Removing this check has no test to catch it” is a consequence. Rate the consequence, whether it’s a UX bug, a security gap, or a silent regression.
- When a finding involves async code (fetch, await, setTimeout), trace the full execution chain past the async boundary. What renders, what callbacks fire, what state changes? Rate based on what happens at the END of the chain, not the start.
- Findings MUST have evidence. An assertion without evidence is an opinion.
- Evidence should be specific (file paths, line numbers, scenarios) but concise. Write it like you’re explaining to a colleague, not building a legal case.
- For each finding, include your practical judgment: is this worth fixing now, or is the current tradeoff acceptable? If there’s an obvious fix, mention it briefly.
- Observations don’t need evidence, just a clear explanation of why someone should know about this.
- Check the surrounding code for existing conventions. Flag when the change introduces a new pattern where an existing one would work (new file vs. extending existing, new naming scheme vs. established prefix, etc.).
- Note what the change does well. Good patterns are worth calling out so they get repeated.
- For comment quality standards (confidence threshold, avoiding speculation, verifying claims), see `.claude/skills/code-review/SKILL.md` Comment Standards section.
- If you find nothing, write a single line to the output file: “No findings.”
