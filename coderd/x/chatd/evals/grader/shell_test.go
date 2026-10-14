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
		{"env python", `env python3 -c "open('a.py', 'w').write('x')"`, true},
		{"env with options and assignments", `env -i -u HOME PATH=/usr/bin python3 -c "open('a.py', 'w').write('x')"`, true},
		{"timeout python", `timeout 60 python3 -c "open('a.py', 'w').write('x')"`, true},
		{"timeout with signal", `timeout -s KILL 60 python3 -c "open('a.py', 'w').write('x')"`, true},
		{"sudo env nested wrappers", `sudo -u root env A=1 python3 -c "open('a.py', 'w').write('x')"`, true},
		{"xargs sed", `find . -name '*.py' | xargs sed -i 's/a/b/'`, true},
		{"xargs -I sed", `ls *.py | xargs -I {} sed -i 's/a/b/' {}`, true},
		{"find -exec sed plus", `find . -name '*.py' -exec sed -i 's/a/b/' {} +`, true},
		{"find -exec sed semicolon", `find . -name '*.py' -exec sed -i 's/a/b/' {} \;`, true},
		{"cp in workspace", "cp a.py b.py", true},
		{"mv in workspace", "mv a.py b.py", true},
		{"mv out of the workspace", "mv a.py /tmp/a.py", true},
		{"rm in workspace", "rm -rf build", true},
		{"rm expanded operand", `rm "$f"`, true},
		{"touch", "touch a.py", true},
		{"truncate", "truncate -s 0 app.log", true},
		{"ln", "ln -s a.py b.py", true},
		{"patch from stdin", "patch -p1 < fix.diff", true},
		{"git apply", "git apply fix.diff", true},
		{"git apply of a patch under /tmp", "git apply /tmp/fix.diff", true},
		{"git apply --stat --apply", "git apply --stat --apply fix.diff", true},
		{"find -delete", "find . -name '*.orig' -delete", true},
		{"ruby FileUtils.rm_rf", `ruby -e "require 'fileutils'; FileUtils.rm_rf('build')"`, true},
		{"xargs rm", "find . -name '*.pyc' | xargs rm -f", true},
		{"xargs -a rm", "xargs -a list.txt rm", true},
		{"xargs -I sh -c sed", `find . | xargs -I{} sh -c 'sed -i "s/a/b/" {}'`, true},
		{"xargs -I bash -c redirect", `ls | xargs -I{} bash -c 'echo x > {}'`, true},
		{"bash -c with escaped single quotes", `bash -c 'bash -c '\''sed -i s/a/b/ a.py'\'''`, true},
		{"bash -c two deep with escaped single quotes", `bash -c 'bash -c '\''sed -i '\''\'\'''\''s/a/b/'\''\'\'''\'' a.py'\'''`, true},
		{"cp through /tmp/..", "cp a.py /tmp/../work/workspace/b.py", true},
		{"rm through /tmp/..", "rm /tmp/../work/workspace/a.py", true},
		{"redirect through /tmp/..", "echo x > /tmp/../work/workspace/a.py", true},
		{"env -S sed", `env -S "sed -i s/a/b/" a.py`, true},
		{"env --split-string python", `env --split-string="python3 -c 'open(1, \"w\").close()'"`, true},
		{"env - python", `env - python3 -c "open('a.py', 'w').write('x')"`, true},
		{"xargs attached -i sh -c sed", `ls | xargs -i% sh -c 'sed -i s/a/b/ %'`, true},
		{"five nested wrappers", `sudo env A=1 timeout 60 nice -n 5 stdbuf -oL python3 -c "open('a.py', 'w').write('x')"`, true},
		{"cp backup to /tmp", "cp a.py /tmp/a.py.bak", false},
		{"cp -t /tmp", "cp -t /tmp a.py b.py", false},
		{"mv within /tmp", "mv /tmp/a /tmp/b", false},
		{"rm under /tmp", "rm -f /tmp/out.txt /dev/shm/x", false},
		{"find -delete under /tmp", "find /tmp/build -name '*.o' -delete", false},
		{"patch --dry-run", "patch --dry-run -p1 < fix.diff", false},
		{"patch into /tmp", "patch -d /tmp/copy -p1 < fix.diff", false},
		{"git apply --check", "git apply --check fix.diff", false},
		{"git apply --stat", "git apply --stat fix.diff", false},
		{"xargs -I cp to /tmp", "find . -name '*.py' | xargs -I {} cp {} /tmp/backup/", false},
		{"xargs -I cp into a /tmp path built from the input", "ls | xargs -I{} cp {} /tmp/backup/{}", false},
		{"xargs -I cp to /tmp/{}", "ls | xargs -I{} cp {} /tmp/{}", false},
		{"xargs -I rm under /tmp", "ls | xargs -I {} rm /tmp/{}", false},
		{"xargs attached -i cp into /tmp", "ls | xargs -i% cp % /tmp/backup/%", false},
		{"xargs --replace= cp into /tmp", "ls | xargs --replace=% cp % /tmp/backup/%", false},
		{"xargs cp -t /tmp", "ls *.py | xargs cp -t /tmp", false},
		{"cp to a cleaned /tmp path", "cp a.py /tmp/x/../a.py", false},
		{"env -S read only", `env -S "python3 -c 'print(1)'"`, false},
		{"ruby FileUtils.pwd", `ruby -e "require 'fileutils'; puts FileUtils.pwd"`, false},
		{"env python reads", `env python3 -c "print(open('a.py').read())"`, false},
		{"timeout tests", "timeout 120 python3 -m unittest", false},
		{"command -v", "command -v python3", false},
		{"xargs grep", "find . -name '*.py' | xargs grep -n x", false},
		{"find -exec grep", "find . -name '*.py' -exec grep -n x {} +", false},
		{"python reads a file", `python3 -c "print(open('a.py').read())"`, false},
		{"python reads with an explicit mode", `python3 -c "print(open('a.py', 'rb').read())"`, false},
		{"python writes to stdout", `python3 -c "import sys; sys.stdout.write('done')"`, false},
		{"python mentions rewrite", `python3 -c "print(rewrite_rules('a.py'))"`, false},
		{"python csv writer to stdout", `python3 -c "import csv, sys; csv.writer(sys.stdout).writerow(['a'])"`, false},
		{"python prints the word write", `python3 -c "print('write the file')"`, false},
		{"ruby puts", `ruby -e "puts File.read('a.rb')"`, false},
		{"bash -c read only", `bash -c 'grep -n x a.py'`, false},
		{"xargs -I sh -c read only", `ls | xargs -I{} sh -c 'grep -n x {}'`, false},
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

