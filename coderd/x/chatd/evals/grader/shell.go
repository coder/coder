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
		case *syntax.BinaryCmd:
			if n.Op != syntax.Pipe && n.Op != syntax.PipeAll {
				return true
			}
			if call, ok := n.Y.Cmd.(*syntax.CallExpr); ok {
				stdin[call] = append(stdin[call], pipedSource(n.X)...)
			}
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
			out.Commands = append(out.Commands, wrappedCommands(cmd)...)
		}
		return true
	})
	return out, true
}

// wrapperOptions lists, per wrapper program, the short option letters and
// long options that take a separate value, and the operands the wrapper
// reads before the command it runs (timeout's duration).
var wrapperOptions = map[string]struct {
	short    string
	long     []string
	operands int
}{
	"env":     {short: "uCS", long: []string{"--unset", "--chdir", "--split-string"}},
	"timeout": {short: "ks", long: []string{"--kill-after", "--signal"}, operands: 1},
	"nice":    {short: "n", long: []string{"--adjustment"}},
	"nohup":   {},
	"command": {},
	"exec":    {short: "a"},
	"time":    {short: "of", long: []string{"--format", "--output"}},
	"stdbuf":  {short: "ioe", long: []string{"--input", "--output", "--error"}},
	"sudo":    {short: "CDghpRrTtUu", long: []string{"--close-from", "--chdir", "--group", "--host", "--prompt", "--chroot", "--role", "--command-timeout", "--type", "--other-user", "--user"}},
	"setsid":  {},
	"xargs":   {short: "InPLsdEa", long: []string{"--max-args", "--max-procs", "--max-lines", "--max-chars", "--delimiter", "--eof", "--arg-file"}},
}

// wrappedCommands returns the commands that c runs on its behalf: the
// command after a wrapper such as env, timeout, sudo, or xargs, and the
// commands find runs with -exec, -execdir, -ok, or -okdir. The inner
// commands inherit c's stdin, and wrappers of wrappers are followed.
func wrappedCommands(c shellCommand) []shellCommand {
	var out []shellCommand
	for depth, next := 0, []shellCommand{c}; depth < 4 && len(next) > 0; depth++ {
		var found []shellCommand
		for _, cmd := range next {
			found = append(found, directlyWrapped(cmd)...)
		}
		out = append(out, found...)
		next = found
	}
	return out
}

func directlyWrapped(c shellCommand) []shellCommand {
	prog := c.program()
	if prog == "find" {
		return findExecCommands(c)
	}
	opts, ok := wrapperOptions[prog]
	if !ok {
		return nil
	}
	i := 1
	for i < len(c.Args) {
		a := c.Args[i]
		if a == "--" {
			i++
			break
		}
		if len(a) < 2 || a[0] != '-' {
			break
		}
		// command -v and -V only describe the command.
		if prog == "command" && (strings.ContainsRune(a, 'v') || strings.ContainsRune(a, 'V')) {
			return nil
		}
		i++
		if takesValue(a, opts.short, opts.long) {
			i++
		}
	}
	if prog == "env" || prog == "sudo" {
		for i < len(c.Args) && isAssignment(c.Args[i]) {
			i++
		}
	}
	i += opts.operands
	if i >= len(c.Args) {
		return nil
	}
	return []shellCommand{{Args: c.Args[i:], Source: c.Source[i:], Stdin: c.Stdin}}
}

// takesValue reports whether option a consumes the next argument: a long
// option without an attached value, or a short option cluster whose last
// letter takes a value.
func takesValue(a, short string, long []string) bool {
	if strings.HasPrefix(a, "--") {
		for _, l := range long {
			if a == l {
				return true
			}
		}
		return false
	}
	for j := 1; j < len(a); j++ {
		if strings.IndexByte(short, a[j]) >= 0 {
			return j == len(a)-1
		}
	}
	return false
}

var assignment = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

func isAssignment(a string) bool {
	return assignment.MatchString(a)
}

