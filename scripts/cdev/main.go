package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ory/dockertest/v3/docker"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/scripts/cdev/api"
	"github.com/coder/coder/v2/scripts/cdev/catalog"
	"github.com/coder/serpent"
)

func main() {
	cmd := &serpent.Command{
		Use:   "cdev",
		Short: "Development environment manager for Coder",
		Long:  "A smart, opinionated tool for running the Coder development stack.",
		Children: []*serpent.Command{
			upCmd(),
			psCmd(),
			resourcesCmd(),
			downCmd(),
			cleanCmd(),
			pprofCmd(),
			logsCmd(),
			generateCmd(),
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigs := make(chan os.Signal, 1)
	// We want to catch SIGINT (Ctrl+C) and SIGTERM (graceful shutdown).
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigs

		// Notify the main function that cleanup is finished.
		// TODO: Would be best to call a `Close()` function and try a graceful shutdown first, but this is good enough for now.
		cancel()
	}()

	err := cmd.Invoke().WithContext(ctx).WithOS().Run()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1) //nolint:gocritic // exitAfterDefer: deferred cancel is for the non-error path.
	}
}

func cleanCmd() *serpent.Command {
	return &serpent.Command{
		Use:   "clean",
		Short: "Remove all cdev-managed resources (volumes, containers, etc.)",
		Handler: func(inv *serpent.Invocation) error {
			logger := slog.Make(catalog.NewLoggerSink(inv.Stderr, nil))
			return catalog.Cleanup(inv.Context(), logger)
		},
	}
}

func downCmd() *serpent.Command {
	return &serpent.Command{
		Use:   "down",
		Short: "Stop all running cdev-managed containers, but keep volumes and other resources.",
		Handler: func(inv *serpent.Invocation) error {
			logger := slog.Make(catalog.NewLoggerSink(inv.Stderr, nil))
			return catalog.Down(inv.Context(), logger)
		},
	}
}

func psCmd() *serpent.Command {
	var apiAddr string
	var interval time.Duration
	return &serpent.Command{
		Use:   "ps",
		Short: "Show status of cdev services.",
		Options: serpent.OptionSet{
			{
				Flag:        "api-addr",
				Description: "Address of the cdev control API server.",
				Default:     "localhost:" + api.DefaultAPIPort,
				Value:       serpent.StringOf(&apiAddr),
			},
			{
				Flag:          "interval",
				FlagShorthand: "n",
				Description:   "Refresh interval (0 to disable auto-refresh).",
				Default:       "2s",
				Value:         serpent.DurationOf(&interval),
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			m := &psModel{
				apiAddr:  apiAddr,
				interval: interval,
			}

			p := tea.NewProgram(m,
				tea.WithContext(inv.Context()),
				tea.WithOutput(inv.Stdout),
				tea.WithInput(inv.Stdin),
			)
			_, err := p.Run()
			return err
		},
	}
}

// psModel is the bubbletea model for the ps command.
type psModel struct {
	apiAddr  string
	interval time.Duration
	services []api.ServiceInfo
	err      error
}

type psTickMsg time.Time
type psDataMsg struct {
	services []api.ServiceInfo
}

func (m *psModel) Init() tea.Cmd {
	cmds := []tea.Cmd{m.fetchData}
	if m.interval > 0 {
		cmds = append(cmds, m.tick())
	}
	return tea.Batch(cmds...)
}

func (m *psModel) tick() tea.Cmd {
	return tea.Tick(m.interval, func(t time.Time) tea.Msg {
		return psTickMsg(t)
	})
}

func (m *psModel) fetchData() tea.Msg {
	url := fmt.Sprintf("http://%s/api/services", m.apiAddr)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req) //nolint:gosec // User-provided API address.
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return xerrors.Errorf("API returned status %d", resp.StatusCode)
	}

	var data api.ListServicesResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return err
	}

	return psDataMsg{services: data.Services}
}

func (m *psModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "r":
			// Manual refresh.
			return m, m.fetchData
		}
	case psTickMsg:
		return m, tea.Batch(m.fetchData, m.tick())
	case psDataMsg:
		m.services = msg.services
		m.err = nil
		return m, nil
	case error:
		m.err = msg
		return m, nil
	}
	return m, nil
}

