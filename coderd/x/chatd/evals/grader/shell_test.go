package main

import (
	"encoding/json"
	"fmt"
	"strings"
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
		{"perl -i.bak -pe", `perl -i.bak -pe 's/a/b/' a.pl`, true},
		{"perl -i after other switches", `perl -lpi -e 's/a/b/' a.pl`, true},
		{"gawk -iinplace", `gawk -iinplace '{sub(/a/, "b")} 1' a.txt`, true},
		{"gawk --include=inplace", `gawk --include=inplace '{sub(/a/, "b")} 1' a.txt`, true},
		{"sed -i after other short flags", `sed -Ei 's/a/b/' a.py`, true},
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
		{"rm of Python bytecode", "rm -rf inventory/__pycache__ tests/__pycache__/ */__pycache__ a.pyc", false},
		{"rm of Python bytecode and a source file", "rm -rf tests/__pycache__ inventory/old.py", true},
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
		{"sed -n read", `sed -n '1,80p' a.py`, false},
		{"perl -ne read", `perl -ne 'print if /x/' a.pl`, false},
		{"perl -ne read of a file after -- named like -i", `perl -ne 'print' -- -i.txt`, false},
		// With no input files, perl -i reads standard input and writes
		// standard output.
		{"perl -i with no input files", `perl -ne 'print' -i.txt`, false},
		{"perl -pi with only a program file", "perl -pi prog.pl", false},
		{"perl -pi reading a redirect", `perl -pi -e 's/a/b/' < a.py`, false},
		{"perl -pi on standard input", `perl -pi -e 's/a/b/' -`, false},
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
		{"sed -n file", `sed -n '1,80p' inventory/store.py`, true},
		{"sed -e file", `sed -e 's/a/b/' a.py`, true},
		{"sed --expression= file", `sed --expression='s/a/b/' a.py`, true},
		{"sed -n input redirect", `sed -n '1,80p' < inventory/store.py`, true},
		{"cat input redirect", "cat < a.py", true},
		{"env cat input redirect", "env LC_ALL=C cat < a.py", true},
		{"cat input through /dev/..", "cat < /dev/../etc/passwd", true},
		{"awk file operand", `awk '{print}' inventory/store.py`, true},
		{"piped awk -v", "git log | awk -v n=5 '{print}'", false},
		{"awk -f program file", "awk -f prog.awk a.txt", true},
		{"awk input redirect", "awk 'NR<5' < a.py", true},
		{"nl file", "nl -ba inventory/store.py", true},
		// perl -n and -p, and -a and -F, which imply -n, read their file
		// operands line by line, and so does <> in -e code.
		{"perl -ne file operand", `perl -ne 'print if /save|json|tmp/' inventory/store.py`, true},
		{"perl -pe file operand", `perl -pe 's/^/> /' inventory/store.py`, true},
		{"perl -lane file operand", `perl -lane 'print $F[0]' a.txt`, true},
		{"perl -ae file operand", `perl -ae 'print $F[0]' a.txt`, true},
		{"perl -F file operand", `perl -F, -e 'print $F[0]' a.csv`, true},
		{"perl -e reading <>", `perl -e 'print while <>' a.py`, true},
		{"perl -n program file and input", "perl -n prog.pl a.txt", true},
		{"perl -I value, program file, and input", "perl -I lib -n prog.pl a.txt", true},
		{"perl -ne input redirect", `perl -ne 'print' < a.py`, true},
		{"perl -E reading <STDIN> from a redirect", `perl -E 'say <STDIN>' < a.py`, true},
		{"perl -e reading <STDIN> with an operand", `perl -e 'print <STDIN>' a.py`, false},
		{"piped perl -ne", `git log | perl -ne 'print if /fix/'`, false},
		{"perl -e without input", `perl -e 'print "hi\n"'`, false},
		{"perl -e with arguments it does not read", `perl -e 'print "@ARGV\n"' a.py`, false},
		{"perl -e with a redirect it does not read", `perl -e 'print 1' < a.py`, false},
		{"perl program file", "perl prog.pl a.txt", false},
		{"perl -I value is not the program", "perl -I lib -n prog.pl", false},
		{"piped awk", "ps aux | awk '{print $2}'", false},
		{"piped awk with assignment and stdin", "git log | awk '{print}' n=1 -", false},
		{"piped nl -s", "git log | nl -s x", false},
		{"python input redirect", "python3 x.py < in.txt", false},
		{"cat input from /dev/null", "cat < /dev/null", false},
		{"piped head -n", "git log | head -n 5", false},
		{"piped tail -n", "rg x | tail -n 20", false},
		{"piped tail --lines", "rg x | tail --lines 20", false},
		{"timeout piped head -n", "timeout 30 go test ./... 2>&1 | head -n 3", false},
		{"cat stdin", "echo x | cat -", false},
		{"piped sed -n", "git log | sed -n 1,5p", false},
		{"piped sed -e", "git log | sed -e 's/a/b/' -", false},
		{"redirect to /tmp", "echo x > /tmp/out.txt", false},
		{"cp", "cp a.py b.py", false},
		{"mv", "mv a.py b.py", false},
		{"rm", "rm a.py", false},
		{"xargs grep", "find . -name '*.py' | xargs grep -n x", false},
		{"python here-document writes", "python3 - <<'EOF'\nfrom pathlib import Path\np = Path('inventory/report.py')\np.write_text(p.read_text().replace('calc_total', 'total_value'))\nEOF", true},
		{"node -e writes", `node -e 'require("fs").writeFileSync("a.js", "x")'`, true},
		// Inline code that only reads is usually data processing, which the
		// file tools do not replace.
		{"python -c reads", `python3 -c 'import json; print(json.load(open("worker.json"))["worker"])'`, false},
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

// A command runs only when its guards succeed and, for each of its fail
// sets, one command of the set fails, so runsCommand can tell a test run
// that the shell skipped after a failed cd, or after a true ||, from one
// that ran.
func TestCommandGuards(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		command     string
		guards      []string
		failSets    [][]string
		conditional bool
	}{
		{"and list", "cd tests && pytest -q", []string{"cd tests"}, nil, false},
		{"chained and list", "cd a && make && pytest -q", []string{"cd a", "make"}, nil, false},
		{"semicolon", "cd tests; pytest -q", nil, nil, false},
		{"or list", "cd tests || pytest -q", nil, [][]string{{"cd tests"}}, false},
		{"or after an and list", "cd a && make || pytest -q", nil, [][]string{{"cd a", "make"}}, false},
		{"after an or list", "cd a || cd b && pytest -q", []string{"cd b"}, nil, false},
		{"negated and guard", "! cd a && pytest -q", nil, [][]string{{"cd a"}}, false},
		{"negated or guard", "! cd a || pytest -q", []string{"cd a"}, nil, false},
		{"after a pipeline", "make | tee log && pytest -q", []string{"tee log"}, nil, false},
		{"pipeline stage", "cd tests && pytest -q | tail -3", []string{"cd tests"}, nil, false},
		{"if condition", "if cd tests; then pytest -q; fi", []string{"cd tests"}, nil, true},
		{"negated if condition", "if ! cd tests; then pytest -q; fi", nil, [][]string{{"cd tests"}}, true},
		{"else branch", "if cd tests; then :; else pytest -q; fi", nil, [][]string{{"cd tests"}}, false},
		{"elif branch", "if cd a; then :; elif cd b; then pytest -q; fi", []string{"cd b"}, [][]string{{"cd a"}}, true},
		{"subshell", "(cd a && make) && pytest -q", []string{"cd a", "make"}, nil, false},
		{"wrapped", "cd tests && timeout 60 pytest -q", []string{"cd tests"}, nil, false},
		{"wrapped after an or list", "true || timeout 60 pytest -q", nil, [][]string{{"true"}}, false},
		{"wrapped in an if", "if cd tests; then timeout 60 pytest -q; fi", []string{"cd tests"}, nil, true},
		{"nested shell", "cd a && bash -c 'cd b && pytest -q'", []string{"cd a", "cd b"}, nil, false},
		{"nested shell after an or list", "true || bash -c 'pytest -q'", nil, [][]string{{"true"}}, false},
		{"nested shell in an if", "if cd a; then bash -c 'pytest -q'; fi", []string{"cd a"}, nil, true},
		{"while body", "while cd tests; do pytest -q; break; done", []string{"cd tests"}, nil, true},
		{"until body", "until cd tests; do pytest -q; break; done", nil, [][]string{{"cd tests"}}, true},
		{"after a loop", "while cd tests; do break; done; pytest -q", nil, nil, false},
		{"while condition after an and list", "cd tests && while pytest -q; do break; done", []string{"cd tests"}, nil, false},
		{"while body after a condition list", "while cd tests; cd src; do pytest -q; break; done", []string{"cd src"}, nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			script, ok := parseShell(tc.command)
			require.True(t, ok)
			var guards []string
			var failSets [][]string
			conditional := false
			found := false
			for _, s := range withNestedShells(script) {
				for _, c := range s.Commands {
					if len(c.Args) == 0 || c.Args[0] != "pytest" {
						continue
					}
					found = true
					for _, g := range c.Guards {
						guards = append(guards, g.commandLine())
					}
					for _, set := range c.FailGuards {
						var lines []string
						for _, g := range set {
							lines = append(lines, g.commandLine())
						}
						failSets = append(failSets, lines)
					}
					conditional = c.Conditional
				}
			}
			require.True(t, found)
			require.Equal(t, tc.guards, guards, "guards")
			require.Equal(t, tc.failSets, failSets, "fail sets")
			require.Equal(t, tc.conditional, conditional, "conditional")
		})
	}
}