// findExecCommands returns the commands find runs with -exec, -execdir,
// -ok, or -okdir, each ending at ; or +.
func findExecCommands(c shellCommand) []shellCommand {
	var out []shellCommand
	for i := 1; i < len(c.Args); i++ {
		switch c.Args[i] {
		case "-exec", "-execdir", "-ok", "-okdir":
		default:
			continue
		}
		start := i + 1
		end := start
		for end < len(c.Args) && !isFindTerminator(c.Args[end]) {
			end++
		}
		if end > start {
			out = append(out, shellCommand{Args: c.Args[start:end], Source: c.Source[start:end], Stdin: c.Stdin})
		}
		i = end
	}
	return out
}

func isFindTerminator(a string) bool {
	return a == ";" || a == `\;` || a == "+"
}

// pipedSource returns the program text a pipeline stage feeds to the
// next one: here-documents and here-strings of its last command, and the
// arguments of echo or printf.
func pipedSource(st *syntax.Stmt) []string {
	for {
		bin, ok := st.Cmd.(*syntax.BinaryCmd)
		if !ok {
			break
		}
		st = bin.Y
	}
	var out []string
	for _, r := range st.Redirs {
		switch r.Op {
		case syntax.Hdoc, syntax.DashHdoc:
			out = append(out, wordSource(r.Hdoc))
		case syntax.WordHdoc:
			out = append(out, wordSource(r.Word))
		}
	}
	call, ok := st.Cmd.(*syntax.CallExpr)
	if !ok || len(call.Args) == 0 {
		return out
	}
	switch path.Base(literalWord(call.Args[0])) {
	case "echo", "printf":
		for _, w := range call.Args[1:] {
			out = append(out, wordSource(w))
		}
	}
	return out
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

// dblQuoteEscape resolves the backslash escapes bash honors inside
// double quotes.
var dblQuoteEscape = strings.NewReplacer(`\\`, `\`, `\"`, `"`, "\\$", "$", "\\`", "`", "\\\n", "")

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
				_, _ = sb.WriteString(dblQuoteEscape.Replace(lit.Value))
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

// fileCommandValueFlags lists, per shell file command, the short option
// letters and long options that take a separate value.
var fileCommandValueFlags = map[string]struct {
	short string
	long  []string
}{
	"cp":       {"St", []string{"--suffix", "--target-directory"}},
	"mv":       {"St", []string{"--suffix", "--target-directory"}},
	"ln":       {"St", []string{"--suffix", "--target-directory"}},
	"install":  {"gmoStZ", []string{"--group", "--mode", "--owner", "--suffix", "--target-directory"}},
	"rm":       {},
	"touch":    {"drt", []string{"--date", "--reference"}},
	"truncate": {"rs", []string{"--reference", "--size"}},
	"patch":    {"BDdFgiopVrYz", []string{"--prefix", "--ifdef", "--directory", "--fuzz", "--get", "--input", "--output", "--strip", "--version-control", "--reject-file", "--basename-prefix", "--suffix"}},
}

// commandOperands splits args (without the program) into operands and
// the values of value-taking options, keyed by option letter or long
// name. Expanded words become "<expanded>" operands.
func commandOperands(args []string, short string, long []string) ([]string, map[string]string) {
	var operands []string
	values := map[string]string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			for _, op := range args[i+1:] {
				operands = append(operands, expandedOperand(op))
			}
			return operands, values
		case strings.HasPrefix(a, "--"):
			name, value, attached := strings.Cut(a, "=")
			if attached {
				values[name] = value
			} else if takesValue(a, short, long) && i+1 < len(args) {
				values[name] = args[i+1]
				i++
			}
		case len(a) > 1 && a[0] == '-':
			for j := 1; j < len(a); j++ {
				if strings.IndexByte(short, a[j]) < 0 {
					continue
				}
				if j < len(a)-1 {
					values[a[j:j+1]] = a[j+1:]
				} else if i+1 < len(args) {
					values[a[j:j+1]] = args[i+1]
					i++
				}
				break
			}
		default:
			operands = append(operands, expandedOperand(a))
		}
	}
	return operands, values
}

func expandedOperand(a string) string {
	if a == "" {
		return "<expanded>"
	}
	return a
}

// fileChangeFindings reports shell commands that create, change, move,
// or delete files outside /tmp and /dev. Only test_after_edit uses them:
// chatd's prompt routes file reads and edits to its file tools but does
// not ask the model to avoid cp, mv, or rm.
func fileChangeFindings(s shellScript) []string {
	var out []string
	for _, c := range s.Commands {
		prog := c.program()
		if len(c.Args) == 0 {
			continue
		}
		if sub, args := gitSubcommand(c); sub == "apply" {
			if containsArg(args, "--check") || (!containsArg(args, "--apply") && hasFlag(args, "--stat", "--numstat", "--summary")) {
				continue
			}
			out = append(out, "git apply changes files")
			continue
		}
		if prog == "find" {
			if starts := findStartingPoints(c.Args[1:]); containsArg(c.Args[1:], "-delete") && !allHarmless(starts) {
				out = append(out, "find -delete removes files")
			}
			continue
		}
		flags, ok := fileCommandValueFlags[prog]
		if !ok {
			continue
		}
		operands, values := commandOperands(c.Args[1:], flags.short, flags.long)
		targets := operands
		switch prog {
		case "cp", "ln", "install":
			if dir, ok := targetDirectory(values); ok {
				targets = []string{dir}
			} else if len(operands) > 1 && (prog != "install" || !hasShortFlagLetter(c.Args[1:], 'd')) {
				targets = operands[len(operands)-1:]
			}
		case "mv":
			if dir, ok := targetDirectory(values); ok {
				targets = append(targets, dir)
			}
		case "patch":
			if containsArg(c.Args[1:], "--dry-run") {
				continue
			}
			if len(targets) > 1 {
				targets = targets[:1]
			}
			if dir, ok := values["d"]; ok {
				targets = []string{dir}
			} else if dir, ok := values["--directory"]; ok {
				targets = []string{dir}
			} else if len(targets) == 0 {
				targets = []string{"."}
			}
		}
		if len(targets) > 0 && !allHarmless(targets) {
			out = append(out, prog+" changes files")
		}
	}
	return out
}

func targetDirectory(values map[string]string) (string, bool) {
	if dir, ok := values["t"]; ok {
		return dir, true
	}
	dir, ok := values["--target-directory"]
	return dir, ok
}

// findStartingPoints returns find's path operands, which come before its
// first expression; with none, find searches the current directory.
func findStartingPoints(args []string) []string {
	var out []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") || a == "(" || a == `\(` || a == "!" {
			break
		}
		out = append(out, expandedOperand(a))
	}
	if len(out) == 0 {
		return []string{"."}
	}
	return out
}

func allHarmless(paths []string) bool {
	for _, p := range paths {
		if p != "/tmp" && !harmlessOutput(p) {
			return false
		}
	}
	return true
}

var (
	interpreterProgram = regexp.MustCompile(`^(python[0-9.]*|node|nodejs|ruby|perl|php[0-9.]*)$`)
	shellProgram       = regexp.MustCompile(`^(bash|dash|ksh|zsh|sh)$`)
	// Writes to the process's own output streams are not file edits.
	streamWrite = regexp.MustCompile(`(?i)std(out|err)\.(sys)?write(lines)?`)
	// fileWrite matches calls that change files. Names must be whole
	// identifiers, so rewrite_rules or csv.writer do not count, and a
	// bare name needs a call, so the word "write" in a string does not.
	fileWrite = regexp.MustCompile(strings.Join([]string{
		`\.(write|writelines|syswrite)\b`,
		`\b(write|writelines|write_text|write_bytes|writeFile|writeFileSync|writeSync|appendFile|appendFileSync|createWriteStream|fwrite|fputs|file_put_contents|syswrite|truncate|ftruncate|unlink|unlinkSync|rename|renameSync|rmSync|rmtree|copyfile|copyFile|copyFileSync)\s*\(`,
		`\bos\.(remove|removedirs|replace|rmdir)\b`,
		`\bshutil\.(move|copy\w*)\b`,
		`\bFileUtils\.(cp|cp_r|cp_lr|copy|copy_file|copy_entry|mv|move|rm|rm_f|rm_r|rm_rf|rmtree|remove|remove_dir|remove_file|remove_entry|remove_entry_secure|touch|ln|ln_s|ln_sf|link|symlink|install)\b`,
		`\bFile\.delete\b`,
		// open with a write, append, create, or update mode: Python,
		// Ruby, PHP, and Node calls, pathlib's Path.open, and Perl's
		// two- and three-argument open with or without parentheses.
		`\b(f?open|openSync)\s*\((?:[^()]|\([^()]*\))*?,\s*(mode\s*=\s*)?` + openMode,
		`\.open\s*\(\s*(mode\s*=\s*)?` + openMode,
		`\bopen\s+(my\s+)?[$\w]+\s*,\s*` + openMode,
	}, "|"))
)

const openMode = `['"]([rbt]*[wax+][rwabtx+]*|\+?>[^'"]*|\+<[^'"]*)['"]`

// inlineCodeFlags lists, per interpreter, the short flag letters and long
// flags whose next argument is program text.
var inlineCodeFlags = map[string]struct {
	short string
	long  []string
}{
	"python": {"c", nil},
	"node":   {"ep", []string{"--eval", "--print"}},
	"ruby":   {"e", nil},
	"perl":   {"eE", nil},
	"php":    {"r", nil},
}

func interpreterFamily(prog string) string {
	switch {
	case strings.HasPrefix(prog, "python"):
		return "python"
	case strings.HasPrefix(prog, "node"):
		return "node"
	case strings.HasPrefix(prog, "php"):
		return "php"
	}
	return prog
}

// codeArgs returns the words that hold program text: the word after a
// code flag (python -c, node --eval, perl -ne, php -r), or the attached
// value of a long flag (--eval=...). args are the resolved literals used
// to find the flags; words, the same command as resolved literals or as
// source text, supply the result.
func codeArgs(args, words []string, short string, long []string) []string {
	var out []string
	for i := 1; i < len(args); i++ {
		a := args[i]
		takesNext := false
		for _, l := range long {
			if a == l {
				takesNext = true
			} else if strings.HasPrefix(a, l+"=") {
				out = append(out, strings.TrimPrefix(words[i], l+"="))
			}
		}
		if len(a) > 1 && a[0] == '-' && a[1] != '-' && strings.ContainsRune(short, rune(a[len(a)-1])) {
			takesNext = true
		}
		if takesNext && i+1 < len(args) {
			out = append(out, words[i+1])
			i++
		}
	}
	return out
}

// interpreterEditFindings reports interpreters that run inline code (a
// here-document, a here-string, piped echo or here-document text, or a
// code flag such as -c, -e, --eval, or -r) that writes files, such as
// python3 - <<'EOF' ... open(p, "w").write(...) ... EOF. The grader never
// sees these edits as file paths, only as program text.
func interpreterEditFindings(s shellScript) []string {
	var out []string
	for _, c := range s.Commands {
		prog := c.program()
		if !interpreterProgram.MatchString(prog) {
			continue
		}
		flags := inlineCodeFlags[interpreterFamily(prog)]
		code := append(append([]string{}, c.Stdin...), codeArgs(c.Args, c.Source, flags.short, flags.long)...)
		for _, src := range code {
			if fileWrite.MatchString(streamWrite.ReplaceAllString(src, "")) {
				out = append(out, prog+" inline code writes files")
				break
			}
		}
	}
	return out
}

// withNestedShells returns s followed by the scripts that its shell
// commands run: bash -c '...' arguments and here-documents fed to a
// shell. Nesting stops after a few levels.
func withNestedShells(s shellScript) []shellScript {
	out := []shellScript{s}
	for depth, next := 0, []shellScript{s}; depth < 3 && len(next) > 0; depth++ {
		var found []shellScript
		for _, script := range next {
			for _, c := range script.Commands {
				if !shellProgram.MatchString(c.program()) {
					continue
				}
				for _, src := range append(codeArgs(c.Args, c.Args, "c", nil), c.Stdin...) {
					if nested, ok := parseShell(src); ok && src != "" {
						found = append(found, nested)
					}
				}
			}
		}
		out = append(out, found...)
		next = found
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
