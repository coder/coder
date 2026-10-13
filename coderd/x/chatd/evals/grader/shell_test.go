package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// isEdit decides which execute calls test_after_edit treats as edits, so
// a missed write idiom passes an untested edit and a false positive fails
// a trial that tested after its real last edit.
func TestExecuteEditDetection(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		command string
		edit    bool
	}{
		{"python heredoc write_text", "python3 - <<'EOF'\nimport pathlib\npathlib.Path('a.py').write_text('x')\nEOF", true},
		{"python json.dump to opened file", `python3 -c "import json; json.dump({}, open('a.json', 'w'))"`, true},
		{"python print to opened file", "python3 - <<'EOF'\nwith open('a.txt', mode='a') as f:\n    print('x', file=f)\nEOF", true},
		{"python os.remove", `python3 -c "import os; os.remove('a.py')"`, true},
		{"python script piped from cat heredoc", "cat <<'EOF' | python3\nopen('a.py', 'w').close()\nEOF", true},
		{"python code piped from echo", `echo "open('a.py', 'w').close()" | python3 -`, true},
		{"ruby File.open block", `ruby -e "File.open('a.rb', 'w') { |f| f.puts 'x' }"`, true},
		{"perl open and print", `perl -e 'open(my $fh, ">", "a.pl"); print $fh "x";'`, true},
		{"perl open without parentheses", `perl -e 'open my $fh, ">>", "a.pl"; print $fh "x"'`, true},
		{"node appendFileSync", `node -e "require('fs').appendFileSync('a.js', 'x')"`, true},
		{"node --eval", `node --eval "require('fs').writeFileSync('a.js', 'x')"`, true},
		{"php -r", `php -r "file_put_contents('a.php', 'x');"`, true},
		{"php heredoc", "php <<'EOF'\n<?php file_put_contents('a.php', 'x');\nEOF", true},
		{"bash -c nested python", `bash -c "python3 -c \"open('a.py', 'w').write('x')\""`, true},
		{"bash -lc nested sed", `bash -lc 'sed -i s/a/b/ a.py'`, true},
		{"redirect", "echo x > a.py", true},
		{"python reads a file", `python3 -c "print(open('a.py').read())"`, false},
		{"python reads with an explicit mode", `python3 -c "print(open('a.py', 'rb').read())"`, false},
		{"python writes to stdout", `python3 -c "import sys; sys.stdout.write('done')"`, false},
		{"python mentions rewrite", `python3 -c "print(rewrite_rules('a.py'))"`, false},
		{"python csv writer to stdout", `python3 -c "import csv, sys; csv.writer(sys.stdout).writerow(['a'])"`, false},
		{"python prints the word write", `python3 -c "print('write the file')"`, false},
		{"ruby puts", `ruby -e "puts File.read('a.rb')"`, false},
		{"bash -c read only", `bash -c 'grep -n x a.py'`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			args, err := json.Marshal(map[string]string{"command": tc.command})
			require.NoError(t, err)
			require.Equal(t, tc.edit, isEdit(toolCall{Name: "execute", Args: args}))
		})
	}
}
