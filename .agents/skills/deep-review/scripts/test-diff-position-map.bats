#!/usr/bin/env bats

# Tests for diff-position-map.sh

setup() {
	SCRIPT_DIR="$(cd "$(dirname "$BATS_TEST_FILENAME")" && pwd)"
	SCRIPT="$SCRIPT_DIR/diff-position-map.sh"
}

@test "single file, single hunk" {
	diff_input='diff --git a/main.go b/main.go
index 1111111..2222222 100644
--- a/main.go
+++ b/main.go
@@ -10,4 +10,6 @@ package main
 context line 10
 context line 11
+added line 12
+added line 13
 context line 12 old
 context line 13 old'

	result="$(echo "$diff_input" | "$SCRIPT")"

	# @@ header = position 1
	# context 10 = position 2
	# context 11 = position 3
	# +added 12  = position 4
	# +added 13  = position 5
	# context (now 14) = position 6
	# context (now 15) = position 7
	[ "$(echo "$result" | jq -r '.["main.go"]["12"]')" = "4" ]
	[ "$(echo "$result" | jq -r '.["main.go"]["13"]')" = "5" ]
	# Context lines also get mapped.
	[ "$(echo "$result" | jq -r '.["main.go"]["10"]')" = "2" ]
	[ "$(echo "$result" | jq -r '.["main.go"]["11"]')" = "3" ]
}

@test "single file, multiple hunks — position continues across hunks" {
	diff_input='diff --git a/file.go b/file.go
index 1111111..2222222 100644
--- a/file.go
+++ b/file.go
@@ -1,3 +1,4 @@
 line 1
+added line 2
 line 2 old
 line 3 old
@@ -20,3 +21,4 @@ some function
 line 21
+added line 22
 line 22 old
 line 23 old'

	result="$(echo "$diff_input" | "$SCRIPT")"

	# First hunk:
	# @@ = position 1
	# context 1 = position 2
	# +added 2  = position 3
	# context 3 = position 4
	# context 4 = position 5
	[ "$(echo "$result" | jq -r '.["file.go"]["2"]')" = "3" ]

	# Second hunk:
	# @@ = position 6 (continues from 5)
	# context 21 = position 7
	# +added 22  = position 8
	# context 23 = position 9
	# context 24 = position 10
	[ "$(echo "$result" | jq -r '.["file.go"]["22"]')" = "8" ]
	# Verify positions didn't reset.
	[ "$(echo "$result" | jq -r '.["file.go"]["21"]')" = "7" ]
}

@test "multiple files — separate position namespaces" {
	diff_input='diff --git a/a.go b/a.go
index 1111111..2222222 100644
--- a/a.go
+++ b/a.go
@@ -1,2 +1,3 @@
 line 1
+added line 2
 line 2 old
diff --git a/b.go b/b.go
index 3333333..4444444 100644
--- a/b.go
+++ b/b.go
@@ -5,2 +5,3 @@ package b
 line 5
+added line 6
 line 6 old'

	result="$(echo "$diff_input" | "$SCRIPT")"

	# a.go: @@ pos=1, ctx 1 pos=2, +added 2 pos=3, ctx 3 pos=4
	[ "$(echo "$result" | jq -r '.["a.go"]["2"]')" = "3" ]

	# b.go: position resets for new file.
	# @@ pos=1, ctx 5 pos=2, +added 6 pos=3, ctx 7 pos=4
	[ "$(echo "$result" | jq -r '.["b.go"]["6"]')" = "3" ]

	# Verify both files exist.
	[ "$(echo "$result" | jq 'keys | length')" = "2" ]
}

@test "new file (all additions)" {
	diff_input='diff --git a/new.go b/new.go
new file mode 100644
index 0000000..1234567
--- /dev/null
+++ b/new.go
@@ -0,0 +1,3 @@
+package new
+
+func init() {}'

	result="$(echo "$diff_input" | "$SCRIPT")"

	# @@ = position 1
	# +package new = position 2 (line 1)
	# +(blank)     = position 3 (line 2)
	# +func init   = position 4 (line 3)
	[ "$(echo "$result" | jq -r '.["new.go"]["1"]')" = "2" ]
	[ "$(echo "$result" | jq -r '.["new.go"]["2"]')" = "3" ]
	[ "$(echo "$result" | jq -r '.["new.go"]["3"]')" = "4" ]
}

