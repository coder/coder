package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/xerrors"
)

// Terminal-Bench tasks each ship their own container image, while an aegis
// experiment runs every trial in one runner image. The agent therefore
// works on a copy of the task's /app in the runner image, and this file
// verifies the result in the task's own image (the hybrid verifier).

const (
	tbSeedsFile = "seeds.json"
	tbLabel     = "chatd-evals"
	// tbVerifierPad covers the verifier's tool installs on top of the
	// task's own verifier timeout.
	tbVerifierPad = 5 * time.Minute
	// emptySeedPlaceholder stands in for an empty /app, because the aegis
	// sidecar refuses a seed whose baseline commit has no files.
	emptySeedPlaceholder = ".gitkeep"
)

var (
	safeTaskID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
	// digestRef is an image reference pinned by digest; its first group is
	// the repository.
	digestRef = regexp.MustCompile(`^([^@\s]+)@sha256:[0-9a-f]{64}$`)
)

// tbTaskList is terminalbench/tasks.json.
type tbTaskList struct {
	Source struct {
		Repo   string `json:"repo"`
		Commit string `json:"commit"`
	} `json:"source"`
	Tasks []tbTask `json:"tasks"`
}

// tbTask is one task of tbTaskList. Image pins the image task.toml names by
// digest.
type tbTask struct {
	ID    string `json:"id"`
	Image string `json:"image"`
}

// tbSeed is one imported task, as recorded in seeds.json.
type tbSeed struct {
	Image            string `json:"image"`
	SeedRepo         string `json:"seed_repo"`
	SeedRef          string `json:"seed_ref"`
	Instruction      string `json:"instruction"`
	TestsDir         string `json:"tests_dir"`
	SolutionDir      string `json:"solution_dir"`
	VerifierTimeoutS int    `json:"verifier_timeout_s"`
	MemoryMB         int    `json:"memory_mb"`
	CPUs             int    `json:"cpus"`
	// Placeholder records that the importer added emptySeedPlaceholder,
	// which verification removes again before running the task tests.
	Placeholder bool `json:"placeholder,omitempty"`
}

type tbSeeds struct {
	SourceCommit string            `json:"source_commit"`
	Tasks        map[string]tbSeed `json:"tasks"`
}

func readSeeds(tbDir string) (tbSeeds, error) {
	var seeds tbSeeds
	data, err := os.ReadFile(filepath.Join(tbDir, tbSeedsFile))
	if err != nil {
		return seeds, xerrors.Errorf("read seeds: %w", err)
	}
	if err := json.Unmarshal(data, &seeds); err != nil {
		return seeds, xerrors.Errorf("parse seeds: %w", err)
	}
	return seeds, nil
}

type stringList []string

func (s *stringList) String() string     { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }

// tbImportCommand pulls each task image, copies its /app into a seed git
// repository with a deterministic commit, and writes seeds.json.
func tbImportCommand(ctx context.Context, args []string, stdout io.Writer) error {
	fsFlags := flag.NewFlagSet("tb-import", flag.ContinueOnError)
	tasksFile := fsFlags.String("tasks", "", "terminalbench/tasks.json")
	repo := fsFlags.String("repo", "", "Terminal-Bench checkout at the pinned commit")
	out := fsFlags.String("out", "", "output directory (seeds.json and seed repositories)")
	var only stringList
	fsFlags.Var(&only, "task", "import only this task (repeatable)")
	if err := fsFlags.Parse(args); err != nil {
		return err
	}
	if *tasksFile == "" || *repo == "" || *out == "" {
		return xerrors.New("--tasks, --repo, and --out are required")
	}
	tasks, list, err := selectedTasks(*tasksFile, only)
	if err != nil {
		return err
	}
	outDir, err := filepath.Abs(*out)
	if err != nil {
		return err
	}
	seeds, err := readSeeds(outDir)
	if err != nil {
		seeds = tbSeeds{}
	}
	if seeds.Tasks == nil || seeds.SourceCommit != list.Source.Commit {
		seeds = tbSeeds{SourceCommit: list.Source.Commit, Tasks: map[string]tbSeed{}}
	}
	for _, task := range tasks {
		seed, err := importTask(ctx, *repo, outDir, task)
		if err != nil {
			return xerrors.Errorf("task %s: %w", task.ID, err)
		}
		seeds.Tasks[task.ID] = seed
		_, _ = fmt.Fprintf(stdout, "imported %s: image %s, seed %s\n", task.ID, seed.Image, seed.SeedRef[:12])
	}
	data, err := json.MarshalIndent(seeds, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outDir, tbSeedsFile), append(data, '\n'), 0o600)
}

