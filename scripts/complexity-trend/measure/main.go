// Command measure records the cyclomatic and cognitive complexity of Go
// functions touched by a list of merge commits, comparing each commit with
// its first parent. It can also snapshot every Go function at one revision.
//
// PR mode reads "<number> <merge sha>" lines on stdin and writes
// functions.csv and files.csv to -out. Snapshot mode (-snapshot REV) writes
// snapshot_functions.csv for that revision instead.
package main

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/fzipp/gocyclo"
	"github.com/uudashr/gocognit"
)

func main() {
	repo := flag.String("repo", ".", "path to the git repository")
	out := flag.String("out", ".", "output directory")
	snapshot := flag.String("snapshot", "", "snapshot every Go function at this revision instead of measuring PRs")
	snapshotName := flag.String("snapshot-name", "snapshot_functions.csv", "file name for snapshot output")
	workers := flag.Int("workers", runtime.NumCPU(), "parallel workers")
	flag.Parse()

	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}
	var err error
	if *snapshot != "" {
		err = runSnapshot(*repo, *snapshot, filepath.Join(*out, *snapshotName))
	} else {
		err = runPRs(*repo, *out, *workers, os.Stdin)
	}
	if err != nil {
		log.Fatal(err)
	}
}

// fn is one function declaration measured in one revision of a file.
type fn struct {
	Name      string
	Start     int
	End       int
	Cyclo     int
	Cognitive int
}

type parsedFile struct {
	Exists     bool
	ParseError bool
	Generated  bool
	Funcs      map[string]fn
}

func (p parsedFile) totals() (cyclo, cognit int) {
	for _, f := range p.Funcs {
		cyclo += f.Cyclo
		cognit += f.Cognitive
	}
	return cyclo, cognit
}

// recvName renders a receiver type without pointer or type parameters so
// that "(*T[K]).M" and "(T).M" both map to "T.M".
func recvName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.StarExpr:
		return recvName(t.X)
	case *ast.IndexExpr:
		return recvName(t.X)
	case *ast.IndexListExpr:
		return recvName(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.ParenExpr:
		return recvName(t.X)
	}
	return "?"
}

func funcKey(d *ast.FuncDecl) string {
	if d.Recv != nil && len(d.Recv.List) > 0 {
		return recvName(d.Recv.List[0].Type) + "." + d.Name.Name
	}
	return d.Name.Name
}

func analyze(path string, src []byte) parsedFile {
	p := parsedFile{Exists: true, Funcs: map[string]fn{}}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		p.ParseError = true
		return p
	}
	p.Generated = ast.IsGenerated(f)
	seen := map[string]int{}
	for _, decl := range f.Decls {
		d, ok := decl.(*ast.FuncDecl)
		if !ok || d.Body == nil {
			continue
		}
		name := funcKey(d)
		// init and blank functions can repeat within one file.
		seen[name]++
		if n := seen[name]; n > 1 {
			name = fmt.Sprintf("%s#%d", name, n)
		}
		p.Funcs[name] = fn{
			Name:      name,
			Start:     fset.Position(d.Pos()).Line,
			End:       fset.Position(d.End()).Line,
			Cyclo:     gocyclo.Complexity(d),
			Cognitive: gocognit.Complexity(d),
		}
	}
	return p
}

// catFile reads blobs through a long-running "git cat-file --batch".
type catFile struct {
	cmd *exec.Cmd
	in  io.WriteCloser
	out *bufio.Reader
}

func newCatFile(repo string) (*catFile, error) {
	cmd := exec.Command("git", "-C", repo, "cat-file", "--batch")
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &catFile{cmd: cmd, in: in, out: bufio.NewReaderSize(out, 1<<20)}, nil
}

// get returns the blob at rev:path, or ok=false when it does not exist.
func (c *catFile) get(rev, path string) (data []byte, ok bool, err error) {
	if _, err := fmt.Fprintf(c.in, "%s:%s\n", rev, path); err != nil {
		return nil, false, err
	}
	header, err := c.out.ReadString('\n')
	if err != nil {
		return nil, false, err
	}
	fields := strings.Fields(header)
	if len(fields) == 2 && fields[1] == "missing" {
		return nil, false, nil
	}
	if len(fields) != 3 {
		return nil, false, fmt.Errorf("unexpected cat-file header %q", header)
	}
	size, err := strconv.Atoi(fields[2])
	if err != nil {
		return nil, false, err
	}
	data = make([]byte, size+1) // trailing newline
	if _, err := io.ReadFull(c.out, data); err != nil {
		return nil, false, err
	}
	return data[:size], true, nil
}

func (c *catFile) close() {
	_ = c.in.Close()
	_ = c.cmd.Wait()
}

func (c *catFile) load(rev, path string) (parsedFile, error) {
	if path == "" {
		return parsedFile{Funcs: map[string]fn{}}, nil
	}
	data, ok, err := c.get(rev, path)
	if err != nil {
		return parsedFile{}, err
	}
	if !ok {
		return parsedFile{Funcs: map[string]fn{}}, nil
	}
	return analyze(path, data), nil
}

