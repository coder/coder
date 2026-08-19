---
name: learn-about-poc-audit
description: Guided first pass through the poc_audit design corpus for someone new to it. Answers their questions under an explicit statement of what is and is not known, and records the session verbatim as feedback for the author. Use when someone wants to learn the audit, entity lifecycle, entity identity, and workspace_agent credential design rather than look up one answer. For a single question arising mid-task, use the poc-audit skill instead.
---

# Learn about poc_audit

A guided session with two purposes: bring someone up to speed on the design
corpus faster than meeting it cold, and capture what was unclear, disputed, or
out of step with the code so the author can fix it.

This supersedes the `poc-audit` skill for the session. Do not invoke that one
as well. Everything it carries is either here or in `poc_audit/AGENTS.md`.

## 1. Say what this is, then make these disclosures

Someone arriving cold does not know what they have walked into, and neither
the disclosures below nor the reading step make sense without framing. Open
with two or three plain sentences covering:

- **What these documents are.** Design work for a proof of concept covering
  audit, entity lifecycle, entity identity, and credentials for
  `workspace_agent` entities. They record positions and findings. None of it
  is built, so nothing in them describes how the system behaves today.
- **What is about to happen.** You will read all of them, which takes a
  moment, then answer their questions, asking after each answer how it landed.
  It ends whenever they want it to.

Speak plainly. "Corpus" is internal vocabulary; say "these documents" unless
you introduce the word. Saying you are loading them into context is fine.

Then say both of these, briefly and in your own words:

- Candor is preferable to cheerleading. Saying a passage is confusing helps
  the next person read it. Saying it was fine when it was not helps nobody.
- Feedback is not anonymous. It is recorded verbatim and delivered through git
  under the participant's name. Delivery through git is a proof of concept
  convenience rather than part of the process, and is expected to be replaced.

Then ask what they want to know.

## 2. Load the corpus

Read every document in `poc_audit/`. Do not sample, and do not route to a
single document. The confidence reporting in step 4 depends on having read all
of it: no answer can honestly claim the corpus is silent on something you have
not read.

Tell the participant what you are doing in concrete terms, that you are
reading the design documents in `poc_audit/` and it will take a moment.

If the corpus no longer fits comfortably, stop and ask the participant to run
this in a larger context. Do not proceed with a partial read.

## 3. Ask what they want to know

Ask before offering anything. A question the participant brings themselves is
worth more, to them and to the record both, because it shows what a newcomer
actually wonders and in what words they arrive at it. Producing a list
unprompted steers them towards your questions and away from their own.

Only if they have no question yet, offer these three, and say that is what
they are for:

1. Why not reuse the existing `audit_logs` table?
2. What makes an event auditable?
3. What does "actor" mean here, and who counts as one?

Present them and stop. Do not answer one unless the participant picks it.

## 4. Answer under an explicit statement of what you know

Every answer carries a label and its citations.

Keep answers short: a few sentences to a short paragraph, then offer to go
deeper rather than pre-empting. A newcomer handed six paragraphs cannot say
which part was unclear, and step 5 depends on them being able to.

**Lead with the answer.** The first sentence answers the question that was
asked. Not a definition, not context, not a preamble announcing how many parts
the answer comes in. If the participant read your opening line and stopped,
they should have the answer; everything after it is elaboration they are free
to abandon.

Watch for the answer that arrives last. Etymology, how a term came to be used,
and why these documents chose one word over another are background about the
vocabulary rather than answers about the subject. They belong after the point,
or not at all. "What is this about?" is answered by what the work does, not by
where the word came from.

**Summarise, do not argue.** State what these documents hold. Do not reproduce
the reasoning that got them there. The reasoning is in the documents and you
have cited where, so a reader who wants it can go and read it, and a reader
who does not yet know the subject cannot weigh it anyway. Justifying a
position to someone who has no basis to judge it spends their attention and
buys nothing.

If you find yourself walking through a distinction in order to defend it, you
are arguing rather than summarising. Say the thing, say where the argument
lives, and offer to go into it.

**Terms of art.** These documents run on a vocabulary a newcomer does not
have, and `poc_audit/AGENTS.md` records which document is authoritative for
which term. An answer resting on one that has not been introduced is opaque
however correct it is, and glossing five of them in one answer is no better,
because five new ideas still arrive at once.

So before answering, notice which terms the answer will be load-bearing on,
and then:

- Introduce at most one or two in a single answer. Gloss each in a clause and
  say which document defines it properly.
- If an honest answer needs more than that, say so rather than delivering them
  all. Name the one the others rest on, offer to take that first, and let the
  participant choose the order.
- Keep account of what you have introduced. Once a term has been introduced
  and the participant has not objected, use it freely.
- Naming a term you are deliberately not unpacking yet, and saying that is
  what you are doing, leaves a path open without spending it. It tells the
  participant there is more and where it goes, and lets them choose when.

This is a brake rather than a prohibition. The vocabulary is what these
documents are written in, and acquiring it is much of what the participant
came for.

**Label**, from this vocabulary, which is a starting point rather than a closed
set:

- **Definitive.** Answered from Established or Findings content.
- **Partial.** The corpus speaks to part of the question only.
- **Present but not settled.** The answer exists only as Derived or Open.
- **Outside the corpus.** Answered from general knowledge, and said so.
- **Corpus appears inconsistent.** Two documents seem to disagree.
- **Misaligned with the code.** See below.

If none of these fits, say so in your own words and record that the vocabulary
was insufficient. That is itself a finding.

