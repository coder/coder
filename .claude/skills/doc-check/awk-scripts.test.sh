#!/bin/bash
# Regression tests for the doc-check awk scripts. Run it from anywhere:
#
#   bash .claude/skills/doc-check/awk-scripts.test.sh
#
# changed-lines.awk: builds a throwaway repository, makes a commit with the
# cases the parser must handle, and compares the output with the expected
# lines.
#
# rule-sections.awk: lists the rule sections of pages that mix real
# headings with example headings inside fenced code blocks.

set -euo pipefail

skill_dir="$(cd "$(dirname "$0")" && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
failed=0

check() {
	local name=$1 expected=$2 actual=$3
	if [ "$actual" != "$expected" ]; then
		echo "$name output does not match." >&2
		diff <(printf '%s\n' "$expected") <(printf '%s\n' "$actual") >&2 || true
		failed=1
	fi
}

# changed-lines.awk

repo="$work/repo"
mkdir -p "$repo"
cd "$repo"

git init -q
git config user.email test@example.com
git config user.name test
git config core.autocrlf false

mkdir -p "docs/sub dir" docs/reference/cli docs/.style/style-guide
printf 'line one\nline two\nline three\n' >docs/a.md
printf 'crlf one\r\ncrlf two\r\n' >docs/crlf.md
printf 'keep\n' >"docs/sub dir/spaced.md"
printf 'gone\n' >docs/deleted.md
printf 'generated\n' >docs/reference/cli/x.md
printf 'rule\n' >docs/.style/style-guide/r.md
git add -A
git commit -qm base
base=$(git rev-parse HEAD)

# Edits: an added line that starts with "++" and one that starts with "--",
# which look like diff headers; CRLF line endings; a path with a space; a
# deleted page; a new page; and files outside the review scope.
printf 'line one\n++ starts with plus plus\nline two edited\nline three\n-- starts with dashes\n' >docs/a.md
printf 'crlf one\r\ncrlf two changed\r\ncrlf three\r\n' >docs/crlf.md
printf 'keep\nadded in spaced path\n' >"docs/sub dir/spaced.md"
git rm -q docs/deleted.md
printf 'brand new\npage\n' >docs/new.md
printf 'generated, changed\n' >docs/reference/cli/x.md
printf 'rule, changed\n' >docs/.style/style-guide/r.md
printf 'not docs\n' >README.md
git add -A
git commit -qm head
head=$(git rev-parse HEAD)

actual=$(git diff --no-color --no-ext-diff --unified=0 "$base" "$head" -- 'docs/*.md' \
	':(exclude)docs/reference/api' ':(exclude)docs/reference/cli' ':(exclude)docs/.style/style-guide' |
	awk -f "$skill_dir/changed-lines.awk")

check changed-lines.awk 'docs/a.md:2: ++ starts with plus plus
docs/a.md:3: line two edited
docs/a.md:5: -- starts with dashes
docs/crlf.md:2: crlf two changed
docs/crlf.md:3: crlf three
docs/new.md:1: brand new
docs/new.md:2: page
docs/sub dir/spaced.md:2: added in spaced path' "$actual"

# rule-sections.awk

cd "$work"
cat >rules.md <<'EOF'
# Page title

## Rule one

```markdown
## Example inside a backtick fence
```

### Rule two  

````markdown
```sh
## Example inside a nested fence
```
## Still inside the four-backtick fence
````

~~~text
## Example inside a tilde fence
~~~

  ```
  ## Example inside an indented fence
  ```

    ```
## Rule three, after a four-space indent that is not a fence

#### Too deep to be a rule section
EOF
# The backticks are Markdown fences, not command substitution.
# shellcheck disable=SC2016
printf '## CRLF rule\r\n\r\n```\r\n## CRLF example\r\n```\r\n' >crlf.md
# shellcheck disable=SC2016
printf '## Unclosed\n\n```\n## Hidden\n' >unclosed.md
printf '## First in the next file\n' >next.md

actual=$(awk -f "$skill_dir/rule-sections.awk" rules.md crlf.md unclosed.md next.md)

check rule-sections.awk "$(printf '%s\t%s\t%s\n' \
	rules.md 3 'Rule one' \
	rules.md 9 'Rule two' \
	rules.md 27 'Rule three, after a four-space indent that is not a fence' \
	crlf.md 1 'CRLF rule' \
	unclosed.md 1 'Unclosed' \
	next.md 1 'First in the next file')" "$actual"

if [ "$failed" -ne 0 ]; then
	exit 1
fi
echo "doc-check awk scripts: all cases pass."