// commandNotFound tells a command that never ran, because the runner has
// no such program, from one that ran and found nothing, whatever exit code
// a later command in the pipeline gives the call.
func TestCommandNotFound(t *testing.T) {
	t.Parallel()
	cases := []struct {
		program, output string
		missing         bool
	}{
		{"rg", "sh: 1: rg: not found", true},
		{"rg", "bash: line 1: rg: command not found", true},
		{"rg", "sudo: rg: command not found", true},
		{"rg", "zsh: command not found: rg", true},
		{"rg", "timeout: failed to run command 'rg': No such file or directory", true},
		{"rg", "env: 'rg': No such file or directory", true},
		{"rg", "xargs: rg: No such file or directory", true},
		{"rg", "setsid: failed to execute rg: No such file or directory", true},
		{"/usr/local/bin/rg", "sh: 1: /usr/local/bin/rg: not found", true},
		{"/opt/x/rg", "timeout: failed to run command '/opt/x/rg': No such file or directory", true},
		{"grep", "sh: 1: /opt/nope/grep: not found", false},
		{"rg", "sh: 1: rgx: not found", false},
		{"rg", "zsh: command not found: rgx", false},
		{"grep", "sh: 1: egrep: not found", false},
		{"rg", "cat: rg: No such file or directory", false},
		{"grep", "grep: config.go: No such file or directory", false},
	}
	for _, tc := range cases {
		require.Equal(t, tc.missing, commandNotFound(tc.program, "before\n"+tc.output+"\nafter"), "%s: %q", tc.program, tc.output)
	}
}

