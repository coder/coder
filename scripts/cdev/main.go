package main

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
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
		},
	}

	err := cmd.Invoke().WithOS().Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
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
				return fmt.Errorf("failed to connect to docker: %w", err)
			}

			logger := slog.Make(sloghuman.Sink(inv.Stderr))

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
				return fmt.Errorf("failed to connect to docker: %w", err)
			}

			logger := slog.Make(sloghuman.Sink(inv.Stderr))

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
				return fmt.Errorf("failed to connect to docker: %w", err)
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

	return dataMsg{containers: containers, volumes: vols}
}

type dataMsg struct {
	containers []docker.APIContainers
	volumes    []docker.Volume
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
	s.WriteString("CONTAINERS\n")
	tw := tabwriter.NewWriter(&s, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tIMAGE\tSTATUS\tPORTS")

	// Sort containers by name.
	containers := slices.Clone(m.containers)
	slices.SortFunc(containers, func(a, b docker.APIContainers) int {
		return strings.Compare(a.Names[0], b.Names[0])
	})
	for _, c := range containers {
		name := strings.TrimPrefix(c.Names[0], "/")
		ports := formatPorts(c.Ports)
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", name, c.Image, c.Status, ports)
	}
	tw.Flush()
	if len(containers) == 0 {
		s.WriteString("  (none)\n")
	}

	// Volumes table.
	s.WriteString("\nVOLUMES\n")
	tw = tabwriter.NewWriter(&s, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tDRIVER\tLABELS")

	// Sort volumes by name.
	volumes := slices.Clone(m.volumes)
	slices.SortFunc(volumes, func(a, b docker.Volume) int {
		return strings.Compare(a.Name, b.Name)
	})
	for _, v := range volumes {
		labels := formatLabels(v.Labels)
		fmt.Fprintf(tw, "%s\t%s\t%s\n", v.Name, v.Driver, labels)
	}
	tw.Flush()
	if len(volumes) == 0 {
		s.WriteString("  (none)\n")
	}

	s.WriteString(fmt.Sprintf("\nRefreshing every %s. Press q to quit.\n", m.interval))

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

	optionSet := serpent.OptionSet{}
	_ = services.ForEach(func(srv catalog.ServiceBase) error {
		if configurable, ok := srv.(catalog.ConfigurableService); ok {
			optionSet = append(optionSet, configurable.Options()...)
		}
		return nil
	})

	return &serpent.Command{
		Use:     "up",
		Short:   "Start the development environment",
		Options: optionSet,
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			logger := slog.Make(sloghuman.Sink(inv.Stderr))
			services.SetLogger(logger)

			fmt.Fprintln(inv.Stdout, "🚀 Starting cdev...")

			err = services.Start(ctx, logger)
			if err != nil {
				return fmt.Errorf("failed to start services: %w", err)
			}

			coderd := services.MustGet(catalog.OnCoderd()).(*catalog.Coderd)
			fmt.Fprintf(inv.Stdout, "✅ Coder is ready at %s\n", coderd.Result().URL)
			return nil
		},
	}
}
