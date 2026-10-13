// Command chatd-eval-grader grades Coder Agents (chatd) eval trials run by
// coder/aegis. With no arguments it speaks the aegis.grader.v1 subprocess
// protocol: one JSON request on stdin, one verdict on stdout. Subcommands
// summarize finished runs and prepare Terminal-Bench tasks.
package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "grade" {
		return grade(stdin, stdout, stderr)
	}
	var err error
	switch args[0] {
	case "summarize":
		err = summarizeCommand(args[1:], stdout)
	case "tb-import":
		err = tbImportCommand(args[1:], stdout)
	case "tb-selfcheck":
		err = tbSelfCheckCommand(args[1:], stdout)
	case "help", "-h", "--help":
		_, _ = fmt.Fprint(stdout, usage)
		return 0
	default:
		_, _ = fmt.Fprintf(stderr, "chatd-eval-grader: unknown command %q\n\n%s", args[0], usage)
		return 2
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "chatd-eval-grader %s: %v\n", args[0], err)
		return 1
	}
	return 0
}

const usage = `Usage:
  chatd-eval-grader                 grade one trial (aegis.grader.v1 on stdin/stdout)
  chatd-eval-grader summarize       summarize a run (see summarize -h)
  chatd-eval-grader tb-import       import Terminal-Bench tasks into seed repositories
  chatd-eval-grader tb-selfcheck    verify imported tasks with their reference solutions
`
