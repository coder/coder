package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/coder/coder/v2/scripts/cdevmux/catalog"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "cdev",
		Short: "Development environment manager for Coder",
		Long:  "A smart, opinionated tool for running the Coder development stack.",
	}

	rootCmd.AddCommand(upCmd())
	rootCmd.AddCommand(downCmd())
	rootCmd.AddCommand(statusCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func upCmd() *cobra.Command {
	var (
		profileName string
		withOIDC    bool
		withProxy   bool
		provCount   int
	)

	cmd := &cobra.Command{
		Use:   "up",
		Short: "Start the development environment",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()

			// Handle shutdown signals.
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

			cat := catalog.New()

			// Load profile or build custom config.
			if profileName != "" {
				profiles := catalog.Profiles()
				profile, ok := profiles[profileName]
				if !ok {
					return fmt.Errorf("unknown profile: %s", profileName)
				}
				for _, svc := range profile.Services {
					if err := cat.Register(svc); err != nil {
						return err
					}
				}
				fmt.Printf("📦 Using profile: %s\n", profile.Name)
			} else {
				// Custom configuration from flags.
				if err := cat.Register(catalog.NewBuildSlim()); err != nil {
					return err
				}
				if err := cat.Register(catalog.NewDatabase()); err != nil {
					return err
				}
				if err := cat.Register(catalog.NewCoderd()); err != nil {
					return err
				}

				if withOIDC {
					if err := cat.Register(catalog.NewOIDC(catalog.OIDCVariantFake)); err != nil {
						return err
					}
				}
				if withProxy {
					if err := cat.Register(catalog.NewWSProxy()); err != nil {
						return err
					}
				}
				if provCount > 0 {
					prov := catalog.NewProvisioner()
					prov.Count = provCount
					if err := cat.Register(prov); err != nil {
						return err
					}
				}
			}

			fmt.Println("🚀 Starting development environment...")

			if err := cat.Start(ctx); err != nil {
				return fmt.Errorf("failed to start: %w", err)
			}

			fmt.Println("✅ Development environment ready!")
			fmt.Println("   Coderd: http://127.0.0.1:3000")
			fmt.Println("\nPress Ctrl+C to stop...")

			// Wait for shutdown signal.
			<-sigCh
			fmt.Println("\n🛑 Shutting down...")

			if err := cat.Stop(context.Background()); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: shutdown error: %v\n", err)
			}

			fmt.Println("👋 Goodbye!")
			return nil
		},
	}

	cmd.Flags().StringVarP(&profileName, "profile", "p", "", "Use a predefined profile (default, full)")
	cmd.Flags().BoolVar(&withOIDC, "oidc", false, "Enable OIDC provider")
	cmd.Flags().BoolVar(&withProxy, "wsproxy", false, "Enable workspace proxy")
	cmd.Flags().IntVar(&provCount, "provisioner-daemons", 0, "Number of external provisioner daemons")

	return cmd
}

func downCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "down",
		Short: "Stop and clean up the development environment",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			fmt.Println("🧹 Cleaning up cdev resources...")

			// Stop all containers with cdev label.
			//nolint:gosec
			out, err := execCommand(ctx, "docker", "ps", "-q", "--filter", "label=cdev=true")
			if err == nil && len(out) > 0 {
				//nolint:gosec
				_, _ = execCommand(ctx, "docker", "stop", string(out))
				//nolint:gosec
				_, _ = execCommand(ctx, "docker", "rm", string(out))
			}

			fmt.Println("✅ Cleanup complete!")
			return nil
		},
	}
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show status of development environment",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			fmt.Println("📊 cdev Status")
			fmt.Println("──────────────")

			// Check for running containers.
			out, err := execCommand(ctx, "docker", "ps", "--filter", "label=cdev=true", "--format", "{{.Names}}\t{{.Status}}")
			if err != nil {
				return fmt.Errorf("failed to check containers: %w", err)
			}

			if len(out) == 0 {
				fmt.Println("No cdev services running.")
			} else {
				fmt.Println(string(out))
			}

			return nil
		},
	}
}

func execCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	//nolint:gosec
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.Output()
}
