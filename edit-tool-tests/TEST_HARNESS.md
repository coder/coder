# File Edit Tool Test Harness

This document describes test scenarios for validating a file-editing tool.
Each scenario specifies:

- **Initial file content** (the file state before the edit)
- **Edit operation** (prose description of what to change)
- **Expected result** (the file state after the edit, or the expected error)
- **Verification** (what the agent must check after applying the edit)

The agent performing the test MUST read the file after every edit to verify
the result matches the expected output exactly. Use byte-level checks
(e.g. `xxd` or `cat -A`) when whitespace or line endings matter.

Each scenario references a fixture file in the `fixtures/` directory. The
fixture files contain the clean initial state. Before running a scenario,
the agent should copy or restore the fixture to avoid polluting it for
other scenarios.

---

## Category 1: Basic Operations

### Scenario 1.1: Simple single-line replacement

**Fixture:** `fixtures/1.1_basic.go.txt`

**Edit:** Replace `fmt.Println("hello world")` with
`fmt.Println("hello universe")`.

**Expected result:** Only line 6 changes. All other lines remain identical.

```go
package main

import "fmt"

func hello() {
	fmt.Println("hello universe")
}

func goodbye() {
	fmt.Println("goodbye world")
}

func main() {
	hello()
	goodbye()
}
```

**Verification:** Read back the file. Confirm only the target string changed.
Confirm `goodbye world` is untouched.

---

### Scenario 1.2: Mid-line (partial line) replacement

**Fixture:** `fixtures/1.2_partial.txt`

**Edit:** Replace `fox jumps` with `fox flies` (mid-line, partial match).

**Expected result:**

```text
the quick brown fox flies over the lazy dog
the quick brown cat sleeps on the lazy dog
```

**Verification:** The rest of line 1 (`over the lazy dog`) is preserved.
Line 2 is untouched.

---

### Scenario 1.3: Delete content (replace with empty string)

**Fixture:** `fixtures/1.3_delete.txt`

**Edit:** Replace `third\n` (including the newline) with an empty string.

**Expected result:**

```text
first
second
fourth
fifth
```

**Verification:** The file has exactly 4 lines. `fourth` immediately follows
`second`.

---

### Scenario 1.4: Expand single line to multiple lines

**Fixture:** `fixtures/1.4_expand.txt`

**Edit:** Replace `target line` with five new lines:

```text
expanded line 1
expanded line 2
expanded line 3
expanded line 4
expanded line 5
```

**Expected result:**

```text
before
expanded line 1
expanded line 2
expanded line 3
expanded line 4
expanded line 5
after
```

**Verification:** File now has 7 non-empty lines. `before` and `after` are
unchanged.

---

### Scenario 1.5: Collapse multiple lines to single line

**Fixture:** `fixtures/1.5_collapse.txt`

**Edit:** Replace the four lines `line A\nline B\nline C\nline D` with
`single collapsed line`.

**Expected result:**

```text
before
single collapsed line
after
```

**Verification:** File has exactly 3 non-empty lines.

---

### Scenario 1.6: Insert content between existing lines

**Fixture:** `fixtures/1.6_insert.txt`

**Edit:** Replace `second\nfourth` with `second\nthird\nfourth`
(inserting `third` between `second` and `fourth`).

**Expected result:**

```text
first
second
third
fourth
fifth
```

**Verification:** All original lines preserved. `third` inserted at line 3.

---

## Category 2: Duplicate/Ambiguous Content

### Scenario 2.1: Replace-all with repeated content

**Fixture:** `fixtures/2.1_replace_all.txt`

**Edit:** Replace ALL occurrences of `line one` with `LINE ONE`.

**Expected result:**

```text
LINE ONE
line two
line three
LINE ONE
line two
line three
LINE ONE
line two
line three
```

**Verification:** All 3 occurrences changed. `line two` and `line three`
untouched.

---

### Scenario 2.2: Ambiguous single replacement must fail

