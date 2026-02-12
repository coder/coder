package main

import (
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/scripts/cdev/catalog"
	"github.com/coder/coder/v2/scripts/cdev/cleanup"
	"github.com/coder/serpent"
)

func main() {
	cmd := &serpent.Command{
		Use:   "cdev",
		Short: "Development environment manager for Coder",
		Long:  "A smart, opinionated tool for running the Coder development stack.",
		Children: []*serpent.Command{
			upCmd(),
			watchCmd(),
			downCmd(),
			cleanCmd(),
			pprofCmd(),
		},
	}

	err := cmd.Invoke().WithOS().Run()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func cleanCmd() *serpent.Command {
	return &serpent.Command{
		Use:   "clean",
		Short: "Remove all cdev-managed resources (volumes, containers, etc.)",
		Handler: func(inv *serpent.Invocation) error {
			pool, err := dockertest.NewPool("")
			if err != nil {
				return xerrors.Errorf("failed to connect to docker: %w", err)
			}

			logger := slog.Make(catalog.NewLoggerSink(inv.Stderr, nil))

			return cleanup.Cleanup(inv.Context(), logger, pool)
		},
	}
}

func downCmd() *serpent.Command {
	return &serpent.Command{
		Use:   "down",
		Short: "Stop all running cdev-managed containers, but keep volumes and other resources.",
		Handler: func(inv *serpent.Invocation) error {
			pool, err := dockertest.NewPool("")
			if err != nil {
				return xerrors.Errorf("failed to connect to docker: %w", err)
			}

			logger := slog.Make(catalog.NewLoggerSink(inv.Stderr, nil))

			return cleanup.Down(inv.Context(), logger, pool)
		},
	}
}

func watchCmd() *serpent.Command {
	var interval time.Duration
	return &serpent.Command{
		Use:   "watch",
		Short: "Watch all cdev-managed containers and volumes.",
		Options: serpent.OptionSet{
			{
				Flag:          "interval",
				FlagShorthand: "n",
				Description:   "Refresh interval.",
				Default:       "2s",
				Value:         serpent.DurationOf(&interval),
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			pool, err := dockertest.NewPool("")
			if err != nil {
				return xerrors.Errorf("failed to connect to docker: %w", err)
			}

			m := &watchModel{
				pool:     pool,
				interval: interval,
				filter:   catalog.NewLabels().Filter(),
			}

			p := tea.NewProgram(m,
				tea.WithContext(inv.Context()),
				tea.WithOutput(inv.Stdout),
				tea.WithInput(inv.Stdin),
			)
			_, err = p.Run()
			return err
		},
	}
}

// watchModel is the bubbletea model for the watch command.
type watchModel struct {
	pool       *dockertest.Pool
	interval   time.Duration
	filter     map[string][]string
	containers []docker.APIContainers
	volumes    []docker.Volume
	images     []docker.APIImages
	err        error
}

type tickMsg time.Time

func (m *watchModel) Init() tea.Cmd {
	return tea.Batch(m.fetchData, m.tick())
}

func (m *watchModel) tick() tea.Cmd {
	return tea.Tick(m.interval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m *watchModel) fetchData() tea.Msg {
	containers, err := m.pool.Client.ListContainers(docker.ListContainersOptions{
		All:     true,
		Filters: m.filter,
	})
	if err != nil {
		return err
	}

	vols, err := m.pool.Client.ListVolumes(docker.ListVolumesOptions{
		Filters: m.filter,
	})
	if err != nil {
		return err
	}

	imgs, err := m.pool.Client.ListImages(docker.ListImagesOptions{
		Filters: m.filter,
		All:     true,
	})
	if err != nil {
		return err
	}

	return dataMsg{containers: containers, volumes: vols, images: imgs}
}

type dataMsg struct {
	containers []docker.APIContainers
	volumes    []docker.Volume
	images     []docker.APIImages
}

func (m *watchModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	case tickMsg:
		return m, tea.Batch(m.fetchData, m.tick())
	case dataMsg:
		m.containers = msg.containers
		m.volumes = msg.volumes
		m.images = msg.images
		return m, nil
	case error:
		m.err = msg
		return m, tea.Quit
	}
	return m, nil
}

func (m *watchModel) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n", m.err)
	}

	var s strings.Builder

	// Containers table.
	_, _ = s.WriteString("CONTAINERS\n")
	tw := tabwriter.NewWriter(&s, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tIMAGE\tSTATUS\tPORTS")

	// Sort containers by name.
	containers := slices.Clone(m.containers)
	slices.SortFunc(containers, func(a, b docker.APIContainers) int {
		return strings.Compare(a.Names[0], b.Names[0])
	})
	for _, c := range containers {
		name := strings.TrimPrefix(c.Names[0], "/")
		ports := formatPorts(c.Ports)
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", name, c.Image, c.Status, ports)
	}
	_ = tw.Flush()
	if len(containers) == 0 {
		_, _ = s.WriteString("  (none)\n")
	}

	// Volumes table.
	_, _ = s.WriteString("\nVOLUMES\n")
	tw = tabwriter.NewWriter(&s, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tDRIVER\tLABELS")

	// Sort volumes by name.
	volumes := slices.Clone(m.volumes)
	slices.SortFunc(volumes, func(a, b docker.Volume) int {
		return strings.Compare(a.Name, b.Name)
	})
	for _, v := range volumes {
		labels := formatLabels(v.Labels)
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", v.Name, v.Driver, labels)
	}
	_ = tw.Flush()
	if len(volumes) == 0 {
		_, _ = s.WriteString("  (none)\n")
	}

	// Images table.
	_, _ = s.WriteString("\nIMAGES\n")
	tw = tabwriter.NewWriter(&s, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "TAG\tID\tSIZE\tLABELS")

	// Sort images by tag.
	images := slices.Clone(m.images)
	slices.SortFunc(images, func(a, b docker.APIImages) int {
		aTag := formatImageTag(a.RepoTags)
		bTag := formatImageTag(b.RepoTags)
		return strings.Compare(aTag, bTag)
	})
	for _, img := range images {
		tag := formatImageTag(img.RepoTags)
		id := formatImageID(img.ID)
		size := formatSize(img.Size)
		labels := formatLabels(img.Labels)
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", tag, id, size, labels)
	}
	_ = tw.Flush()
	if len(images) == 0 {
		_, _ = s.WriteString("  (none)\n")
	}

	_, _ = s.WriteString(fmt.Sprintf("\nRefreshing every %s. Press q to quit.\n", m.interval))

	return s.String()
}

func formatPorts(ports []docker.APIPort) string {
	var parts []string
	for _, p := range ports {
		if p.PublicPort != 0 {
			parts = append(parts, fmt.Sprintf("%s:%d->%d/%s", p.IP, p.PublicPort, p.PrivatePort, p.Type))
		}
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ", ")
}

func formatLabels(labels map[string]string) string {
	var parts []string
	for k, v := range labels {
		// Only show cdev-specific labels for brevity.
		if strings.HasPrefix(k, "cdev") {
			parts = append(parts, fmt.Sprintf("%s=%s", k, v))
		}
	}
	if len(parts) == 0 {
		return "-"
	}
	// Sort for deterministic output.
	slices.Sort(parts)
	return strings.Join(parts, ", ")
}

func formatImageTag(repoTags []string) string {
	if len(repoTags) == 0 {
		return "<none>"
	}
	return repoTags[0]
}

func formatImageID(id string) string {
	// Shorten "sha256:abc123..." to "abc123..." (first 12 chars of hash).
	id = strings.TrimPrefix(id, "sha256:")
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func formatSize(bytes int64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)
	switch {
	case bytes >= gb:
		return fmt.Sprintf("%.1fGB", float64(bytes)/gb)
	case bytes >= mb:
		return fmt.Sprintf("%.1fMB", float64(bytes)/mb)
	case bytes >= kb:
		return fmt.Sprintf("%.1fKB", float64(bytes)/kb)
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

func pprofCmd() *serpent.Command {
	return &serpent.Command{
		Use:   "pprof <profile>",
		Short: "Open pprof web UI for a running coderd instance",
		Long: `Open the pprof web UI for a running coderd instance.

Supported profiles:
  profile      CPU profile (30s sample)
  heap         Heap memory allocations
  goroutine    Stack traces of all goroutines
  allocs       Past memory allocations
  block        Stack traces of blocking operations
  mutex        Stack traces of mutex contention
  threadcreate Stack traces that led to new OS threads
  trace        Execution trace (30s sample)

Examples:
  cdev pprof heap
  cdev pprof profile
  cdev pprof goroutine`,
		Handler: func(inv *serpent.Invocation) error {
			if len(inv.Args) != 1 {
				_ = serpent.DefaultHelpFn()(inv)
				return xerrors.New("expected exactly one argument: the profile name")
			}
			profile := inv.Args[0]

			url := fmt.Sprintf("http://localhost:%d/debug/pprof/%s", catalog.PprofPortNum(0), profile)
			if profile == "profile" || profile == "trace" {
				url += "?seconds=30"
			}

			_, _ = fmt.Fprintf(inv.Stdout, "Opening pprof web UI for %q at %s\n", profile, url)

			//nolint:gosec // User-provided profile name is passed as a URL path.
			cmd := exec.CommandContext(inv.Context(), "go", "tool", "pprof", "-http=:", url)
			cmd.Stdout = inv.Stdout
			cmd.Stderr = inv.Stderr
			return cmd.Run()
		},
	}
}

func upCmd() *serpent.Command {
	services := catalog.New()
	err := services.Register(
		catalog.NewDocker(),
		catalog.NewBuildSlim(),
		catalog.NewPostgres(),
		catalog.NewCoderd(),
		catalog.NewOIDC(),
	)
	if err != nil {
		panic(fmt.Sprintf("failed to register services: %v", err))
	}

	// Create provisioner to collect its options, but don't register
	// it yet — we only register when count > 0 (after option parsing).
	provisioner := catalog.NewProvisioner(services)
	prometheus := catalog.NewPrometheus()

	// Fail fast if HA is enabled without a license.
	catalog.Configure[*catalog.Coderd](services, catalog.OnCoderd(), func(c *catalog.Coderd) {
		if c.HACount() > 1 {
			catalog.RequireLicense("HA coderd (--coderd-count > 1)")
		}
	})

	optionSet := serpent.OptionSet{}
	_ = services.ForEach(func(srv catalog.ServiceBase) error {
		if configurable, ok := srv.(catalog.ConfigurableService); ok {
			optionSet = append(optionSet, configurable.Options()...)
		}
		return nil
	})
	// Add provisioner options even though it's not registered yet,
	// so --provisioner-count always appears in help text.
	optionSet = append(optionSet, provisioner.Options()...)
	optionSet = append(optionSet, prometheus.Options()...)

	return &serpent.Command{
		Use:     "up",
		Short:   "Start the development environment",
		Options: optionSet,
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()

			// Register provisioner only if count > 0.
			if provisioner.Count() > 0 {
				if err := services.Register(provisioner); err != nil {
					return xerrors.Errorf("failed to register provisioner: %w", err)
				}
			}

			// Register prometheus only if enabled.
			if prometheus.Enabled() {
				if err := services.Register(prometheus); err != nil {
					return xerrors.Errorf("failed to register prometheus: %w", err)
				}
			}

			services.Init(inv.Stderr)

			if err := services.ApplyConfigurations(); err != nil {
				return xerrors.Errorf("failed to apply configurations: %w", err)
			}

			_, _ = fmt.Fprintln(inv.Stdout, "🚀 Starting cdev...")

			err = services.Start(ctx)
			if err != nil {
				return xerrors.Errorf("failed to start services: %w", err)
			}

			coderd, ok := services.MustGet(catalog.OnCoderd()).(*catalog.Coderd)
			if !ok {
				return xerrors.New("unexpected type for coderd service")
			}
			_, _ = fmt.Fprintf(inv.Stdout, "✅ Coder is ready at %s\n", coderd.Result().URL)
			if prometheus.Enabled() {
				_, _ = fmt.Fprintf(inv.Stdout, "📊 Prometheus is ready at http://localhost:9090\n")
			}
			return nil
		},
	}
}
