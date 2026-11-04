# Lists the rule sections of Markdown pages: every level-2 and level-3
# heading outside fenced code blocks, as "path<TAB>line<TAB>heading".
# Headings inside a fence are examples, not rules, so they are skipped.
#
# Usage: awk -f rule-sections.awk <page>...
#
# A fence opens with 3 or more backticks or tildes, indented at most 3
# spaces, and closes with at least as many of the same character and
# nothing else on the line. Handles CRLF line endings.

FNR == 1 { fence = "" }

{
	sub(/\r$/, "")
	line = $0
	match(line, /^ */)
	if (RLENGTH <= 3) {
		line = substr(line, RLENGTH + 1)
	}
}

fence == "" && match(line, /^(```+|~~~+)/) {
	fence = substr(line, 1, RLENGTH)
	next
}

fence != "" {
	if (match(line, /^(```+|~~~+)[ \t]*$/)) {
		run = line
		sub(/[ \t]*$/, "", run)
		if (substr(run, 1, 1) == substr(fence, 1, 1) && length(run) >= length(fence)) {
			fence = ""
		}
	}
	next
}

/^(## |### )/ {
	heading = $0
	sub(/^#+ /, "", heading)
	sub(/[ \t]+$/, "", heading)
	print FILENAME "\t" FNR "\t" heading
}