func (m *psModel) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n\nIs cdev running? Try: cdev up\n\nPress q to quit, r to retry.\n", m.err)
	}

	if len(m.services) == 0 {
		return "Loading...\n"
	}

	var s strings.Builder
	_, _ = s.WriteString("SERVICES\n")
	tw := tabwriter.NewWriter(&s, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tEMOJI\tSTATUS\tCURRENT STEP\tDEPENDS ON")

	// Sort services by name.
	services := slices.Clone(m.services)
	slices.SortFunc(services, func(a, b api.ServiceInfo) int {
		return strings.Compare(a.Name, b.Name)
	})

	for _, svc := range services {
		deps := "-"
		if len(svc.DependsOn) > 0 {
			deps = strings.Join(svc.DependsOn, ", ")
		}
		step := "-"
		if svc.CurrentStep != "" {
			step = svc.CurrentStep
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", svc.Name, svc.Emoji, svc.Status, step, deps)
	}
	_ = tw.Flush()

	if m.interval > 0 {
		_, _ = s.WriteString(fmt.Sprintf("\nRefreshing every %s. Press q to quit, r to refresh.\n", m.interval))
	} else {
		_, _ = s.WriteString("\nPress q to quit, r to refresh.\n")
	}

	return s.String()
}

func resourcesCmd() *serpent.Command {
	var interval time.Duration
	return &serpent.Command{
		Use:     "resources",
		Aliases: []string{"res"},
		Short:   "Watch all cdev-managed resources like containers, images, and volumes.",
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
			client, err := docker.NewClientFromEnv()
			if err != nil {
				return xerrors.Errorf("failed to connect to docker: %w", err)
			}

			m := &watchModel{
				client:   client,
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
	client     *docker.Client
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
	containers, err := m.client.ListContainers(docker.ListContainersOptions{
		All:     true,
		Filters: m.filter,
	})
	if err != nil {
		return err
	}

	vols, err := m.client.ListVolumes(docker.ListVolumesOptions{
		Filters: m.filter,
	})
	if err != nil {
		return err
	}

	imgs, err := m.client.ListImages(docker.ListImagesOptions{
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
	var instance int64
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
  cdev pprof goroutine
  cdev pprof -i 1 heap     # instance 1`,
		Options: serpent.OptionSet{
			{
				Name:          "Instance",
				Description:   "Coderd instance index (0-based).",
				Flag:          "instance",
				FlagShorthand: "i",
				Default:       "0",
				Value:         serpent.Int64Of(&instance),
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			if len(inv.Args) != 1 {
				_ = serpent.DefaultHelpFn()(inv)
				return xerrors.New("expected exactly one argument: the profile name")
			}
			profile := inv.Args[0]

			url := fmt.Sprintf("http://localhost:%d/debug/pprof/%s", catalog.PprofPortNum(int(instance)), profile)
			if profile == "profile" || profile == "trace" {
				url += "?seconds=30"
			}

			_, _ = fmt.Fprintf(inv.Stdout, "Opening pprof web UI for instance %d, %q at %s\n", instance, profile, url)

			//nolint:gosec // User-provided profile name is passed as a URL path.
			cmd := exec.CommandContext(inv.Context(), "go", "tool", "pprof", "-http=:", url)
			cmd.Stdout = inv.Stdout
			cmd.Stderr = inv.Stderr
			return cmd.Run()
		},
	}
}

func logsCmd() *serpent.Command {
	var follow bool
	return &serpent.Command{
		Use:   "logs <service>",
		Short: "Show logs for a cdev-managed service",
		Long: `Show logs for a cdev-managed service container.

Available services:
  coderd       Main Coder API server
  postgres     PostgreSQL database
  oidc         OIDC test provider
  provisioner  Provisioner daemon
  prometheus   Prometheus metrics server
  site         Frontend development server

Examples:
  cdev logs coderd
  cdev logs -f postgres`,
		Options: serpent.OptionSet{
			{
				Flag:          "follow",
				FlagShorthand: "f",
				Description:   "Follow log output (like tail -f).",
				Default:       "false",
				Value:         serpent.BoolOf(&follow),
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			if len(inv.Args) != 1 {
				_ = serpent.DefaultHelpFn()(inv)
				return xerrors.New("expected exactly one argument: the service name")
			}
			service := inv.Args[0]

			client, err := docker.NewClientFromEnv()
			if err != nil {
				return xerrors.Errorf("failed to connect to docker: %w", err)
			}

			// Find containers matching the service label.
			filter := catalog.NewServiceLabels(catalog.ServiceName(service)).Filter()
			containers, err := client.ListContainers(docker.ListContainersOptions{
				All:     true,
				Filters: filter,
			})
			if err != nil {
				return xerrors.Errorf("failed to list containers: %w", err)
			}

			if len(containers) == 0 {
				return xerrors.Errorf("no container found for service %q", service)
			}

			// Use the first container's name (strip leading slash).
			containerName := strings.TrimPrefix(containers[0].Names[0], "/")

			// Build docker logs command.
			args := []string{"logs"}
			if follow {
				args = append(args, "-f")
			}
			args = append(args, containerName)

			//nolint:gosec // User-provided service name is validated by docker.
			cmd := exec.CommandContext(inv.Context(), "docker", args...)
			cmd.Stdout = inv.Stdout
			cmd.Stderr = inv.Stderr
			return cmd.Run()
		},
	}
}

func generateCmd() *serpent.Command {
	var (
		coderdCount      int64
		provisionerCount int64
		oidc             bool
		prometheus       bool
		outputFile       string
	)

	return &serpent.Command{
		Use:   "generate",
		Short: "Generate docker-compose.yml for the cdev stack",
		Options: serpent.OptionSet{
			{
				Flag:    "coderd-count",
				Default: "1",
				Value:   serpent.Int64Of(&coderdCount),
			},
			{
				Flag:    "provisioner-count",
				Default: "0",
				Value:   serpent.Int64Of(&provisionerCount),
			},
			{
				Flag:    "oidc",
				Default: "false",
				Value:   serpent.BoolOf(&oidc),
			},
			{
				Flag:    "prometheus",
				Default: "false",
				Value:   serpent.BoolOf(&prometheus),
			},
			{
				Flag:          "output",
				FlagShorthand: "o",
				Description:   "Output file (default: stdout).",
				Value:         serpent.StringOf(&outputFile),
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			cwd, err := os.Getwd()
			if err != nil {
				return xerrors.Errorf("get working directory: %w", err)
			}

			dockerGroup := os.Getenv("DOCKER_GROUP")
			if dockerGroup == "" {
				dockerGroup = "999"
			}
			dockerSocket := os.Getenv("DOCKER_SOCKET")
			if dockerSocket == "" {
				dockerSocket = "/var/run/docker.sock"
			}

			cfg := catalog.ComposeConfig{
				CoderdCount:      int(coderdCount),
				ProvisionerCount: int(provisionerCount),
				OIDC:             oidc,
				Prometheus:       prometheus,
				DockerGroup:      dockerGroup,
				DockerSocket:     dockerSocket,
				CWD:              cwd,
				License:          os.Getenv("CODER_LICENSE"),
			}

			data, err := catalog.GenerateYAML(cfg)
			if err != nil {
				return xerrors.Errorf("generate compose YAML: %w", err)
			}

			if outputFile != "" {
				if err := os.WriteFile(outputFile, data, 0o644); err != nil { //nolint:gosec // G306: Generated compose file, 0o644 is intentional.
					return xerrors.Errorf("write output file: %w", err)
				}
				_, _ = fmt.Fprintf(inv.Stdout, "Wrote compose file to %s\n", outputFile)
				return nil
			}

			_, err = inv.Stdout.Write(data)
			return err
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
		catalog.NewSetup(),
		catalog.NewSite(),
		catalog.NewLoadBalancer(),
	)
	if err != nil {
		panic(fmt.Sprintf("failed to register services: %v", err))
	}

	// Create provisioner to collect its options, but don't register
	// it yet — we only register when count > 0 (after option parsing).
	provisioner := catalog.NewProvisioner(services)
	prometheusSvc := catalog.NewPrometheus()

	// Fail fast if HA is enabled without a license.
	catalog.Configure[*catalog.Coderd](services, catalog.OnCoderd(), func(c *catalog.Coderd) {
		if c.HACount() > 1 {
			catalog.RequireLicense("HA coderd (--coderd-count > 1)")
		}
	})

	var apiAddr string
	var startPaused bool

	optionSet := serpent.OptionSet{
		{
			Flag:        "api-addr",
			Description: "Address for the cdev control API server.",
			Default:     "localhost:" + api.DefaultAPIPort,
			Value:       serpent.StringOf(&apiAddr),
		},
		{
			Flag:        "start-paused",
			Description: "Start cdev without auto-starting services. Services can be started via the API or UI.",
			Default:     "false",
			Value:       serpent.BoolOf(&startPaused),
		},
	}
	_ = services.ForEach(func(srv catalog.ServiceBase) error {
		if configurable, ok := srv.(catalog.ConfigurableService); ok {
			optionSet = append(optionSet, configurable.Options()...)
		}
		return nil
	})
	// Add provisioner options even though it's not registered yet,
	// so --provisioner-count always appears in help text.
	optionSet = append(optionSet, provisioner.Options()...)
	optionSet = append(optionSet, prometheusSvc.Options()...)

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
			if prometheusSvc.Enabled() {
				if err := services.Register(prometheusSvc); err != nil {
					return xerrors.Errorf("failed to register prometheus: %w", err)
				}
			}

			services.Init(inv.Stderr)

			if err := services.ApplyConfigurations(); err != nil {
				return xerrors.Errorf("failed to apply configurations: %w", err)
			}

			// Start the API server first so we can query status while services
			// are starting.
			apiServer := api.NewServer(services, services.Logger(), apiAddr)
			if err := apiServer.Start(ctx); err != nil {
				return xerrors.Errorf("failed to start API server: %w", err)
			}
			_, _ = fmt.Fprintf(inv.Stdout, "🔌 API server is ready at http://%s\n", apiAddr)

			if startPaused {
				_, _ = fmt.Fprintln(inv.Stdout, "⏸️  Started in paused mode. Services can be started via the API or UI.")
				_, _ = fmt.Fprintf(inv.Stdout, "   Start all: curl -X POST http://%s/api/services/start\n", apiAddr)
				_, _ = fmt.Fprintf(inv.Stdout, "   UI: http://%s\n", apiAddr)
				<-inv.Context().Done()
				return nil
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
			if prometheusSvc.Enabled() {
				_, _ = fmt.Fprintf(inv.Stdout, "📊 Prometheus is ready at http://localhost:9090\n")
			}
			<-inv.Context().Done()
			return nil
		},
	}
}