// fileDiff is one file entry from "git diff -U0".
type fileDiff struct {
	OldPath string // empty when the file was added
	NewPath string // empty when the file was deleted
	Removed []int  // removed line numbers in the old file
	Added   []int  // added line numbers in the new file
}

func parseRange(s string) (start, count int) {
	parts := strings.SplitN(s, ",", 2)
	start, _ = strconv.Atoi(parts[0])
	count = 1
	if len(parts) == 2 {
		count, _ = strconv.Atoi(parts[1])
	}
	return start, count
}

func parseDiff(out []byte) []*fileDiff {
	var files []*fileDiff
	var cur *fileDiff
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 1<<20), 1<<26)
	inHeader := false
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "diff --git "):
			rest := strings.TrimPrefix(line, "diff --git ")
			cur = &fileDiff{}
			if i := strings.Index(rest, " b/"); i >= 0 {
				cur.OldPath = strings.TrimPrefix(rest[:i], "a/")
				cur.NewPath = rest[i+3:]
			}
			files = append(files, cur)
			inHeader = true
		case cur == nil:
		case inHeader && strings.HasPrefix(line, "new file mode"):
			cur.OldPath = ""
		case inHeader && strings.HasPrefix(line, "deleted file mode"):
			cur.NewPath = ""
		case inHeader && strings.HasPrefix(line, "rename from "):
			cur.OldPath = strings.TrimPrefix(line, "rename from ")
		case inHeader && strings.HasPrefix(line, "rename to "):
			cur.NewPath = strings.TrimPrefix(line, "rename to ")
		case inHeader && strings.HasPrefix(line, "--- "):
			if p := strings.TrimPrefix(line, "--- "); p == "/dev/null" {
				cur.OldPath = ""
			} else {
				cur.OldPath = strings.TrimPrefix(p, "a/")
			}
		case inHeader && strings.HasPrefix(line, "+++ "):
			if p := strings.TrimPrefix(line, "+++ "); p == "/dev/null" {
				cur.NewPath = ""
			} else {
				cur.NewPath = strings.TrimPrefix(p, "b/")
			}
		case strings.HasPrefix(line, "@@ "):
			inHeader = false
			// @@ -a,b +c,d @@
			fields := strings.Fields(line)
			if len(fields) < 3 {
				continue
			}
			oldStart, oldCount := parseRange(strings.TrimPrefix(fields[1], "-"))
			newStart, newCount := parseRange(strings.TrimPrefix(fields[2], "+"))
			for i := 0; i < oldCount; i++ {
				cur.Removed = append(cur.Removed, oldStart+i)
			}
			for i := 0; i < newCount; i++ {
				cur.Added = append(cur.Added, newStart+i)
			}
		}
	}
	return files
}

func countIn(lines []int, start, end int) int {
	n := 0
	for _, l := range lines {
		if l >= start && l <= end {
			n++
		}
	}
	return n
}

type prResult struct {
	Index int
	Funcs [][]string
	Files [][]string
	Err   error
}

var funcHeader = []string{
	"pr", "file", "old_file", "func", "status",
	"cyclo_before", "cyclo_after", "cognitive_before", "cognitive_after",
	"loc_before", "loc_after", "lines_added", "lines_removed", "generated", "test",
}

var fileHeader = []string{
	"pr", "file", "old_file", "status", "lines_added", "lines_removed",
	"funcs_before", "funcs_after", "cyclo_before", "cyclo_after",
	"cognitive_before", "cognitive_after", "generated", "test", "parse_error",
}