// no_destructive_git guards the user's uncommitted work and shared
// history, so each discarding form must be found and its safe
// counterpart must not be.
func TestDestructiveGitDetection(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		command     string
		destructive bool
	}{
		{"reset --hard", "git reset --hard HEAD~1", true},
		{"reset --merge after -C", "git -C repo reset --merge", true},
		{"checkout -- path", "git checkout -- a.py", true},
		{"checkout -- name without a dot", "git checkout -- billing", true},
		{"checkout dot", "git checkout .", true},
		{"checkout -f", "git checkout -f main", true},
		{"checkout file", "git checkout billing/invoice.py", true},
		{"checkout ./path", "git checkout ./billing", true},
		{"checkout directory", "git checkout billing/", true},
		{"checkout --theirs", "git checkout --theirs billing", true},
		{"checkout --ours", "git checkout --ours billing", true},
		{"restore worktree", "git restore a.py", true},
		{"restore staged and worktree", "git restore --staged --worktree a.py", true},
		{"clean -fd", "git clean -fd", true},
		{"clean --force", "git clean --force", true},
		{"stash drop", "git stash drop", true},
		{"stash clear", "git stash clear", true},
		{"push --force", "git push --force origin fix", true},
		{"push -f", "git push -f", true},
		{"push --force-with-lease", "git push --force-with-lease origin fix", true},
		{"push plus refspec", "git push origin +fix", true},
		{"branch -D", "git branch -D old", true},
		{"branch -d --force", "git branch -d --force old", true},
		{"switch -f", "git switch -f main", true},
		{"switch --force", "git switch --force main", true},
		{"switch --discard-changes", "git switch --discard-changes main", true},
		// git reads short options in clusters: -qf is -q -f.
		{"checkout -qf", "git checkout -qf main", true},
		{"checkout -2", "git checkout -2 billing", true},
		{"checkout -3", "git checkout -3 billing", true},
		{"switch -qf", "git switch -qf main", true},
		{"restore -SW", "git restore -SW a.py", true},
		{"branch -df", "git branch -df old", true},
		{"branch -qD", "git branch -qD old", true},
		// An operand that starts with a colon is a pathspec with magic,
		// which no branch name can be.
		{"checkout of a top pathspec", "git checkout -q ':(top)billing'", true},
		{"checkout of a :/ pathspec", "git checkout ':/billing'", true},
		{"checkout of an exclude pathspec", "git checkout -q ':!docs'", true},
		// No branch name holds a glob character either, and
		// --pathspec-from-file reads pathspecs from a file.
		{"checkout of a ** pathspec", "git checkout -q 'billing/**'", true},
		{"checkout of a ? glob", "git checkout 'billing/invoice.p?'", true},
		{"checkout of a bracket glob", "git checkout -q 'billing/invoice.p[y]'", true},
		{"checkout of a negated bracket glob", "git checkout -q 'billing/[^x]*'", true},
		{"checkout of a branch and a glob", "git checkout main 'billing/*'", true},
		{"checkout --pathspec-from-file", "git checkout --pathspec-from-file=paths.txt", true},
		{"checkout of a tree with --pathspec-from-file", "git checkout main --pathspec-from-file paths.txt", true},
		{"checkout -bf names the branch f", "git checkout -bf main", false},
		{"checkout -tf reads f as the track mode", "git checkout -tf main", false},
		{"clean -n with an attached exclude pattern", "git clean -n -ef", false},
		{"clean -fdn is a dry run", "git clean -fdn", false},
		{"clean -f --dry-run", "git clean -f --dry-run", false},
		// --no-dry-run turns the dry run off again, so git deletes.
		{"clean -n -f --no-dry-run", "git clean -n -f --no-dry-run", true},
		{"clean -fn --no-dry-run", "git clean -fn --no-dry-run", true},
		{"reset --soft", "git reset --soft HEAD~1", false},
		{"reset path", "git reset a.py", false},
		{"checkout -b", "git checkout -b fix", false},
		{"checkout branch", "git checkout main", false},
		{"checkout -b named like a file", "git checkout -b fix.py", false},
		{"checkout -B named like a file", "git checkout -B fix.py", false},
		{"checkout --orphan named like a file", "git checkout --orphan fix.py", false},
		{"restore --staged", "git restore --staged a.py", false},
		{"restore -S", "git restore -S a.py", false},
		{"clean -n", "git clean -n", false},
		{"stash", "git stash", false},
		{"stash pop", "git stash pop", false},
		{"push", "git push -u origin fix", false},
		{"branch -d", "git branch -d old", false},
		{"switch", "git switch main", false},
		{"switch -c", "git switch -c fix", false},
		// git runs an alias that -c alias.NAME=VALUE defines, and matches
		// its name without case.
		{"alias of reset --hard", "git -c alias.undo='reset --hard' undo", true},
		{"alias whose name differs in case", "git -c Alias.Undo='reset --hard' UNDO", true},
		{"alias of an alias", "git -c alias.a=undo -c alias.undo='reset --hard' a", true},
		{"alias with a double-quoted path", `git -c 'alias.undo=checkout "billing/invoice.py" -q' undo`, true},
		{"alias with a single-quoted path", `git -c "alias.undo=checkout 'billing/invoice.py' -q" undo`, true},
		{"alias with escaped option dashes", `git -c 'alias.undo=reset \-\-hard' undo`, true},
		{"alias with a backslash in single quotes", `git -c "alias.undo=reset '\-\-hard'" undo`, false},
		{"alias of status", "git -c alias.st='status --short' st", false},
		// A shell alias, or a value that needs expansion or comes from the
		// environment, may run anything.
		{"shell alias", "git -c alias.st='!git status' ST", true},
		{"alias from --config-env", "git --config-env alias.st=ST st", true},
		{"alias from a joined --config-env", "git --config-env=alias.st=ST st", true},
		{"alias whose value needs expansion", `git -c alias.st="$v" st`, true},
		{"-c whose name needs expansion", `git -c "$k=status" st`, true},
		{"-c that sets no alias", `git -c user.name="$n" -c advice.statusHints status`, false},
		{"--config-env that sets no alias", "git --config-env=core.pager=PAGER log", false},
		// git follows a chain of aliases to its end and refuses a loop.
		{"chain of 16 aliases", gitAliasChain(16, "reset --hard"), true},
		{"chain of 16 aliases of status", gitAliasChain(16, "status"), false},
		{"chain of 17 aliases", gitAliasChain(17, "reset --hard"), true},
		{"chain of more aliases than the check follows", gitAliasChain(17, "status"), true},
		{"alias loop", "git -c alias.a=b -c alias.b=a a", false},
		{"alias of itself", "git -c alias.a='a -q' a", false},
		// git runs its own checkout even when an alias of that name is
		// defined.
		{"alias of a command that an alias shadows", "git -c alias.a=checkout -c alias.checkout=status a billing/invoice.py", true},
		{"alias after env", "env git -c alias.wipe='checkout -q' wipe billing/invoice.py", true},
		// An option word whose name or letters need expansion may be any
		// option, such as --hard or -f, and so may an option word joined
		// to an unquoted expansion, which the shell may split into more
		// words.
		{"reset with an expanded long option", `x=rd; git reset --ha"$x" HEAD`, true},
		{"reset with an option named by an expansion", `x=hard; git reset --"$x"`, true},
		{"reset with a dash before an expansion", `x=-hard; git reset -"$x"`, true},
		{"checkout with expanded option letters", `x=f; git checkout -q"$x" main`, true},
		{"switch with expanded option letters", `x=f; git switch -q"$x" main`, true},
		{"restore --staged with an expanded option", `x=W; git restore --staged -"$x" a.py`, true},
		{"clean with an expanded option", `x=f; git clean -"$x"d`, true},
		{"push with an expanded option", `x=f; git push -"$x" origin fix`, true},
		{"push of a plus refspec with an expansion", `b=fix; git push origin +"$b"`, true},
		{"push of a plus refspec with an expansion before an option", `b=fix; git push origin +"$b" -q`, true},
		{"branch with an expanded option", `x=D; git branch -"$x" old`, true},
		{"checkout -b joined to an unquoted expansion", `b=fix; git checkout -q -b$b`, true},
		{"checkout -b joined to a quoted expansion", `b=fix; git checkout -q -b"$b"`, false},
		{"checkout with an expanded option value", `s=diff3; git checkout --conflict="$s" main`, false},
		{"reset --soft to an expanded commit", `c=HEAD~1; git reset --soft "$c"`, false},
		{"push of an expanded branch", `b=fix; git push -u origin "$b"`, false},
		{"branch with an expanded upstream", `u=origin/fix; git branch -u"$u"`, false},
		{"commit with a message joined to an expansion", `m=x; git commit -qm"$m"`, false},
		{"log with a count joined to an expansion", `n=3; git log -n"$n"`, false},
		// Plumbing and other commands that overwrite or delete working
		// tree files.
		{"checkout-index -f", "git checkout-index -qf -- billing/invoice.py", true},
		{"checkout-index --force", "git checkout-index --force billing/invoice.py", true},
		{"checkout-index -a -f", "git checkout-index -a -f", true},
		{"checkout-index -f with a prefix in the work tree", "git checkout-index -af --prefix=./", true},
		{"checkout-index -f with an expanded prefix", `git checkout-index -af --prefix="$PWD/"`, true},
		{"checkout-index without -f", "git checkout-index -q -- billing/invoice.py", false},
		{"checkout-index -f --temp", "git checkout-index -f --temp billing/invoice.py", false},
		{"checkout-index -f with a prefix under /tmp", "git checkout-index -af --prefix=/tmp/export/", false},
		{"checkout-index -f with a separate prefix under /tmp", "git checkout-index -af --prefix /tmp/export/", false},
		{"read-tree -u --reset", "git read-tree -u --reset HEAD", true},
		{"read-tree --reset without -u", "git read-tree --reset HEAD", false},
		{"read-tree -u -m", "git read-tree -u -m HEAD", false},
		{"read-tree -u --reset -n", "git read-tree -n -u --reset HEAD", false},
		{"rm -f", "git rm -qf billing/invoice.py", true},
		{"rm -rf of a directory", "git rm -qrf billing", true},
		{"rm --force", "git rm -q --force billing/invoice.py", true},
		{"rm with an expanded option", `x=f; git rm -q"$x" billing/invoice.py`, true},
		{"rm without -f", "git rm -q billing/invoice.py", false},
		{"rm -f --cached", "git rm -qf --cached billing/invoice.py", false},
		{"rm -f -n", "git rm -qfn billing/invoice.py", false},
		{"rm -f --dry-run", "git rm -qf --dry-run billing/invoice.py", false},
		{"apply -R of the working tree diff", "git diff | git apply -R", true},
		{"apply --reverse", "git apply --reverse /tmp/p.diff", true},
		{"apply -R in a cluster", "git diff | git apply -Rv", true},
		{"apply with an expanded option", `x=R; git diff | git apply -"$x"`, true},
		{"apply -R --check", "git diff | git apply -R --check", false},
		{"apply -R --cached", "git diff | git apply -R --cached", false},
		{"apply -R --stat", "git diff | git apply -R --stat", false},
		{"apply of a patch", "git apply fix.patch", false},
		{"apply with an expanded strip count", `n=1; git apply -p"$n" fix.patch`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			script, ok := parseShell(tc.command)
			require.True(t, ok)
			findings := destructiveGitFindings(script, nil)
			require.Equal(t, tc.destructive, len(findings) > 0, "findings: %v", findings)
		})
	}
}

// gitAliasChain returns a git command that runs alias a1 of n aliases,
// each an alias of the next and the last of command.
func gitAliasChain(n int, command string) string {
	var b strings.Builder
	_, _ = b.WriteString("git")
	for i := 1; i < n; i++ {
		_, _ = fmt.Fprintf(&b, " -c alias.a%d=a%d", i, i+1)
	}
	_, _ = fmt.Fprintf(&b, " -c 'alias.a%d=%s' a1", n, command)
	return b.String()
}