**Fixture:** `fixtures/2.2_ambiguous.txt`

**Edit:** Replace ONLY the first occurrence of `line two` with `LINE TWO`
(without using replace-all).

**Expected behavior:** The tool MUST reject this edit because `line two`
appears 3 times and there is no way to determine which one to change.
The file MUST remain unmodified after the error.

**Verification:** Read the file to confirm no changes were made.

---

### Scenario 2.3: Disambiguating with surrounding context

**Fixture:** `fixtures/2.3_disambiguate.txt`

**Edit:** Replace `LINE ONE\nline two\nline three\nLINE ONE` (spanning the
boundary of groups 1 and 2) with
`LINE ONE\nLINE TWO\nline three\nLINE ONE` - this uniquely identifies
the first/second group boundary.

**Expected result:**

```text
LINE ONE
LINE TWO
line three
LINE ONE
line two
line three
LINE ONE
line two
line three
```

**Verification:** Only the `line two` on the second line changed. The other
two `line two` occurrences are untouched.

---

### Scenario 2.4: Substring ambiguity

**Fixture:** `fixtures/2.4_substring.txt`

**Edit:** Replace `item_1` with `ITEM_1` (as a single unique replacement).

**Expected behavior:** The tool MUST reject this because `item_1` is a
substring of `item_10`, `item_11`, and `item_12`, giving 4+ matches.

**Verification:** File unchanged.

---

### Scenario 2.5: Disambiguating substring with newline boundary

**Fixture:** `fixtures/2.5_substring_newline.txt`

**Edit:** Replace `item_1\n` (including the trailing newline) with
`ITEM_1\n`.

**Expected result:**

```text
ITEM_1
item_10
item_11
item_12
item_2
item_20
```

**Verification:** Only the first line changed. `item_10` etc. are intact.

---

## Category 3: Whitespace and Indentation

### Scenario 3.1: Tab-indented content with exact tab matching

**Fixture:** `fixtures/3.1_tab_match.txt`

The file uses ACTUAL TAB characters on lines 1, 2, and 5, and 4 SPACES
on lines 3 and 4.

**Edit:** Replace `\ttab indented line 1` (with a real tab character) with
`\ttab indented line 1 EDITED`.

**Expected result:** Only line 1 changes. Tab character preserved in
the replacement. All other lines unchanged.

**Verification:** Use `cat -A` to confirm line 1 starts with `^I` (tab)
and no other lines changed.

---

### Scenario 3.2: MUST NOT fuzzy-match tabs as spaces

**Fixture:** `fixtures/3.2_fuzzy_tabs.txt`

The file uses ACTUAL TAB characters.

**Edit:** Search for `    tab indented line 1` (4 spaces instead of tab)
and replace with `    tab indented line 1 EDITED`.

**Expected behavior:** The tool MUST reject this edit because the search
string (using spaces) does not literally exist in the file (which uses
tabs). Fuzzy whitespace matching MUST NOT be performed because it can
corrupt files by changing indentation style.

**Verification:** File unchanged. Error returned.

---

### Scenario 3.3: MUST NOT fuzzy-match differing indentation levels

**Fixture:** `fixtures/3.3_fuzzy_indent.go.txt`

The file uses ACTUAL TAB characters for indentation.

**Edit:** Search for `deeply := "nested"\nfmt.Println(deeply)` (no
indentation) and replace with
`deeply := "very nested"\nfmt.Println(deeply)`.

**Expected behavior:** The tool MUST reject this edit. The search string
has no leading whitespace, but the file uses double-tab indentation on
those lines. Accepting this via fuzzy matching would strip the
indentation from the replacement, corrupt the file structure, and
potentially merge lines.

**Verification:** File unchanged. Error returned.

---

### Scenario 3.4: Trailing whitespace preservation

**Fixture:** `fixtures/3.4_trailing.txt`

Line 1 has 3 trailing spaces. Line 2 has no trailing spaces.