**Put the label where it helps.** A label that qualifies the answer changes
how it should be read, so it belongs before the answer: partial, present but
not settled, outside the corpus, corpus appears inconsistent, and misaligned
with the code all lead. Definitive does not qualify anything, so it goes at
the end. Opening with a confident label reads oddly and tells the participant
nothing they need before reading.

**Citations.** Name the document and the section heading, specifically enough
that the participant can check it. A label without a citation is decoration.

**External claims**, in law, accounting, or anything else outside the corpus,
carry their own discipline. Default to the unnamed general form: "this is a
general principle in accounting, not a specific source I can cite." Offer to
verify a named source before relying on it, and name it only once verified. If
the participant declines verification, keep the general form rather than
naming something unchecked. When you do name a source, validate its existence,
its salience to the point, your interpretation of it, and its relevance to the
question. Never invent a citation.

**Misalignment.** Where the corpus and the code disagree, report both sides:
what the corpus says with its citation, what the code does with its file and
symbol. Do not decide which is authoritative, do not propose a fix, and do not
describe either as stale. A misalignment is invisible if you answer straight
from the code, so check the corpus first whenever an answer draws on the
codebase.

## 5. Ask how the answer landed

After each answer, ask whether it was clear, partly clear, disagreed with,
misaligned with what they know of the code, or in need of a follow-up.

Record what they say in their own words. Do not argue.

**On disagreement**, ask exactly one follow-up question, aimed at telling a
genuine disagreement apart from an objection resting on a misunderstanding.
Record it as whichever it turns out to be, because they are different
findings: a misunderstanding means the corpus led a careful reader astray,
which is a clarity defect rather than a null result.

After a disagreement, ask whether the author may contact them about it. That
is a separate consent from the recording one, and declining it does not remove
the disagreement from the record.

## 6. Keep the record as you go

One file per session, `poc_audit_feedback/YYYY-MM-DD-<name>.md`. Append after
each exchange rather than writing at the end, so a session that ends abruptly
still leaves everything it covered.

YAML front matter, then the exchanges:

```yaml
record_version: 1
participant: <name>
date: <YYYY-MM-DD>
corpus_tree: <output of: git rev-parse HEAD:poc_audit>
corpus_commit: <output of: git rev-parse HEAD>
```

`record_version` describes the shape of this file, not the version of this
skill. It comes first because a later reader has to know the shape before
interpreting any other field. Bump it whenever the fields change, and leave
older records untouched: they remain readable under the version they were
written at, even when they are no longer directly comparable to newer ones.

`corpus_tree` is the durable identifier. This branch has been rebased before,
which rewrites every commit hash while leaving the tree of an unchanged
directory identical. The commit is recorded for convenience; the tree is what
pins which corpus was actually read.

Each exchange records the question as the participant phrased it, whether it
was seeded or spontaneous, the label used, the citations, the terms of art the
answer rested on with the ones you introduced there marked, their verdict in
their own words, and any consents.

The terms field is the one whose value is least obvious. Across sessions it
shows which ideas a reader has to acquire before a given question can be
answered at all, and which of them cost an introduction every single time.
That is evidence about the shape of these documents rather than about any one
reader. Where the verdict is simply that the answer
was clear, that alone is enough, but keep the citations either way. Across
sessions they show which parts of the corpus are load-bearing and which are
never reached.

## 7. Close and deliver

Close when the participant says they are done, and offer to close if the
session is running long.

Say what happens next. These documents stay loaded, so they can keep asking
for as long as the session lasts. Be plain that those answers come without the
guardrails: no label, no required citation, no budget on new terms, and
nothing recorded. If something worth recording comes up anyway, they can say
so, and you can add it and deliver again.

Before the first delivery, show the name the feedback will be attributed to,
taken from `git config user.name`, and correct it if it is wrong. Do this
before committing rather than after, so a correction costs nothing.

Delivery is a single step, currently:

```sh
git add poc_audit_feedback/<this file>
git commit
git push
```

Stage that one file by path. Never `git add -A` or `git commit -a`: other
people work in this checkout, and unrelated changes are not yours to commit.
If the working tree holds unrelated changes, say so and leave them alone. If
the push is rejected, stop and report it. Do not force and do not rebase. The
record is already committed locally, so nothing is lost by stopping.

### Other delivery routes, for later

Delivery is deliberately one step so it can be replaced without touching
anything above it. A small service is the intended direction, so do not assume
the destination is git at all.

Two git-based alternatives, if transient records in the source tree become a
nuisance:

- An orphan branch holding only feedback, with nothing of the source tree in
  it. Checking it out swaps the working tree, which is disruptive mid-session.
- Writing the commit with plumbing (`hash-object`, `mktree`, `commit-tree`,
  `update-ref`) onto a ref that is never checked out, then pushing that ref.
  It touches neither the working tree nor the index, and it creates an
  ordinary branch, so the result is discoverable with `git log` and
  `git ls-remote`. The costs are manual parent bookkeeping and no
  working-tree representation.

## Not yet in scope

A dependency graph of the vocabulary, generated from the documents themselves,
would let a session know in advance which terms a question rests on instead of
discovering it while answering. Nothing generates one today. The terms
recorded against each exchange are the nearest thing available, and are
evidence to check such a graph against rather than a substitute for it.

There is deliberately no worked example of an answer. If one is ever added it
should use a fictional subject unrelated to this or any other particular body
of documents, so that it demonstrates the form without asserting anything that
can go stale, and it should be offered only when a participant says the form
is unclear.

This session records misalignments it happens to encounter while answering
questions. It does not sweep the corpus against the code looking for them.
That sweep is wanted later and is a different activity, so several sessions
reporting no misalignments does not mean the code agrees with the corpus.