func selectedTasks(tasksFile string, only []string) ([]tbTask, tbTaskList, error) {
	var list tbTaskList
	data, err := os.ReadFile(tasksFile)
	if err != nil {
		return nil, list, err
	}
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, list, xerrors.Errorf("parse %s: %w", tasksFile, err)
	}
	known := map[string]tbTask{}
	for _, t := range list.Tasks {
		if !safeTaskID.MatchString(t.ID) {
			return nil, list, xerrors.Errorf("unsafe task id %q", t.ID)
		}
		known[t.ID] = t
	}
	if len(only) == 0 {
		return list.Tasks, list, nil
	}
	tasks := make([]tbTask, 0, len(only))
	for _, id := range only {
		t, ok := known[id]
		if !ok {
			return nil, list, xerrors.Errorf("task %q is not in %s", id, tasksFile)
		}
		tasks = append(tasks, t)
	}
	return tasks, list, nil
}

// pinnedImage returns the image reference to import for a task whose
// task.toml names declared and whose tasks.json entry pins pin, or "" when
// the task builds its image. Tags can be moved to other images, so a named
// image is used only by a digest from its own repository.
func pinnedImage(declared, pin string) (string, error) {
	if declared == "" {
		if pin != "" {
			return "", xerrors.Errorf("tasks.json pins %q, but task.toml names no docker_image", pin)
		}
		return "", nil
	}
	m := digestRef.FindStringSubmatch(pin)
	if m == nil {
		return "", xerrors.Errorf("task.toml names %q; pin it by digest in tasks.json (\"image\": \"<repository>@sha256:<digest>\"), got %q", declared, pin)
	}
	if repo := imageRepository(declared); m[1] != repo {
		return "", xerrors.Errorf("pinned image %q is not from %s, the repository task.toml names", pin, repo)
	}
	return pin, nil
}

// imageRepository returns an image reference without its tag or digest.
func imageRepository(ref string) string {
	ref, _, _ = strings.Cut(ref, "@")
	if i := strings.LastIndex(ref, ":"); i > strings.LastIndex(ref, "/") {
		ref = ref[:i]
	}
	return ref
}

