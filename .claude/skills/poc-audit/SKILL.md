---
name: poc-audit
description: Design corpus for the audit, entity lifecycle, entity identity, and workspace_agent credential proof of concept in poc_audit/. Load before answering any question about auditability, journals versus logs, whether the existing audit_logs table can serve a need, what "actor", "sandbox", "AI agent", or workspace_agent mean in this work, entity identity and attribution, or the security findings on agent credentials. The documents record positions rather than current behaviour, including where code for a position now exists. For a guided first pass with feedback captured, use the learn-about-poc-audit skill instead.
---

# poc_audit router

Answers a question that has come up mid-task, from the design corpus in
`poc_audit/`. For someone meeting the corpus for the first time, the
`learn-about-poc-audit` skill runs a guided session and records feedback for
the author; this skill does neither.

## Three things to get right before answering

**The documents record positions, not behaviour.** They are not a description
of the running system and must never be presented as one, and that holds
whether or not code for a position exists yet. Some of what they propose has
been built and some has not; `work_breakdown.md` records which. Findings
sections are different in kind, stating verifiable facts about the existing
codebase, and are marked as such.

**"Audit" never means the existing `audit_logs` table.** They are independent
systems that happen to share a word. Where the existing mechanism is meant,
name it explicitly.

**"Agent" has three senses.** Unqualified it means an agent in the principal
and agent relation. The other two are always written out, as
`workspace_agent` and "AI agent". Hold to the same discipline in your answer.

## Standing, and what it obliges you to say

Sections are marked by standing. **Established** holds decided positions,
**Derived** holds reasoning offered for challenge, **Findings** holds
verifiable facts about this codebase, and **Open** holds questions not yet
answered.

Report Derived and Open content as what it is. An answer that presents
Derived reasoning as a decision has manufactured a position nobody holds.

## Routing and reading

`poc_audit/AGENTS.md` is the index. It records which document is
authoritative for which question, and carries these conventions in full. Read
it, then the document it points to, rather than guessing from filenames.

To read a document cheaply, take its heading map first:

```sh
grep -nE '^#{1,6} ' poc_audit/audit_approach.md
```

then read only the section you need. When the question is broad, read the
whole corpus rather than sampling it.

Cite the document and the section heading you answered from, specifically
enough that the reader can check it.