**Edit:** Replace `line with trailing spaces` (without the trailing spaces
in the search) with `line with trailing spaces EDITED`.

**Expected result:** The trailing 3 spaces after the original match survive
because they were not part of the search string. Alternatively, if the
tool is line-based, the replacement should overwrite the entire matched
region without leaving dangling trailing whitespace. Either behavior is
acceptable as long as it is consistent and documented.

**Verification:** Use `cat -A` to check exactly what trailing whitespace
exists on the edited line.

---

### Scenario 3.5: Blank line exact matching

**Fixture:** `fixtures/3.5_blank_exact.txt`

Two blank lines between `above` and `below`.

**Edit:** Search for `above\nbelow` (no blank lines) and replace with
`above\nbelow`.

**Expected behavior:** The tool MUST reject this because the file has
`above\n\n\nbelow`, not `above\nbelow`. Blank lines are significant
content.

**Verification:** File unchanged. Error returned.

---

### Scenario 3.6: Removing blank lines

**Fixture:** `fixtures/3.6_blank_remove.txt`

Two blank lines between `above` and `below`.

**Edit:** Replace `above\n\n\nbelow` (exact match including blank lines)
with `above\nbelow`.

**Expected result:**

```text
above
below
```

**Verification:** Only one newline between `above` and `below`.

---

## Category 4: Atomicity and Ordering

### Scenario 4.1: Multiple edits are atomic (all-or-nothing within a file)

**Fixture:** `fixtures/4.1_atomic.txt`

**Edit (two operations in one call):**

1. Replace `first` with `FIRST`
2. Replace `nonexistent string` with `NOPE`

**Expected behavior:** The entire batch MUST fail because edit #2 has no
match. Edit #1 MUST NOT be applied.

**Verification:** File is unchanged. `first` is still lowercase.

---

### Scenario 4.2: Multiple edits apply sequentially

**Fixture:** `fixtures/4.2_sequential.txt`

**Edit (two operations in one call):**

1. Replace `original` with `modified`
2. Replace `modified` with `FINAL`

**Expected result:**

```text
FINAL text here
```

**Verification:** Edit 2 successfully found `modified` (the output of
edit 1). The sequential application is correct.

---

### Scenario 4.3: Overlapping edits fail atomically

**Fixture:** `fixtures/4.3_overlapping.txt`

**Edit (two operations in one call):**

1. Replace `second\nthird` with `SECOND\nTHIRD`
2. Replace `third\nfourth` with `THIRD\nFOURTH`

**Expected behavior:** After edit 1 applies, `third` no longer exists in
its original form, so edit 2 cannot find `third\nfourth`. The entire
batch MUST fail. File MUST be unchanged.

**Verification:** File is unchanged. All lines still lowercase.

---

### Scenario 4.4: Multi-file atomicity

**Fixtures:** `fixtures/4.4_multi_a.txt`, `fixtures/4.4_multi_b.txt`

**Edit (one call, two files):**

1. In file A: replace `file A content` with `file A EDITED`
2. In file B: replace `nonexistent string` with `NOPE`

**Expected behavior:** File B's edit fails. File A MUST also remain
unchanged (cross-file atomicity).

**Verification:** Both files unchanged.

---

### Scenario 4.5: Chained edits across sequential operations

**Fixture:** `fixtures/4.5_chained.txt`

**Edit (three operations in one call):**

1. Replace `step1` with `STEP1`
2. Replace `STEP1\nstep2` with `STEP1\nSTEP2`
3. Replace `STEP1\nSTEP2\nstep3` with `STEP1\nSTEP2\nSTEP3`

**Expected result:**

```text
STEP1
STEP2
STEP3
```

**Verification:** Each edit builds on the previous one's result. All three
applied correctly.

---

### Scenario 4.6: Multiple independent edits in same file

**Fixture:** `fixtures/4.6_independent.txt`

**Edit (five operations in one call):**