func importTask(ctx context.Context, repo, outDir string, task tbTask) (tbSeed, error) {
	id := task.ID
	taskDir := filepath.Join(repo, "tasks", id)
	toml, err := parseSimpleTOML(filepath.Join(taskDir, "task.toml"))
	if err != nil {
		return tbSeed{}, err
	}
	instruction, err := os.ReadFile(filepath.Join(taskDir, "instruction.md"))
	if err != nil {
		return tbSeed{}, err
	}
	image, err := pinnedImage(toml["environment.docker_image"], task.Image)
	if err != nil {
		return tbSeed{}, err
	}
	if image == "" {
		tag := "chatd-evals-tb-" + id + ":local"
		if out, err := runDocker(ctx, 30*time.Minute, "build", "-t", tag, filepath.Join(taskDir, "environment")); err != nil {
			return tbSeed{}, xerrors.Errorf("docker build: %s: %w", truncate(string(out), 400), err)
		}
		// The seed records the image ID, so a later build that moves the
		// tag does not change the image this import is verified in.
		out, err := runDocker(ctx, time.Minute, "image", "inspect", "--format", "{{.Id}}", tag)
		if err != nil {
			return tbSeed{}, xerrors.Errorf("docker image inspect %s: %s: %w", tag, truncate(string(out), 400), err)
		}
		image = strings.TrimSpace(string(out))
	} else if _, err := runDocker(ctx, time.Minute, "image", "inspect", image); err != nil {
		if out, err := runDocker(ctx, 30*time.Minute, "pull", image); err != nil {
			return tbSeed{}, xerrors.Errorf("docker pull %s: %s: %w", image, truncate(string(out), 400), err)
		}
	}
	tmp, err := os.MkdirTemp("", "chatd-evals-tb-import-")
	if err != nil {
		return tbSeed{}, err
	}
	defer os.RemoveAll(tmp)
	appDir := filepath.Join(tmp, "app")
	if err := copyAppFromImage(ctx, image, appDir); err != nil {
		return tbSeed{}, err
	}
	if err := filepath.WalkDir(appDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Name() == ".git" {
			return xerrors.Errorf("seed contains %s; tasks whose /app holds a git repository are out of scope", p)
		}
		return nil
	}); err != nil {
		return tbSeed{}, err
	}
	placeholder, err := addEmptySeedPlaceholder(appDir)
	if err != nil {
		return tbSeed{}, err
	}
	sha, err := commitSeed(ctx, appDir)
	if err != nil {
		return tbSeed{}, err
	}
	seedRepo := filepath.Join(outDir, "tasks", id, "seed.git")
	if err := os.RemoveAll(seedRepo); err != nil {
		return tbSeed{}, err
	}
	if err := os.MkdirAll(filepath.Dir(seedRepo), 0o755); err != nil {
		return tbSeed{}, err
	}
	if out, err := runGit(ctx, tmp, "clone", "-q", "--bare", appDir, seedRepo); err != nil {
		return tbSeed{}, xerrors.Errorf("clone seed: %s: %w", truncate(string(out), 400), err)
	}
	// A missing limit means the verifier default (zero); a present but
	// unparsable one is an import error rather than a silent default.
	timeout, err := tomlValue(toml, "verifier.timeout_sec", func(v string) (float64, error) { return strconv.ParseFloat(v, 64) })
	if err != nil {
		return tbSeed{}, err
	}
	memory, err := tomlValue(toml, "environment.memory_mb", strconv.Atoi)
	if err != nil {
		return tbSeed{}, err
	}
	cpus, err := tomlValue(toml, "environment.cpus", strconv.Atoi)
	if err != nil {
		return tbSeed{}, err
	}
	return tbSeed{
		Image:            image,
		SeedRepo:         "file://" + seedRepo,
		SeedRef:          sha,
		Instruction:      string(instruction),
		TestsDir:         filepath.Join(taskDir, "tests"),
		SolutionDir:      filepath.Join(taskDir, "solution"),
		VerifierTimeoutS: int(timeout),
		MemoryMB:         memory,
		CPUs:             cpus,
		Placeholder:      placeholder,
	}, nil
}

func tomlValue[T any](toml map[string]string, key string, parse func(string) (T, error)) (T, error) {
	var zero T
	raw, ok := toml[key]
	if !ok {
		return zero, nil
	}
	v, err := parse(raw)
	if err != nil {
		return zero, xerrors.Errorf("task.toml %s = %q: %w", key, raw, err)
	}
	return v, nil
}

// removeContainer is the deferred cleanup of a container this grader
// started. A failure is logged so a leaked container is visible.
func removeContainer(ref string) {
	if out, err := runDocker(context.Background(), time.Minute, "rm", "-f", ref); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "chatd-eval-grader: docker rm -f %s: %v: %s\n", ref, err, truncate(strings.TrimSpace(string(out)), 400))
	}
}

// addEmptySeedPlaceholder adds emptySeedPlaceholder to dir when dir has no
// entries and reports whether it did.
func addEmptySeedPlaceholder(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}
	if len(entries) > 0 {
		return false, nil
	}
	if err := os.WriteFile(filepath.Join(dir, emptySeedPlaceholder), nil, 0o600); err != nil {
		return false, err
	}
	return true, nil
}