// no_shell_file_io follows the prompt's routing of file reads and edits
// to the file tools. In-place editors run through a wrapper or find are
// still shell edits; cp, mv, and rm are not file reads or edits the
// prompt routes to those tools.
func TestShellFileIODetection(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		command string
		finding bool
	}{
		{"xargs sed", `find . -name '*.py' | xargs sed -i 's/a/b/'`, true},
		{"find -exec sed", `find . -name '*.py' -exec sed -i 's/a/b/' {} +`, true},
		{"env cat", "env LC_ALL=C cat a.py", true},
		{"xargs cat", "find . -name '*.py' | xargs cat", true},
		{"cat expanded operand", `cat "$f"`, true},
		{"head -n file", "head -n 5 a.py", true},
		{"tail -f -n file", "tail -f -n 20 app.log", true},
		{"redirect through /tmp/..", "echo x > /tmp/../work/a.py", true},
		{"env -S sed", `env -S "sed -i s/a/b/" a.py`, true},
		{"env - cat", "env - cat a.py", true},
		{"piped head -n", "git log | head -n 5", false},
		{"piped tail -n", "rg x | tail -n 20", false},
		{"piped tail --lines", "rg x | tail --lines 20", false},
		{"timeout piped head -n", "timeout 30 go test ./... 2>&1 | head -n 3", false},
		{"cat stdin", "echo x | cat -", false},
		{"redirect to /tmp", "echo x > /tmp/out.txt", false},
		{"cp", "cp a.py b.py", false},
		{"mv", "mv a.py b.py", false},
		{"rm", "rm a.py", false},
		{"xargs grep", "find . -name '*.py' | xargs grep -n x", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			script, ok := parseShell(tc.command)
			require.True(t, ok)
			require.Equal(t, tc.finding, len(fileIOFindings(script)) > 0, "findings: %v", fileIOFindings(script))
		})
	}
}