1. Replace `AAA` with `aaa`
2. Replace `BBB` with `bbb`
3. Replace `CCC` with `ccc`
4. Replace `DDD` with `ddd`
5. Replace `EEE` with `eee`

**Expected result:**

```text
aaa
bbb
ccc
ddd
eee
```

**Verification:** All five edits applied. No ordering issues.

---

## Category 5: Special Characters

### Scenario 5.1: Regex metacharacters treated as literals

**Fixture:** `fixtures/5.1_regex.txt`

**Edit:** Replace `^start .* end$ [a-z]+ (group|or)` with
`^START .* END$ [A-Z]+ (GROUP|OR)`.

**Expected result:**

```text
Regex-like: ^START .* END$ [A-Z]+ (GROUP|OR)
```

**Verification:** All regex metacharacters (`^`, `$`, `.*`, `[]`, `+`,
`|`, `()`) treated as literal text, not as patterns.

---

### Scenario 5.2: Backslashes

**Fixture:** `fixtures/5.2_backslash.txt`

**Edit:** Replace `C:\Users\test` with `D:\Programs\app`.

**Expected result:**

```text
Backslashes: D:\Programs\app \n \t \\
```

**Verification:** Backslashes in both search and replace are treated as
literal characters.

---

### Scenario 5.3: Shell expansion characters

**Fixture:** `fixtures/5.3_shell.txt`

**Edit:** Replace `$PATH ${HOME}` with `$GOPATH ${GOROOT}`.

**Expected result:**

```text
This has: $GOPATH ${GOROOT} `backticks` "quotes" 'single'
```

**Verification:** Dollar signs, braces, backticks, and quotes all preserved
literally.

---

### Scenario 5.4: Unicode and accented characters

**Fixture:** `fixtures/5.4_unicode.txt`

**Edit:** Replace `café résumé` with `cafe resume` (remove accents).

**Expected result:**

```text
Unicode: cafe resume naïve über straße
```

**Verification:** Remaining Unicode characters (`naïve`, `über`, `straße`)
unchanged.

---

### Scenario 5.5: Emoji content

**Fixture:** `fixtures/5.5_emoji.txt`

**Edit:** Replace `🎉 🚀 ❌ ✅` with `🎉 🚀 ❌ ✅ 🔥 💯`.

**Expected result:**

```text
Emoji: 🎉 🚀 ❌ ✅ 🔥 💯
```

**Verification:** All emoji characters preserved and two new ones appended.

---

### Scenario 5.6: JSON content with nested quotes

**Fixture:** `fixtures/5.6_json.json`

**Edit:** Replace the two dependency lines:

```json
    "react": "^18.0.0",
    "react-dom": "^18.0.0"
```

with:

```json
    "react": "^19.0.0",
    "react-dom": "^19.0.0"
```

**Expected result:** Version numbers updated. JSON structure intact.
Indentation (4 spaces) preserved.

**Verification:** Parse the result as JSON to confirm validity.

---

## Category 6: Line Endings

### Scenario 6.1: CRLF file editing

**Fixture:** `fixtures/6.1_crlf.txt` (CRLF line endings)

**Edit:** Replace `line two` with `line two EDITED`.

**Expected result:** The edited line retains CRLF line ending.
All other lines unchanged.

**Verification:** Use `xxd` to confirm `\r\n` (0d 0a) on every line.

---

### Scenario 6.2: Search with LF in CRLF file must match correctly

**Fixture:** `fixtures/6.2_crlf_search.txt` (CRLF line endings)

**Edit:** Search for `line one\nline two` (using LF) and replace with
`line one EDITED\nline two EDITED`.

**Expected behavior:** Either:

- (a) The tool normalizes line endings during matching and preserves the
  original CRLF style in the output, OR
- (b) The tool rejects the search because `\n` does not match `\r\n`

Both are acceptable. What is NOT acceptable:

- Merging lines
- Leaving mixed line endings
- Consuming the CRLF boundary and losing a newline