@test "deleted file — no new-side mappings" {
	diff_input='diff --git a/old.go b/old.go
deleted file mode 100644
index 1234567..0000000
--- a/old.go
+++ /dev/null
@@ -1,3 +0,0 @@
-package old
-
-func gone() {}'

	result="$(echo "$diff_input" | "$SCRIPT")"

	# No new-side lines, so old.go should have no entries (or not appear).
	[ "$(echo "$result" | jq 'if .["old.go"] then .["old.go"] | length else 0 end')" = "0" ]
}

@test "context lines increment position and produce new-side mappings" {
	diff_input='diff --git a/ctx.go b/ctx.go
index 1111111..2222222 100644
--- a/ctx.go
+++ b/ctx.go
@@ -5,5 +5,5 @@ package ctx
 alpha
 beta
-old gamma
+new gamma
 delta
 epsilon'

	result="$(echo "$diff_input" | "$SCRIPT")"

	# @@ = position 1
	# ctx alpha (5) = position 2
	# ctx beta  (6) = position 3
	# -old gamma    = position 4 (no mapping)
	# +new gamma(7) = position 5
	# ctx delta (8) = position 6
	# ctx epsilon(9)= position 7
	[ "$(echo "$result" | jq -r '.["ctx.go"]["5"]')" = "2" ]
	[ "$(echo "$result" | jq -r '.["ctx.go"]["6"]')" = "3" ]
	[ "$(echo "$result" | jq -r '.["ctx.go"]["7"]')" = "5" ]
	[ "$(echo "$result" | jq -r '.["ctx.go"]["8"]')" = "6" ]
	[ "$(echo "$result" | jq -r '.["ctx.go"]["9"]')" = "7" ]
}

@test "mixed add/delete/context — line numbers track correctly" {
	diff_input='diff --git a/mix.go b/mix.go
index 1111111..2222222 100644
--- a/mix.go
+++ b/mix.go
@@ -1,7 +1,7 @@
 line one
-removed A
-removed B
+added A
+added B
+added C
 line four
-removed D
 line five'

	result="$(echo "$diff_input" | "$SCRIPT")"

	# @@ = position 1
	# ctx "line one" new=1 = position 2
	# -removed A            = position 3
	# -removed B            = position 4
	# +added A     new=2    = position 5
	# +added B     new=3    = position 6
	# +added C     new=4    = position 7
	# ctx "line four" new=5 = position 8
	# -removed D            = position 9
	# ctx "line five" new=6 = position 10
	[ "$(echo "$result" | jq -r '.["mix.go"]["1"]')" = "2" ]
	[ "$(echo "$result" | jq -r '.["mix.go"]["2"]')" = "5" ]
	[ "$(echo "$result" | jq -r '.["mix.go"]["3"]')" = "6" ]
	[ "$(echo "$result" | jq -r '.["mix.go"]["4"]')" = "7" ]
	[ "$(echo "$result" | jq -r '.["mix.go"]["5"]')" = "8" ]
	[ "$(echo "$result" | jq -r '.["mix.go"]["6"]')" = "10" ]
	# No mapping for deleted lines — check there's no "line 0" weirdness.
	[ "$(echo "$result" | jq -r '.["mix.go"] | length')" = "6" ]
}

@test "--file filter outputs only requested file" {
	diff_input='diff --git a/keep.go b/keep.go
index 1111111..2222222 100644
--- a/keep.go
+++ b/keep.go
@@ -1,2 +1,3 @@
 line 1
+added line 2
 line 2 old
diff --git a/skip.go b/skip.go
index 3333333..4444444 100644
--- a/skip.go
+++ b/skip.go
@@ -1,2 +1,3 @@
 line 1
+added line 2
 line 2 old'

	result="$(echo "$diff_input" | "$SCRIPT" --file keep.go)"

	# Only keep.go should appear.
	[ "$(echo "$result" | jq 'keys | length')" = "1" ]
	[ "$(echo "$result" | jq 'keys[0]')" = '"keep.go"' ]
	[ "$(echo "$result" | jq -r '.["keep.go"]["2"]')" = "3" ]
}

@test "empty diff produces empty object" {
	result="$(echo "" | "$SCRIPT")"
	[ "$result" = "{}" ]
}
