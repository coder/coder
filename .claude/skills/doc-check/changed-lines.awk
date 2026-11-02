# Print each line a diff adds, as `path:line: text`, with the line number on
# the new side. Input is `git diff --unified=0` output. Handles CRLF files,
# paths with spaces or quotes, deleted files, and added lines that start
# with `++` or `--`.
{ sub(/\r$/, "") }
/^diff --git / { file = ""; header = 1; next }
header && /^\+\+\+ / {
	file = substr($0, 5)
	# Git ends the header with a tab when the path contains a space.
	sub(/\t$/, "", file)
	if (file ~ /^"/) {
		gsub(/^"|"$/, "", file)
	}
	if (file == "/dev/null") {
		file = ""
	} else {
		sub(/^b\//, "", file)
	}
	next
}
header && /^--- / { next }
/^@@ / {
	header = 0
	split($3, range, ",")
	line = substr(range[1], 2) + 0
	next
}
header { next }
/^\+/ {
	if (file != "") {
		print file ":" line ": " substr($0, 2)
	}
	line++
	next
}