**Verification:** Use `xxd` to check no lines were merged and line
endings are consistent.

---

### Scenario 6.3: No trailing newline preserved

**Fixture:** `fixtures/6.3_no_trailing_nl.txt` (no trailing newline)

**Edit:** Replace `no trailing newline` with `no trailing newline EDITED`.

**Expected result:** File still has no trailing newline. Content is
`no trailing newline EDITED` with no `\n` at end.

**Verification:** `xxd` confirms last byte is `44` (D), not `0a`.

---

## Category 7: Error Handling

### Scenario 7.1: Search string not found

**Fixture:** `fixtures/7.1_not_found.txt`

**Edit:** Replace `goodbye world` with `something`.

**Expected behavior:** Error indicating search string not found. File
unchanged.

---

### Scenario 7.2: File does not exist

**Edit:** Edit `/nonexistent/path/file.txt`.

**Expected behavior:** Error indicating file not found (404 or equivalent).

---

### Scenario 7.3: Empty search string

**Fixture:** Any non-empty file.

**Edit:** Replace empty string `""` with `new text`.

**Expected behavior:** Error. Empty search matches at every position and
is meaningless.

---

### Scenario 7.4: Search in empty file

**Fixture:** `fixtures/7.4_empty.txt` (0 bytes)

**Edit:** Replace `anything` with `something`.

**Expected behavior:** Error indicating search string not found.

---

## Category 8: Boundary Conditions

### Scenario 8.1: Edit first line of file

**Fixture:** `fixtures/8.1_first_line.txt`

**Edit:** Replace `FIRST LINE` with `NEW FIRST LINE`.

**Expected result:**

```text
NEW FIRST LINE
middle
LAST LINE
```

**Verification:** Line 1 changed. No content prepended or duplicated.

---

### Scenario 8.2: Edit last line of file

**Fixture:** `fixtures/8.2_last_line.txt`

**Edit:** Replace `LAST LINE` with `NEW LAST LINE`.

**Expected result:**

```text
FIRST LINE
middle
NEW LAST LINE
```

**Verification:** Last line changed. Trailing newline status preserved.

---

### Scenario 8.3: Replace entire file content

**Fixture:** `fixtures/8.3_entire.txt`

**Edit:** Replace `line 1\nline 2\nline 3\n` (the entire content) with
`completely new content\nwith different structure`.

**Expected result:**

```text
completely new content
with different structure
```

**Verification:** Only 2 lines remain. No remnants of original content.

---

### Scenario 8.4: Very large file (500+ lines)

**Fixture:** `fixtures/8.4_large.txt` (500 lines)

**Edit:** Replace
`line 250 content here with some padding text to make it wider` with
`line 250 REPLACED`.

**Expected result:** Only line 250 changes. All other 499 lines untouched.

**Verification:** Lines 249 and 251 still contain their original content.

---

### Scenario 8.5: Very long lines (5000+ characters)

**Fixture:** `fixtures/8.5_longline.txt` (line 1 is 5000 `x` characters,
line 2 is `short`)

**Edit:** Replace `short` with `SHORT`.