// removeEmptySeedPlaceholder undoes addEmptySeedPlaceholder in a tree
// about to be verified, so the task tests see the /app the task defines.
func removeEmptySeedPlaceholder(dir string, seed tbSeed) error {
	if !seed.Placeholder {
		return nil
	}
	err := os.Remove(filepath.Join(dir, emptySeedPlaceholder))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// copyAppFromImage copies an image's /app into dst without running the
// image.
func copyAppFromImage(ctx context.Context, image, dst string) error {
	out, err := runDocker(ctx, time.Minute, "create", "--label", tbLabel+"=tb-import", image)
	if err != nil {
		return xerrors.Errorf("docker create: %s: %w", truncate(string(out), 400), err)
	}
	id := strings.TrimSpace(string(out))
	defer removeContainer(id)
	if out, err := runDocker(ctx, 10*time.Minute, "cp", id+":/app", dst); err != nil {
		return xerrors.Errorf("docker cp /app: %s: %w", truncate(string(out), 400), err)
	}
	return nil
}

// commitSeed turns dir into a git repository with one commit whose
// identity and dates are fixed, so identical content yields the same SHA.
func commitSeed(ctx context.Context, dir string) (string, error) {
	steps := [][]string{
		{"init", "-q", "-b", "main"},
		{"add", "-A", "--force"},
		{"commit", "-q", "--allow-empty", "--no-gpg-sign", "-m", "Terminal-Bench seed"},
	}
	for _, step := range steps {
		if out, err := runGit(ctx, dir, step...); err != nil {
			return "", xerrors.Errorf("git %s: %s: %w", step[0], truncate(string(out), 400), err)
		}
	}
	out, err := runGit(ctx, dir, "rev-parse", "HEAD")
	if err != nil {
		return "", xerrors.Errorf("git rev-parse: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// parseSimpleTOML reads the flat key = value lines of task.toml into
// "section.key" entries. It covers the scalar fields the importer needs,
// not the TOML language.
func parseSimpleTOML(file string) (map[string]string, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	section := ""
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			section = strings.Trim(line, "[] ")
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		if unq, err := strconv.Unquote(value); err == nil {
			value = unq
		}
		out[section+"."+strings.TrimSpace(key)] = value
	}
	return out, scanner.Err()
}

// materializeSeed checks out the task seed into dst.
func materializeSeed(ctx context.Context, seed tbSeed, dst string) error {
	repo := strings.TrimPrefix(seed.SeedRepo, "file://")
	if out, err := runGit(ctx, filepath.Dir(dst), "clone", "-q", "--no-checkout", repo, dst); err != nil {
		return xerrors.Errorf("clone seed: %s: %w", truncate(string(out), 400), err)
	}
	if out, err := runGit(ctx, dst, "checkout", "-q", "--detach", seed.SeedRef); err != nil {
		return xerrors.Errorf("checkout seed %s: %s: %w", seed.SeedRef, truncate(string(out), 400), err)
	}
	return nil
}

type verifyResult struct {
	Reward  string
	LogsDir string
}

// verifyInTaskImage runs the task's tests against appDir inside the task
// image. The agent's tree only ever runs inside that container: it is
// copied in, the tests are mounted read-only, and nothing from the host
// environment is passed through.
func verifyInTaskImage(ctx context.Context, seed tbSeed, appDir, solutionDir, logsOut string) (verifyResult, error) {
	var res verifyResult
	name := "chatd-evals-tb-" + randomHex(6)
	memory := seed.MemoryMB * 2
	if memory < 2048 {
		memory = 2048
	}
	cpus := seed.CPUs
	if cpus < 1 {
		cpus = 1
	}
	timeout := time.Duration(seed.VerifierTimeoutS)*time.Second + tbVerifierPad
	if seed.VerifierTimeoutS <= 0 {
		timeout = 20*time.Minute + tbVerifierPad
	}
	out, err := runDocker(ctx, 2*time.Minute, "run", "-d", "--name", name,
		"--label", tbLabel+"=tb-verify",
		"--memory", strconv.Itoa(memory)+"m", "--memory-swap", strconv.Itoa(memory)+"m",
		"--cpus", strconv.Itoa(cpus), "--pids-limit", "2048",
		"--security-opt", "no-new-privileges",
		"-v", seed.TestsDir+":/tests:ro",
		"--entrypoint", "sh", "-w", "/app", seed.Image, "-c", "sleep 86400")
	if err != nil {
		return res, xerrors.Errorf("start verifier container: %s: %w", truncate(string(out), 400), err)
	}
	defer removeContainer(name)
	if out, err := runDocker(ctx, time.Minute, "exec", name, "sh", "-c", "rm -rf /app && mkdir -p /app /logs/verifier"); err != nil {
		return res, xerrors.Errorf("reset /app: %s: %w", truncate(string(out), 400), err)
	}
	if out, err := runDocker(ctx, 10*time.Minute, "cp", appDir+"/.", name+":/app"); err != nil {
		return res, xerrors.Errorf("copy tree into verifier: %s: %w", truncate(string(out), 400), err)
	}
	var transcript bytes.Buffer
	if solutionDir != "" {
		if out, err := runDocker(ctx, 5*time.Minute, "cp", solutionDir, name+":/solution"); err != nil {
			return res, xerrors.Errorf("copy solution: %s: %w", truncate(string(out), 400), err)
		}
		out, err := runDocker(ctx, timeout, "exec", "-w", "/app", name, "bash", "/solution/solve.sh")
		_, _ = transcript.WriteString("== solve.sh\n")
		_, _ = transcript.Write(out)
		if err != nil {
			_, _ = fmt.Fprintf(&transcript, "solve.sh: %v\n", err)
		}
	}
	out, err = runDocker(ctx, timeout, "exec", "-w", "/app", name, "bash", "/tests/test.sh")
	_, _ = transcript.WriteString("== tests/test.sh\n")
	_, _ = transcript.Write(out)
	if err != nil {
		_, _ = fmt.Fprintf(&transcript, "test.sh: %v\n", err)
	}
	if err := os.MkdirAll(logsOut, 0o755); err != nil {
		return res, err
	}
	res.LogsDir = logsOut
	verifierDir := filepath.Join(logsOut, "verifier")
	skipped, copyErr := copyVerifierLogs(ctx, name, verifierDir)
	if len(skipped) > 0 {
		_, _ = fmt.Fprintf(&transcript, "== /logs/verifier entries the grader did not copy\n%s\n", strings.Join(skipped, "\n"))
	}
	if err := os.WriteFile(filepath.Join(logsOut, "verifier-output.txt"), transcript.Bytes(), 0o600); err != nil {
		return res, err
	}
	if copyErr != nil {
		return res, xerrors.Errorf("copy verifier logs: %w", copyErr)
	}
	reward, err := os.ReadFile(filepath.Join(verifierDir, "reward.txt"))
	if errors.Is(err, fs.ErrNotExist) {
		return res, nil
	}
	if err != nil {
		return res, xerrors.Errorf("read reward: %w", err)
	}
	res.Reward = strings.TrimSpace(string(reward))
	return res, nil
}

// copyVerifierLogs streams the container's /logs/verifier as a tar archive
// into extractVerifierLogs and returns the entries it skipped.
func copyVerifierLogs(ctx context.Context, container, dst string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	cmd, home, err := dockerCommand(ctx, "cp", container+":/logs/verifier/.", "-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(home)
	stderr := &tailBuffer{limit: dockerOutputLimit}
	cmd.Stderr = stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	skipped, extractErr := extractVerifierLogs(stdout, dst)
	if extractErr == nil {
		// Wait closes the pipe, so anything docker writes after the end of
		// the archive is read first.
		_, extractErr = io.Copy(io.Discard, stdout)
	} else {
		cancel()
	}
	waitErr := cmd.Wait()
	if extractErr != nil {
		return skipped, extractErr
	}
	if waitErr != nil {
		return skipped, xerrors.Errorf("docker cp: %s: %w", truncate(strings.TrimSpace(string(stderr.Bytes())), 400), waitErr)
	}
	return skipped, nil
}

// extractVerifierLogs extracts a docker cp archive of /logs/verifier into
// dst. Task tests run agent code, so it writes only regular files,
// directories, and hard links between extracted files: a symlink copied to
// the host would make the grader read a host file when it opens
// reward.txt. It returns a note for every entry it skipped.
func extractVerifierLogs(r io.Reader, dst string) ([]string, error) {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return nil, err
	}
	var skipped []string
	files := map[string]bool{}
	err := walkTar(r, func(hdr *tar.Header, body io.Reader) error {
		name := path.Clean(hdr.Name)
		if name == "." {
			return nil
		}
		if !filepath.IsLocal(name) {
			skipped = append(skipped, hdr.Name+": outside the log directory")
			return nil
		}
		target := filepath.Join(dst, filepath.FromSlash(name))
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := ensureRealParents(dst, name); err != nil {
				return err
			}
			if err := os.Mkdir(target, 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
				return err
			}
		case tar.TypeReg:
			if err := ensureRealParents(dst, name); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, body); err != nil {
				return errors.Join(err, out.Close())
			}
			if err := out.Close(); err != nil {
				return err
			}
			files[name] = true
		case tar.TypeLink:
			// docker cp archives every further name of a file as a hard
			// link to the first one.
			first := path.Clean(hdr.Linkname)
			if !files[first] {
				skipped = append(skipped, name+": hard link to "+hdr.Linkname)
				return nil
			}
			if err := ensureRealParents(dst, name); err != nil {
				return err
			}
			if err := os.Link(filepath.Join(dst, filepath.FromSlash(first)), target); err != nil {
				return err
			}
			files[name] = true
		case tar.TypeSymlink:
			skipped = append(skipped, name+": symlink to "+hdr.Linkname)
		default:
			skipped = append(skipped, name+": not a regular file or directory")
		}
		return nil
	})
	return skipped, err
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(b)
}

// runDocker runs the docker CLI and returns the tail of its combined
// output.
func runDocker(ctx context.Context, timeout time.Duration, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd, home, err := dockerCommand(ctx, args...)
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(home)
	output := &tailBuffer{limit: dockerOutputLimit}
	cmd.Stdout = output
	cmd.Stderr = output
	err = cmd.Run()
	return output.Bytes(), err
}

// dockerCommand prepares the docker CLI without the caller's environment
// beyond what reaching the daemon needs. HOME points at a new empty
// directory, which the caller removes, so no registry credentials are read.
func dockerCommand(ctx context.Context, args ...string) (*exec.Cmd, string, error) {
	home, err := os.MkdirTemp("", "chatd-evals-docker-home-")
	if err != nil {
		return nil, "", err
	}
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Env = []string{"PATH=" + dockerPath(), "HOME=" + home}
	if host := os.Getenv("DOCKER_HOST"); host != "" {
		cmd.Env = append(cmd.Env, "DOCKER_HOST="+host)
	}
	return cmd, home, nil
}

// dockerOutputLimit caps the docker output the grader keeps per command.
// tests/test.sh runs agent-produced code for up to the verifier timeout,
// and its output would otherwise all be held in the grader's memory.
const dockerOutputLimit = 1 << 20

// tailBuffer is an io.Writer that keeps the last limit bytes written to
// it. It holds at most twice limit, so trimming is amortized.
type tailBuffer struct {
	limit int
	buf   []byte
	total int64
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.total += int64(len(p))
	if len(p) >= b.limit {
		b.buf = append(b.buf[:0], p[len(p)-b.limit:]...)
		return len(p), nil
	}
	if len(b.buf)+len(p) > 2*b.limit {
		n := copy(b.buf, b.buf[len(b.buf)-(b.limit-len(p)):])
		b.buf = b.buf[:n]
	}
	b.buf = append(b.buf, p...)
	return len(p), nil
}

// Bytes returns the kept tail, preceded by a line that says how many
// earlier bytes were dropped, if any.
func (b *tailBuffer) Bytes() []byte {
	data := b.buf
	if len(data) > b.limit {
		data = data[len(data)-b.limit:]
	}
	dropped := b.total - int64(len(data))
	if dropped == 0 {
		return data
	}
	return append([]byte(fmt.Sprintf("[%d earlier bytes dropped]\n", dropped)), data...)
}

func dockerPath() string {
	if p := os.Getenv("PATH"); p != "" {
		return p
	}
	return "/usr/local/bin:/usr/bin:/bin"
}

// gradeTerminalBench rehydrates the trial's final /app and runs the task's
// verifier on it. Task success is reward 1.
func gradeTerminalBench(ctx context.Context, req graderRequest, stderr io.Writer) (graderVerdict, error) {
	tbDir, err := stringArg(req.Grader.Args, "tb_dir")
	if err != nil {
		return graderVerdict{}, err
	}
	task, err := stringArg(req.Grader.Args, "task")
	if err != nil {
		return graderVerdict{}, err
	}
	if tbDir == "" || !safeTaskID.MatchString(task) {
		return graderVerdict{}, xerrors.Errorf("terminal_bench mode needs tb_dir and a task id, got %q and %q", tbDir, task)
	}
	seeds, err := readSeeds(tbDir)
	if err != nil {
		return graderVerdict{Passed: false, Reason: "Terminal-Bench seeds unavailable: " + err.Error()}, nil
	}
	seed, ok := seeds.Tasks[task]
	if !ok {
		return graderVerdict{Passed: false, Reason: "task " + task + " was not imported"}, nil
	}
	tmp, err := os.MkdirTemp("", "chatd-evals-tb-verify-")
	if err != nil {
		return graderVerdict{}, err
	}
	defer os.RemoveAll(tmp)
	appDir := filepath.Join(tmp, "app")
	if err := materializeSeed(ctx, seed, appDir); err != nil {
		return graderVerdict{Passed: false, Reason: err.Error()}, nil
	}
	if err := rehydrate(ctx, appDir, req.Paths.BundleDir); err != nil {
		return graderVerdict{Passed: false, Reason: "cannot rebuild the final /app: " + err.Error()}, nil
	}
	if err := os.RemoveAll(filepath.Join(appDir, ".git")); err != nil {
		return graderVerdict{}, err
	}
	if err := removeEmptySeedPlaceholder(appDir, seed); err != nil {
		return graderVerdict{}, err
	}
	logs := filepath.Join(tbDir, "verifier-logs", sanitizePathPart(req.Trial.ExperimentID), sanitizePathPart(req.Trial.ID))
	if err := os.RemoveAll(logs); err != nil {
		return graderVerdict{}, err
	}
	res, err := verifyInTaskImage(ctx, seed, appDir, "", logs)
	if err != nil {
		return graderVerdict{Passed: false, Reason: "verifier failed to run: " + err.Error()}, nil
	}
	_, _ = fmt.Fprintf(stderr, "terminal-bench %s: reward %q, logs %s\n", task, res.Reward, res.LogsDir)
	return rewardVerdict(task, res.Reward, res.LogsDir)
}

// rewardVerdict turns the verifier's reward.txt content into a verdict.
// No reward.txt means the tests did not finish: reward 0. Any other value
// that is not a finite number is a verifier defect, not a task failure.
func rewardVerdict(task, raw, logsDir string) (graderVerdict, error) {
	reward := 0.0
	if raw != "" {
		var err error
		reward, err = strconv.ParseFloat(raw, 64)
		if err != nil {
			return graderVerdict{}, xerrors.Errorf("terminal-bench %s: reward %q is not a number (verifier logs: %s): %w", task, raw, logsDir, err)
		}
		// ParseFloat reads NaN and Inf, which no JSON verdict can carry.
		if math.IsNaN(reward) || math.IsInf(reward, 0) {
			return graderVerdict{}, xerrors.Errorf("terminal-bench %s: reward %q is not a finite number (verifier logs: %s)", task, raw, logsDir)
		}
	}
	return graderVerdict{
		Passed:  reward == 1,
		Reason:  fmt.Sprintf("reward %q (verifier logs: %s)", raw, logsDir),
		Metrics: map[string]any{"reward": reward},
	}, nil
}

var unsafePathPart = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func sanitizePathPart(s string) string {
	s = unsafePathPart.ReplaceAllString(s, "_")
	if s == "" || s == "." || s == ".." {
		return "_"
	}
	return s
}

// tbSelfCheckCommand verifies imported tasks without a model: the reference
// solution must earn reward 1 and the untouched seed reward 0.
func tbSelfCheckCommand(ctx context.Context, args []string, stdout io.Writer) error {
	fsFlags := flag.NewFlagSet("tb-selfcheck", flag.ContinueOnError)
	tbDir := fsFlags.String("tb-dir", "", "directory written by tb-import")
	var only stringList
	fsFlags.Var(&only, "task", "check only this task (repeatable)")
	if err := fsFlags.Parse(args); err != nil {
		return err
	}
	seeds, err := readSeeds(*tbDir)
	if err != nil {
		return err
	}
	ids := only
	if len(ids) == 0 {
		for id := range seeds.Tasks {
			ids = append(ids, id)
		}
		sort.Strings(ids)
	}
	var failures []string
	for _, id := range ids {
		seed, ok := seeds.Tasks[id]
		if !ok {
			return xerrors.Errorf("task %q was not imported", id)
		}
		for _, run := range []struct {
			label, want, solution string
			reward                float64
		}{
			{"solution", "1", seed.SolutionDir, 1},
			{"seed", "0", "", 0},
		} {
			got, logs, err := selfCheckOnce(ctx, *tbDir, id, run.label, seed, run.solution)
			if err != nil {
				return xerrors.Errorf("%s %s: %w", id, run.label, err)
			}
			status := "ok"
			// The self-check needs a reward file: a verifier that never
			// wrote one did not prove the seed fails.
			if reward, err := strconv.ParseFloat(got, 64); err != nil || reward != run.reward {
				status = "FAIL"
				failures = append(failures, fmt.Sprintf("%s %s: reward %q, want %s (logs %s)", id, run.label, got, run.want, logs))
			}
			_, _ = fmt.Fprintf(stdout, "%-28s %-9s reward=%-3s want=%s %s\n", id, run.label, got, run.want, status)
		}
	}
	if len(failures) > 0 {
		return xerrors.Errorf("self-check failed:\n  %s", strings.Join(failures, "\n  "))
	}
	return nil
}

// selfCheckOnce verifies the seed with solutionDir's solve.sh applied
// first, or the untouched seed when solutionDir is empty.
func selfCheckOnce(ctx context.Context, tbDir, id, label string, seed tbSeed, solutionDir string) (reward, logsDir string, err error) {
	tmp, err := os.MkdirTemp("", "chatd-evals-tb-selfcheck-")
	if err != nil {
		return "", "", err
	}
	defer os.RemoveAll(tmp)
	appDir := filepath.Join(tmp, "app")
	if err := materializeSeed(ctx, seed, appDir); err != nil {
		return "", "", err
	}
	if err := os.RemoveAll(filepath.Join(appDir, ".git")); err != nil {
		return "", "", err
	}
	if err := removeEmptySeedPlaceholder(appDir, seed); err != nil {
		return "", "", err
	}
	logs := filepath.Join(tbDir, "selfcheck-logs", id, label)
	if err := os.RemoveAll(logs); err != nil {
		return "", "", err
	}
	res, err := verifyInTaskImage(ctx, seed, appDir, solutionDir, logs)
	if err != nil {
		return "", logs, err
	}
	return res.Reward, logs, nil
}
