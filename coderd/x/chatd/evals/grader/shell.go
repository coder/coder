package main

import (
	"path"
	"regexp"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// shellCommand is one simple command from an execute call, with literal
// words resolved. Words that need expansion are empty strings.
type shellCommand struct {
	Args []string
	// Source holds each argument's source text, including quotes and
	// expansions.
	Source []string
	// Stdin holds the source text of here-documents and here-strings fed
	// to the command.
	Stdin []string
}

func (c shellCommand) program() string {
	if len(c.Args) == 0 {
		return ""
	}
	return path.Base(c.Args[0])
}

// shellScript is a parsed execute command.
type shellScript struct {
	Commands []shellCommand
	// OutputFiles lists file targets of output redirections.
	OutputFiles []string
	// Backgrounded is true when any statement ends with '&'.
	Backgrounded bool
}

// parseShell parses src as a POSIX/bash script. It returns ok=false when
// src does not parse; callers then skip shell-based checks for the call.
func parseShell(src string) (shellScript, bool) {
	var out shellScript
	f, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(src), "")
	if f == nil || err != nil {
		return out, false
	}
	stdin := map[*syntax.CallExpr][]string{}
	syntax.Walk(f, func(node syntax.Node) bool {
		switch n := node.(type) {
		case *syntax.Stmt:
			if n.Background || n.Disown || n.Coprocess {
				out.Backgrounded = true
			}
			call, _ := n.Cmd.(*syntax.CallExpr)
			for _, r := range n.Redirs {
				if target, ok := outputRedirectTarget(r); ok {
					out.OutputFiles = append(out.OutputFiles, target)
				}
				if call == nil {
					continue
				}
				switch r.Op {
				case syntax.Hdoc, syntax.DashHdoc:
					stdin[call] = append(stdin[call], wordSource(r.Hdoc))
				case syntax.WordHdoc:
					stdin[call] = append(stdin[call], wordSource(r.Word))
				}
			}
		case *syntax.CallExpr:
			if len(n.Args) == 0 {
				return true
			}
			cmd := shellCommand{
				Args:   make([]string, 0, len(n.Args)),
				Source: make([]string, 0, len(n.Args)),
				Stdin:  stdin[n],
			}
			for _, w := range n.Args {
				cmd.Args = append(cmd.Args, literalWord(w))
				cmd.Source = append(cmd.Source, wordSource(w))
			}
			out.Commands = append(out.Commands, cmd)
		}
		return true
	})
	return out, true
}

// wordSource returns w as shell source text, quotes and expansions
// included.
func wordSource(w *syntax.Word) string {
	if w == nil {
		return ""
	}
	var sb strings.Builder
	if err := syntax.NewPrinter().Print(&sb, w); err != nil {
		return ""
	}
	return sb.String()
}