**Expected result:** Line 1 (the 5000 x's) is preserved. Line 2 is `SHORT`.

**Verification:** `wc -c` confirms file size changed by exactly 0 bytes
(same length replacement). Line 1 is still 5000 characters.

---

## Category 9: Structural Code Edits

### Scenario 9.1: Change function signature

**Fixture:** `fixtures/9.1_signature.go.txt`

**Edit:** Replace the entire function to add error return and a guard
clause:

```go
func NewOverlayFS(baseFS fs.FS, overlays []Overlay) (fs.FS, error) {
	if len(overlays) == 0 {
		return baseFS, nil
	}
	return overlayFS{
		baseFS:   baseFS,
		overlays: overlays,
	}, nil
}
```

**Expected result:** Function has new signature and body. Surrounding code
unchanged.

**Verification:** The file compiles (if part of a compilable unit). The
function's closing brace is properly placed.

---

### Scenario 9.2: Add entry to map literal

**Fixture:** `fixtures/9.2_map_literal.go.txt`

**Edit:** Replace `"a": 1,\n\t\t"b": 2,` with
`"a": 1,\n\t\t"b": 2,\n\t\t"c": 3,`.

**Expected result:**

```go
func valid() {
	x := map[string]int{
		"a": 1,
		"b": 2,
		"c": 3,
	}
	_ = x
}
```

**Verification:** Proper indentation on new entry. Closing brace unchanged.

---

### Scenario 9.3: Edit one of multiple structurally identical blocks

**Fixture:** `fixtures/9.3_identical_blocks.go.txt`

**Edit:** Change only `processB`'s body. Use the function name as
disambiguating context.

Search for:

```go
func processB() {
	log.Println("processing")
	doWork()
	log.Println("done")
}
```

Replace with:

```go
func processB() {
	log.Println("processing B")
	doWork()
	log.Println("done B")
}
```

**Expected result:** Only processB changed. processA and processC untouched.

**Verification:** Search for `"processing"` - should appear exactly 2 times
(in A and C). `"processing B"` appears exactly once.

---

### Scenario 9.4: Disambiguate identical nested blocks

**Fixture:** `fixtures/9.4_nested.go.txt`

**Edit:** Add `logC()` after `doC()`. The search must include enough of
the unique content (`doC()`) to identify which block to edit.

**Expected result:** `logC()` appears on the line after `doC()`, at the
same indentation level. The `doE()` block is untouched.

---

### Scenario 9.5: YAML editing (indentation-sensitive)

**Fixture:** `fixtures/9.5_config.yaml`

**Edit:** Replace `key1: value1\n  key2: value2` with
`key1: newvalue1\n  key2: newvalue2\n  key3: value3`.

**Expected result:** New key added, existing keys updated. YAML indentation
preserved. The `nested` section is untouched.

**Verification:** The file is valid YAML. `nested` block still has 2-space
indent under `data`.

---

## Category 10: Self-referential and Edge Cases

### Scenario 10.1: Replacement contains the search string

**Fixture:** `fixtures/10.1_self_ref.txt`

**Edit:** Replace `TODO: implement this` with
`// TODO: implement this\nfunc placeholder() {}`.

**Expected result:**

```text
// TODO: implement this
func placeholder() {}
```

**Verification:** No infinite loop. The search string appears in the
replacement but was not re-matched.

---

### Scenario 10.2: Creating a duplicate via replacement, then editing one

**Fixture:** `fixtures/10.2_create_dup.txt`

**Edit 1:** Replace `unique_a` with `duplicate`.
**Edit 2:** Replace `unique_c` with `duplicate`.

**After edits 1 and 2:**

```text
duplicate
unique_b
duplicate
```

**Edit 3:** Replace only the first `duplicate` with `first_dup`. This
requires including context: search for `duplicate\nunique_b`.

**Expected result after edit 3:**

```text
first_dup
unique_b
duplicate
```

**Verification:** Only the first `duplicate` changed.

---

### Scenario 10.3: Symlink editing

**Fixture:** `fixtures/10.3_real_file.txt` and
`fixtures/10.3_symlink.txt` (symlink to real_file.txt)

**Edit:** Edit `10.3_symlink.txt`, replacing `real file content` with
`REAL FILE CONTENT`.

**Expected result:** The edit modifies the target file's content. The
symlink remains a symlink (not replaced with a regular file).

**Verification:** `ls -la` confirms symlink still exists.
`cat real_file.txt` confirms content changed.

---

### Scenario 10.4: Read-only file

**Fixture:** `fixtures/10.4_readonly.txt` (permissions 444)

**Edit:** Attempt to replace `readonly content` with `READONLY CONTENT`.

**Expected behavior:** Document the observed behavior. This is a
platform-dependent scenario. The tool may:

- (a) Succeed (if it elevates privileges or uses a different write
  mechanism)
- (b) Fail with a permissions error

Either is acceptable as long as it is consistent.

---

### Scenario 10.5: Replace-all disabled does not use fuzzy matching

**Fixture:** `fixtures/10.5_fuzzy_replace_all.txt` (tab-indented)

**Edit (with replace-all):** Search for `  item` (2 spaces, NOT tab) and
replace all with `ITEM`.

**Expected behavior:** The tool MUST reject this. The search uses spaces
but the file uses tabs. Replace-all should use exact matching only.

**Verification:** File unchanged. Error returned.

---

## Category 11: Multi-file Operations

### Scenario 11.1: Simultaneous edits to multiple files

**Fixtures:** `fixtures/11.1_multi_a.txt`, `fixtures/11.1_multi_b.txt`

**Edit (single call):**

1. In file A: replace `file A content` with `file A EDITED`
2. In file B: replace `file B content` with `file B EDITED`

**Expected result:** Both files updated.

**Verification:** Read both files to confirm.

---

### Scenario 11.2: Cross-file atomicity on failure

**Fixtures:** `fixtures/11.2_multi_a.txt`, `fixtures/11.2_multi_b.txt`

**Edit (single call):**

1. In file A: replace `file A content` with `file A EDITED`
2. In file B: replace `nonexistent text` with `NOPE`

**Expected behavior:** File B's edit fails. File A MUST also remain
unchanged.

**Verification:** Both files have original content.

---

## Category 12: Documented Dangerous Behaviors (Current Tool)

These scenarios document behaviors observed in the current `edit_files`
tool that an alternative implementation should handle differently.
They represent bugs or dangerous behaviors.

### Scenario 12.1: Fuzzy whitespace matching corrupts indentation

**Observed behavior (BUG):** When the search string uses spaces but the
file uses tabs, the current tool performs fuzzy matching. This succeeds
but corrupts the file:

- Indentation style changes from tabs to spaces
- Line boundaries may be consumed, merging lines
- The replacement's whitespace is used literally instead of preserving
  the file's original indentation

**Example:** File has `\tvalue := 1`, search is `  value := 1` (spaces).
The tool matches, replaces, and the result loses tab indentation.

**Correct behavior:** The tool should reject the edit as "not found"
because the search string does not literally appear in the file.

---

### Scenario 12.2: Fuzzy matching consumes line boundaries

**Observed behavior (BUG):** When fuzzy whitespace matching finds a
near-match, it may consume newline characters and adjacent whitespace
as part of the "whitespace normalization." This causes lines to merge.

**Example:** File has:

```text
	item one
	item two
```

Search for `  item one` (spaces). The tool fuzzy-matches and may consume
`\titem one\n\t`, causing `item two` to lose its newline separator.

**Correct behavior:** Reject the edit as not found.

---

### Scenario 12.3: CRLF line ending consumption

**Observed behavior (BUG):** When searching with `\n` in a CRLF file,
the tool may consume the `\r\n` boundary differently than expected,
potentially merging lines.

**Correct behavior:** Either normalize for matching and preserve original
endings, or reject the search as not found.

---

## Running the Test Harness

An agent evaluating a file-edit tool should:

1. For each scenario, copy the fixture file to a working location (do not
   edit fixtures in place).
2. Attempt the described edit operation on the copy.
3. Read the file back (always).
4. For whitespace-sensitive scenarios, use `cat -A` or `xxd` to verify at
   the byte level.
5. Compare the actual result to the expected result.
6. For error scenarios, verify the file is unchanged after the error.
7. Record PASS/FAIL for each scenario.
8. Report a summary at the end.

### Pass Criteria

- All scenarios in Categories 1-11 must pass.
- Category 12 scenarios document known bugs. An alternative
  implementation passes if it handles them correctly (rejects fuzzy
  matches, preserves line endings).