func b2s(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

func itoa(i int) string { return strconv.Itoa(i) }

func measurePR(repo string, cf *catFile, pr, sha string) ([][]string, [][]string, error) {
	parent := sha + "^1"
	out, err := exec.Command("git", "-C", repo, "diff", "-M", "-U0", "--no-color", "--no-ext-diff",
		parent, sha, "--", "*.go").Output()
	if err != nil {
		return nil, nil, fmt.Errorf("diff %s: %w", sha, err)
	}
	var funcRows, fileRows [][]string
	for _, d := range parseDiff(out) {
		before, err := cf.load(parent, d.OldPath)
		if err != nil {
			return nil, nil, err
		}
		after, err := cf.load(sha, d.NewPath)
		if err != nil {
			return nil, nil, err
		}
		path := d.NewPath
		if path == "" {
			path = d.OldPath
		}
		status := "M"
		switch {
		case d.OldPath == "":
			status = "A"
		case d.NewPath == "":
			status = "D"
		case d.OldPath != d.NewPath:
			status = "R"
		}
		generated := before.Generated || after.Generated
		test := strings.HasSuffix(path, "_test.go")
		cb, gb := before.totals()
		ca, ga := after.totals()
		fileRows = append(fileRows, []string{
			pr, path, d.OldPath, status, itoa(len(d.Added)), itoa(len(d.Removed)),
			itoa(len(before.Funcs)), itoa(len(after.Funcs)), itoa(cb), itoa(ca), itoa(gb), itoa(ga),
			b2s(generated), b2s(test), b2s(before.ParseError || after.ParseError),
		})

		names := map[string]struct{}{}
		for n := range before.Funcs {
			names[n] = struct{}{}
		}
		for n := range after.Funcs {
			names[n] = struct{}{}
		}
		sorted := make([]string, 0, len(names))
		for n := range names {
			sorted = append(sorted, n)
		}
		sort.Strings(sorted)
		for _, n := range sorted {
			bf, hadBefore := before.Funcs[n]
			af, hasAfter := after.Funcs[n]
			added, removed := 0, 0
			if hadBefore {
				removed = countIn(d.Removed, bf.Start, bf.End)
			}
			if hasAfter {
				added = countIn(d.Added, af.Start, af.End)
			}
			var st string
			switch {
			case !hadBefore:
				st = "new"
			case !hasAfter:
				st = "removed"
			case bf.Cyclo != af.Cyclo || bf.Cognitive != af.Cognitive:
				st = "changed"
			case added > 0 || removed > 0:
				st = "touched"
			default:
				continue
			}
			loc := func(f fn, ok bool) int {
				if !ok {
					return 0
				}
				return f.End - f.Start + 1
			}
			funcRows = append(funcRows, []string{
				pr, path, d.OldPath, n, st,
				itoa(bf.Cyclo), itoa(af.Cyclo), itoa(bf.Cognitive), itoa(af.Cognitive),
				itoa(loc(bf, hadBefore)), itoa(loc(af, hasAfter)), itoa(added), itoa(removed),
				b2s(generated), b2s(test),
			})
		}
	}
	return funcRows, fileRows, nil
}

func runPRs(repo, outDir string, workers int, in io.Reader) error {
	type job struct {
		Index   int
		PR, SHA string
	}
	var jobs []job
	sc := bufio.NewScanner(in)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) != 2 {
			continue
		}
		jobs = append(jobs, job{Index: len(jobs), PR: f[0], SHA: f[1]})
	}
	if err := sc.Err(); err != nil {
		return err
	}

	jobCh := make(chan job)
	resCh := make(chan prResult)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cf, err := newCatFile(repo)
			if err != nil {
				log.Fatal(err)
			}
			defer cf.close()
			for j := range jobCh {
				fr, fl, err := measurePR(repo, cf, j.PR, j.SHA)
				resCh <- prResult{Index: j.Index, Funcs: fr, Files: fl, Err: err}
			}
		}()
	}
	go func() {
		for _, j := range jobs {
			jobCh <- j
		}
		close(jobCh)
		wg.Wait()
		close(resCh)
	}()

	results := make([]prResult, len(jobs))
	done := 0
	for r := range resCh {
		if r.Err != nil {
			return fmt.Errorf("PR %s: %w", jobs[r.Index].PR, r.Err)
		}
		results[r.Index] = r
		done++
		if done%100 == 0 {
			log.Printf("measured %d/%d PRs", done, len(jobs))
		}
	}

	write := func(name string, header []string, rows func(prResult) [][]string) error {
		f, err := os.Create(filepath.Join(outDir, name))
		if err != nil {
			return err
		}
		defer f.Close()
		w := csv.NewWriter(f)
		_ = w.Write(header)
		for _, r := range results {
			if err := w.WriteAll(rows(r)); err != nil {
				return err
			}
		}
		w.Flush()
		return w.Error()
	}
	if err := write("functions.csv", funcHeader, func(r prResult) [][]string { return r.Funcs }); err != nil {
		return err
	}
	return write("files.csv", fileHeader, func(r prResult) [][]string { return r.Files })
}

func runSnapshot(repo, rev, outPath string) error {
	out, err := exec.Command("git", "-C", repo, "ls-tree", "-r", "--name-only", rev).Output()
	if err != nil {
		return fmt.Errorf("ls-tree %s: %w", rev, err)
	}
	cf, err := newCatFile(repo)
	if err != nil {
		return err
	}
	defer cf.close()
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	_ = w.Write([]string{"file", "func", "cyclo", "cognitive", "loc", "generated", "test"})
	for _, path := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if !strings.HasSuffix(path, ".go") {
			continue
		}
		p, err := cf.load(rev, path)
		if err != nil {
			return err
		}
		test := strings.HasSuffix(path, "_test.go")
		names := make([]string, 0, len(p.Funcs))
		for n := range p.Funcs {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			fn := p.Funcs[n]
			_ = w.Write([]string{path, n, itoa(fn.Cyclo), itoa(fn.Cognitive), itoa(fn.End - fn.Start + 1), b2s(p.Generated), b2s(test)})
		}
	}
	w.Flush()
	return w.Error()
}