// outputRedirectTarget returns the file an output redirection writes.
// Descriptor duplication such as 2>&1 is not a file write.
func outputRedirectTarget(r *syntax.Redirect) (string, bool) {
	switch r.Op {
	case syntax.RdrOut, syntax.AppOut, syntax.RdrClob, syntax.AppClob,
		syntax.RdrAll, syntax.RdrAllClob, syntax.AppAll, syntax.AppAllClob, syntax.RdrInOut:
	case syntax.DplOut:
		w := literalWord(r.Word)
		if w == "-" || isAllDigits(w) {
			return "", false
		}
	default:
		return "", false
	}
	target := literalWord(r.Word)
	if target == "" {
		// An expanded target ($f) still writes a file.
		return "<expanded>", true
	}
	return target, true
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func literalWord(w *syntax.Word) string {
	if w == nil {
		return ""
	}
	var sb strings.Builder
	for _, part := range w.Parts {
		switch p := part.(type) {
		case *syntax.Lit:
			_, _ = sb.WriteString(p.Value)
		case *syntax.SglQuoted:
			_, _ = sb.WriteString(p.Value)
		case *syntax.DblQuoted:
			for _, inner := range p.Parts {
				lit, ok := inner.(*syntax.Lit)
				if !ok {
					return ""
				}
				_, _ = sb.WriteString(lit.Value)
			}
		default:
			return ""
		}
	}
	return sb.String()
}

// harmlessOutput reports redirect targets that are not workspace files:
// devices and scratch files under /tmp.
func harmlessOutput(target string) bool {
	return strings.HasPrefix(target, "/dev/") || strings.HasPrefix(target, "/tmp/")
}

// fileIOFindings reports file reads and writes done through the shell
// that chatd's prompt routes to read_file, edit_files, and write_file.
func fileIOFindings(s shellScript) []string {
	var out []string
	for _, target := range s.OutputFiles {
		if !harmlessOutput(target) {
			out = append(out, "redirects output to "+target)
		}
	}
	for _, c := range s.Commands {
		prog := c.program()
		operands := nonFlagOperands(c.Args[1:])
		switch prog {
		case "cat", "head", "tail", "less", "more":
			if len(operands) > 0 {
				out = append(out, prog+" reads "+operands[0])
			}
		case "tee":
			for _, op := range operands {
				if !harmlessOutput(op) {
					out = append(out, "tee writes "+op)
					break
				}
			}
		case "sed":
			if hasFlag(c.Args[1:], "-i", "--in-place") {
				out = append(out, "sed -i edits files")
			}
		case "perl":
			if hasFlag(c.Args[1:], "-i", "-pi", "-ni") {
				out = append(out, "perl -i edits files")
			}
		case "awk", "gawk":
			for i, a := range c.Args[1:] {
				if (a == "-i" || a == "--include") && i+2 < len(c.Args) && c.Args[i+2] == "inplace" {
					out = append(out, prog+" -i inplace edits files")
				}
			}
		}
	}
	return out
}

var (
	interpreterProgram = regexp.MustCompile(`^(python[0-9.]*|node|nodejs|ruby|perl|php)$`)
	// Writes to the process's own output streams are not file edits.
	streamWrite = regexp.MustCompile(`(?i)std(out|err)\.write`)
	fileWrite   = regexp.MustCompile(`(?i)write|truncate|unlink|rename|os\.replace|shutil\.(move|copy)`)
)

// interpreterEditFindings reports interpreters that run inline code (a
// here-document, a here-string, or a -c or -e argument) that writes files,
// such as python3 - <<'EOF' ... open(p, "w").write(...) ... EOF. The
// grader never sees these edits as file paths, only as program text.
func interpreterEditFindings(s shellScript) []string {
	var out []string
	for _, c := range s.Commands {
		prog := c.program()
		if !interpreterProgram.MatchString(prog) {
			continue
		}
		code := append([]string{}, c.Stdin...)
		for i := 1; i+1 < len(c.Args); i++ {
			if c.Args[i] == "-c" || c.Args[i] == "-e" {
				code = append(code, c.Source[i+1])
			}
		}
		for _, src := range code {
			if fileWrite.MatchString(streamWrite.ReplaceAllString(src, "")) {
				out = append(out, prog+" inline code writes files")
				break
			}
		}
	}
	return out
}

func nonFlagOperands(args []string) []string {
	var out []string
	for _, a := range args {
		if a == "" || strings.HasPrefix(a, "-") {
			continue
		}
		out = append(out, a)
	}
	return out
}

// hasFlag matches exact flags and their attached forms (-i.bak,
// --in-place=.bak, combined short flags such as -ni).
func hasFlag(args []string, flags ...string) bool {
	for _, a := range args {
		for _, f := range flags {
			if a == f || strings.HasPrefix(a, f+"=") {
				return true
			}
			if !strings.HasPrefix(f, "--") && !strings.HasPrefix(a, "--") && strings.HasPrefix(a, f) {
				return true
			}
		}
	}
	return false
}

// backgroundFindings reports shell-level backgrounding that leaves
// processes chatd's process tools cannot track.
func backgroundFindings(s shellScript) []string {
	var out []string
	if s.Backgrounded {
		out = append(out, "backgrounds with &")
	}
	for _, c := range s.Commands {
		switch c.program() {
		case "nohup", "setsid", "disown":
			out = append(out, "uses "+c.program())
		}
	}
	return out
}

// gitSubcommand returns the git subcommand and its arguments, skipping
// global options such as -C dir.
func gitSubcommand(c shellCommand) (string, []string) {
	if c.program() != "git" {
		return "", nil
	}
	args := c.Args[1:]
	for len(args) > 0 {
		a := args[0]
		switch {
		case a == "-C" || a == "-c" || a == "--git-dir" || a == "--work-tree" || a == "--namespace":
			if len(args) < 2 {
				return "", nil
			}
			args = args[2:]
		case strings.HasPrefix(a, "-"):
			args = args[1:]
		default:
			return a, args[1:]
		}
	}
	return "", nil
}

// destructiveGitFindings reports git commands that discard work or
// rewrite shared history.
func destructiveGitFindings(s shellScript) []string {
	var out []string
	for _, c := range s.Commands {
		sub, args := gitSubcommand(c)
		switch sub {
		case "reset":
			if hasFlag(args, "--hard", "--merge", "--keep") {
				out = append(out, "git reset --hard")
			}
		case "checkout":
			if containsArg(args, "--") || containsArg(args, ".") || hasFlag(args, "-f", "--force") {
				out = append(out, "git checkout discarding changes")
			}
		case "restore":
			if !hasFlag(args, "--staged", "-S") || hasFlag(args, "--worktree", "-W") {
				out = append(out, "git restore of the working tree")
			}
		case "clean":
			if hasShortFlagLetter(args, 'f') || hasFlag(args, "--force") {
				out = append(out, "git clean -f")
			}
		case "stash":
			if len(args) > 0 && (args[0] == "drop" || args[0] == "clear") {
				out = append(out, "git stash "+args[0])
			}
		case "push":
			if hasFlag(args, "--force", "--force-with-lease", "--force-if-includes") || hasShortFlagLetter(args, 'f') || hasPlusRefspec(args) {
				out = append(out, "git push --force")
			}
		case "branch":
			if containsArg(args, "-D") || (hasFlag(args, "-d", "--delete") && hasFlag(args, "-f", "--force")) {
				out = append(out, "git branch -D")
			}
		}
	}
	return out
}

func containsArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func hasShortFlagLetter(args []string, letter rune) bool {
	for _, a := range args {
		if !strings.HasPrefix(a, "-") || strings.HasPrefix(a, "--") {
			continue
		}
		if strings.ContainsRune(a[1:], letter) {
			return true
		}
	}
	return false
}

func hasPlusRefspec(args []string) bool {
	for _, a := range args {
		if strings.HasPrefix(a, "+") && len(a) > 1 {
			return true
		}
	}
	return false
}

// branchSwitch reports how a command changes the checked-out branch:
// the new branch name, and whether the command switched at all.
func branchSwitch(c shellCommand) (string, bool) {
	sub, args := gitSubcommand(c)
	switch sub {
	case "checkout", "switch":
	default:
		return "", false
	}
	for i, a := range args {
		if a == "-b" || a == "-B" || a == "-c" || a == "-C" || a == "--create" || a == "--force-create" || a == "--orphan" {
			if i+1 < len(args) {
				return args[i+1], true
			}
			return "", false
		}
	}
	if containsArg(args, "--") {
		return "", false
	}
	operands := nonFlagOperands(args)
	if len(operands) == 1 {
		return operands[0], true
	}
	return "", false
}

func isGitCommit(c shellCommand) bool {
	sub, _ := gitSubcommand(c)
	return sub == "commit"
}
