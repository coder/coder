#!/usr/bin/env python3
"""Report markdown fragment links that do not identify a unique heading.

Duplicate headings are legal and sometimes deliberate. `security_findings.md`
repeats "Problem" and "Solution" beneath each finding, and that parallelism is
worth keeping. Linking to one of them is the unsafe part. GitHub resolves
`#problem` to the first occurrence and numbers the rest `#problem-1` upward,
so a bare link is ambiguous and a numbered link silently retargets when a
heading with the same text is inserted above it. This checks the links rather
than the headings, so the cost falls only where a document is actually
cross referenced.

Usage:

    python3 check_anchors.py [directory]    # defaults to the current one

Exits 1 when anything is reported, so it can gate a commit or a CI step.

Findings:

    AMBIGUOUS            The fragment matches more than one heading. GitHub
                         resolves it to the first.
    ORDER-DEPENDENT      The fragment names an occurrence by number. It
                         resolves today and retargets if a heading with the
                         same text is inserted before it.
    BROKEN               The fragment matches no heading in the target file.
    UNKNOWN TARGET FILE  The link names a file this run has no table for.

Fixing a finding:

    AMBIGUOUS and ORDER-DEPENDENT have three remedies. Make the heading text
    unique, which is right when the repetition was accidental. Give the one
    heading you link to an explicit anchor, `<a id="p3-problem"></a>` on the
    line above it, which is right when the repetition is deliberate and only
    one occurrence is ever referenced. Or drop the fragment and link to the
    file, which is right when a reader can find the section unaided.

    Do not reach for `## Heading {#custom-id}`. markdownlint accepts it and
    GitHub does not implement it, so the braces render as literal text and
    land in the generated slug. It passes the lint and breaks where it is
    read.

    BROKEN and UNKNOWN TARGET FILE are ordinary mistakes. List the available
    targets with `grep -nE '^#{1,6} ' file.md`.

This does not replace markdownlint MD051, and neither covers the other.
MD051 checks that a same-file fragment resolves to something, but it accepts
both `#problem` and `#problem-1` because it has no notion of ambiguity, and
it does not follow links into other files. This script covers those two
gaps and nothing else.

One thing no checker on this subject reaches: whether the heading still says
what the link was written to point at. A section rewritten beneath an
unchanged heading leaves every link to it valid and wrong. That stays a human
problem, in the same class as the rendered diagrams drifting from their
sources.

Known defects
=============

These produce wrong output on input this corpus could plausibly contain.
They are recorded rather than fixed, deliberately. Nothing here exercises
them today, and a bulletproof version is not wanted yet.

1. Fragment links inside fenced code blocks are checked as though they were
   real links. `scan` tracks fences while collecting headings, but the link
   loop in `main` does not. A markdown example written inside a fence gets
   reported, usually as BROKEN or UNKNOWN TARGET FILE. False positive.

2. UNKNOWN TARGET FILE conflates two different faults. A link to a file that
   does not exist and a link to a file outside the scanned directory report
   identically, because only the `*.md` files in that one directory get a
   table and no existence test runs. `../CLAUDE.md#foo` is reported even
   when that file is present.

Scope limits
============

These leave the checker incomplete rather than wrong. A heading it fails to
see usually surfaces as a spurious BROKEN.

3. `slug` approximates GitHub's slugger instead of reproducing it. Backticks,
   emphasis, inline links, punctuation removal and whitespace collapsing are
   handled, and agree with markdownlint MD051 on the forms this corpus uses.
   Unicode and emoji were never tested. Any divergence appears as a link
   GitHub resolves being called BROKEN, or the reverse.

4. A heading whose text already ends in a number can collide with a generated
   duplicate suffix. "Problem 1" slugs to `problem-1`, which is also what a
   second "Problem" generates. The counts and the generated slugs then
   disagree, and the classification that follows is wrong.

5. Setext headings, underlined with `=` or `-`, are not recognised. Links to
   them report BROKEN. This corpus uses ATX headings throughout.

6. Reference style links (`[text][ref]` with a separate `[ref]:` definition)
   and angle bracket autolinks are not parsed, so their fragments go
   unchecked. Silent omission rather than a false report.

7. Four space indented code blocks are not treated as code. A line beginning
   with `#` inside one counts as a heading, which can inflate a duplicate
   count and make a sound link look AMBIGUOUS.

8. Only `*.md` directly inside the given directory is scanned. Subdirectories
   are neither checked nor available as link targets.

9. Explicit anchors are recognised only as `<a id="...">` or `<a name="...">`
   with double quotes. Other spellings are missed, so a link to one reports
   BROKEN.
"""
import re, sys, os, glob, collections

def slug(t):
    t = re.sub(r'`([^`]*)`', r'\1', t)
    t = re.sub(r'\*{1,2}([^*]*)\*{1,2}', r'\1', t)
    t = re.sub(r'\[([^\]]*)\]\([^)]*\)', r'\1', t)
    t = re.sub(r'[^\w\s-]', '', t.strip().lower())
    return re.sub(r'\s+', '-', t)

def scan(path):
    """Return (slugs, base_counts, explicit) for one file."""
    seen, slugs, explicit = collections.Counter(), set(), set()
    fence = False
    with open(path, encoding='utf-8') as fh:
        for line in fh:
            if line.startswith('```'):
                fence = not fence
                continue
            if fence:
                continue
            for m in re.finditer(r'<a\s+(?:id|name)="([^"]+)"', line):
                explicit.add(m.group(1))
            h = re.match(r'^#{1,6}\s+(.*?)\s*$', line)
            if h:
                b = slug(h.group(1))
                seen[b] += 1
                slugs.add(b if seen[b] == 1 else f'{b}-{seen[b]-1}')
    return slugs, seen, explicit

def report(f, n, kind, detail):
    print(f'{f}:{n}: {kind}  {detail}')

def main(root):
    files = sorted(glob.glob(os.path.join(root, '*.md')))
    tables = {os.path.abspath(f): scan(f) for f in files}
    bad = 0
    for f in files:
        with open(f, encoding='utf-8') as fh:
            lines = list(enumerate(fh, 1))
        for n, line in lines:
            for path, frag in re.findall(r'\]\(([^)#]*)#([^)\s]+)\)', line):
                base = os.path.dirname(f)
                tgt = os.path.abspath(os.path.join(base, path) if path else f)
                if tgt not in tables:
                    report(f, n, 'UNKNOWN TARGET FILE', f'{path}#{frag}')
                    bad += 1
                    continue
                slugs, counts, explicit = tables[tgt]
                if frag in explicit:
                    continue
                name = os.path.basename(tgt)
                m = re.match(r'^(.*)-(\d+)$', frag)
                if counts.get(frag, 0) > 1:
                    report(f, n, 'AMBIGUOUS', f'#{frag} matches {counts[frag]} headings '
                                             f'in {name}; resolves to the first')
                    bad += 1
                elif m and counts.get(m.group(1), 0) > 1:
                    report(f, n, 'ORDER-DEPENDENT', f'#{frag} is occurrence '
                                                   f'{int(m.group(2)) + 1} of "{m.group(1)}"; '
                                                   f'renumbers if a heading is inserted')
                    bad += 1
                elif frag not in slugs:
                    report(f, n, 'BROKEN', f'#{frag} matches no heading in {name}')
                    bad += 1
    print(f'{len(files)} file(s), {bad} problem(s)')
    return 1 if bad else 0

if __name__ == '__main__':
    sys.exit(main(sys.argv[1] if len(sys.argv) > 1 else '.'))
