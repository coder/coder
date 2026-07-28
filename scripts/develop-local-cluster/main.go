// Command develop-local-cluster creates and maintains an isolated kind-based
// Coder development deployment.
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mattn/go-isatty"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/cli/config"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

const (
	defaultNamespace   = "coder"
	defaultCoderPort   = 3000
	defaultGatewayPort = 4001
	coderNodePort      = 30080
	gatewayNodePort    = 30081
	kindNodeImage      = "kindest/node:v1.35.0@sha256:4613778f3cfcd10e615029370f5786704559103cf27bef934597ba562b269661"
	postgresImage      = "us-docker.pkg.dev/coder-v2-images-public/public/postgres@sha256:cb51e9f73d5b6fd77340999cc0fdfcf56a1d580daa6f4c2f6c72264993e6de34"
	coderRelease       = "coder"
	postgresRelease    = "coder-db"
	provisionerRelease = "coder-provisioner"
	gatewayRelease     = "coder-ai-gateway"
	databaseURLName    = "coder-db-url"
	databaseAuthName   = "coder-db-auth"
	provisionerKeyName = "coder-provisioner-key"
	gatewayKeyName     = "coder-ai-gateway-key"
	workspaceNamespace = "coder-workspaces"
	healthTimeout      = 5 * time.Minute
	rolloutTimeout     = 5 * time.Minute
)

type reloadOptions struct {
	chartsOnly bool
}

type clusterConfig struct {
	projectRoot        string
	clusterName        string
	namespace          string
	workspaceNamespace string
	coderPort          int64
	gatewayPort        int64
	password           string
	starterTemplate    string
	keepOnFailure      bool
	noLicensePrompt    bool
	coderValues        []string
	gatewayValues      []string
	provisionerValues  []string

	configDir    string
	runtimeDir   string
	imageRepo    string
	currentImage string
	binaryPath   string
	context      string
	coderURL     string
	gatewayURL   string
	childEnv     []string
}

func main() {
	var cfg clusterConfig
	cmd := &serpent.Command{
		Use:   "develop-local-cluster",
		Short: "Run Coder, AI Gateway, and a provisioner in an isolated kind cluster.",
		Long: `Quick start:
  ./scripts/develop-local-cluster.sh up

The first run deploys Coder and offers to add a license when Premium
entitlements are unavailable. If the prompt is skipped, run:
  ./scripts/develop-local-cluster.sh coder licenses add -f <path>
  ./scripts/develop-local-cluster.sh up

Common commands:
  reload                 Build a fresh Coder image and upgrade workloads.
  reload --charts-only   Apply source chart changes without building an image.
  info                   Print connection details and bootstrap state.
  down                   Delete the complete local cluster.

Read scripts/develop-local-cluster/README.md for prerequisites,
configuration, cleanup behavior, and the full workflow.`,
		Options: serpent.OptionSet{
			{
				Flag:        "cluster-name",
				Env:         "CODER_DEV_CLUSTER_NAME",
				Description: "Name of the kind cluster.",
				Value:       serpent.StringOf(&cfg.clusterName),
			},
			{
				Flag:        "coder-port",
				Env:         "CODER_DEV_CLUSTER_CODER_PORT",
				Default:     fmt.Sprint(defaultCoderPort),
				Description: "Loopback port for Coder.",
				Value:       serpent.Int64Of(&cfg.coderPort),
			},
			{
				Flag:        "gateway-port",
				Env:         "CODER_DEV_CLUSTER_GATEWAY_PORT",
				Default:     fmt.Sprint(defaultGatewayPort),
				Description: "Loopback port for AI Gateway.",
				Value:       serpent.Int64Of(&cfg.gatewayPort),
			},
			{
				Flag:        "namespace",
				Env:         "CODER_DEV_CLUSTER_NAMESPACE",
				Default:     defaultNamespace,
				Description: "Namespace for Coder control-plane components.",
				Value:       serpent.StringOf(&cfg.namespace),
			},
			{
				Flag:        "workspace-namespace",
				Env:         "CODER_DEV_CLUSTER_WORKSPACE_NAMESPACE",
				Default:     workspaceNamespace,
				Description: "Namespace where the Kubernetes starter template creates workspaces.",
				Value:       serpent.StringOf(&cfg.workspaceNamespace),
			},
			{
				Flag:        "password",
				Env:         "CODER_DEV_CLUSTER_ADMIN_PASSWORD",
				Default:     "SomeSecurePassword!",
				Description: "Password for the local admin user.",
				Value:       serpent.StringOf(&cfg.password),
			},
			{
				Flag:        "starter-template",
				Env:         "CODER_DEV_CLUSTER_STARTER_TEMPLATE",
				Default:     "kubernetes",
				Description: "Starter template to create after premium setup. Set to empty to skip.",
				Value:       serpent.StringOf(&cfg.starterTemplate),
			},
			{
				Flag:        "keep-on-failure",
				Description: "Keep a newly created cluster when bootstrap fails before Coder is ready.",
				Value:       serpent.BoolOf(&cfg.keepOnFailure),
			},
			{
				Flag:        "no-license-prompt",
				Description: "Do not offer an interactive license prompt.",
				Value:       serpent.BoolOf(&cfg.noLicensePrompt),
			},
			{
				Flag:        "coder-values",
				Description: "Additional Helm values file for Coder. May be repeated.",
				Value:       serpent.StringArrayOf(&cfg.coderValues),
			},
			{
				Flag:        "gateway-values",
				Description: "Additional Helm values file for AI Gateway. May be repeated.",
				Value:       serpent.StringArrayOf(&cfg.gatewayValues),
			},
			{
				Flag:        "provisioner-values",
				Description: "Additional Helm values file for the provisioner. May be repeated.",
				Value:       serpent.StringArrayOf(&cfg.provisionerValues),
			},
		},
		Children: []*serpent.Command{
			upCommand(&cfg),
			reloadCommand(&cfg),
			coderCommand(&cfg),
			infoCommand(&cfg),
			cleanImagesCommand(&cfg),
			downCommand(&cfg),
			resetCommand(&cfg),
		},
	}

	if err := cmd.Invoke(os.Args[1:]...).WithOS().Run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func upCommand(cfg *clusterConfig) *serpent.Command {
	return &serpent.Command{
		Use:   "up",
		Short: "Create or reconcile the local kind development deployment.",
		Handler: func(inv *serpent.Invocation) error {
			if err := cfg.resolve(); err != nil {
				return err
			}
			return cfg.up(inv.Context(), inv)
		},
	}
}

func reloadCommand(cfg *clusterConfig) *serpent.Command {
	var chartsOnly bool
	return &serpent.Command{
		Use:   "reload",
		Short: "Reload the local image or source Helm charts.",
		Options: serpent.OptionSet{
			{
				Flag:        "charts-only",
				Description: "Reapply charts without building or loading an image.",
				Value:       serpent.BoolOf(&chartsOnly),
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			if err := cfg.resolve(); err != nil {
				return err
			}
			return cfg.reload(inv.Context(), reloadOptions{chartsOnly: chartsOnly})
		},
	}
}

func coderCommand(cfg *clusterConfig) *serpent.Command {
	passthrough := func(inv *serpent.Invocation) error {
		if err := cfg.resolve(); err != nil {
			return err
		}
		if len(inv.Args) == 0 {
			return xerrors.New("coder arguments are required")
		}
		return cfg.runCoder(inv.Context(), inv.Args, os.Stdin, os.Stdout, os.Stderr)
	}
	return &serpent.Command{
		Use:         "coder <arguments...>",
		Short:       "Run the local full Coder CLI against this kind deployment.",
		RawArgs:     true,
		Handler:     passthrough,
		HelpHandler: passthrough,
	}
}

func infoCommand(cfg *clusterConfig) *serpent.Command {
	return &serpent.Command{
		Use:   "info",
		Short: "Print connection details and bootstrap stage.",
		Handler: func(inv *serpent.Invocation) error {
			if err := cfg.resolve(); err != nil {
				return err
			}
			return cfg.info(inv.Context(), inv.Stdout)
		},
	}
}

func cleanImagesCommand(cfg *clusterConfig) *serpent.Command {
	return &serpent.Command{
		Use:   "clean-images",
		Short: "Remove unused local images for this cluster.",
		Handler: func(inv *serpent.Invocation) error {
			if err := cfg.resolve(); err != nil {
				return err
			}
			return cfg.cleanImages(inv.Context())
		},
	}
}

func downCommand(cfg *clusterConfig) *serpent.Command {
	return &serpent.Command{
		Use:   "down",
		Short: "Delete the complete local kind deployment.",
		Handler: func(inv *serpent.Invocation) error {
			if err := cfg.resolve(); err != nil {
				return err
			}
			return cfg.down(inv.Context())
		},
	}
}

func resetCommand(cfg *clusterConfig) *serpent.Command {
	return &serpent.Command{
		Use:   "reset",
		Short: "Replace the local kind deployment with a clean deployment.",
		Handler: func(inv *serpent.Invocation) error {
			if err := cfg.resolve(); err != nil {
				return err
			}
			if err := cfg.down(inv.Context()); err != nil {
				return err
			}
			return cfg.up(inv.Context(), inv)
		},
	}
}

func (cfg *clusterConfig) resolve() error {
	root, err := findProjectRoot()
	if err != nil {
		return err
	}
	cfg.projectRoot = root
	if cfg.namespace == "" {
		cfg.namespace = defaultNamespace
	}
	if cfg.workspaceNamespace == "" {
		cfg.workspaceNamespace = workspaceNamespace
	}
	if cfg.clusterName == "" {
		cfg.clusterName = "coder-dev-" + shortHash(root)
	}
	if !validClusterName(cfg.clusterName) {
		return xerrors.Errorf("cluster-name %q must be a DNS label up to 63 characters", cfg.clusterName)
	}
	if !validClusterName(cfg.namespace) {
		return xerrors.Errorf("namespace %q must be a DNS label up to 63 characters", cfg.namespace)
	}
	if !validClusterName(cfg.workspaceNamespace) {
		return xerrors.Errorf("workspace-namespace %q must be a DNS label up to 63 characters", cfg.workspaceNamespace)
	}
	if cfg.namespace == cfg.workspaceNamespace {
		return xerrors.New("namespace and workspace-namespace must differ")
	}
	if cfg.coderPort == 0 {
		cfg.coderPort = defaultCoderPort
	}
	if cfg.gatewayPort == 0 {
		cfg.gatewayPort = defaultGatewayPort
	}
	if cfg.coderPort == cfg.gatewayPort {
		return xerrors.New("coder-port and gateway-port must differ")
	}
	if err := validatePort(cfg.coderPort, "coder-port"); err != nil {
		return err
	}
	if err := validatePort(cfg.gatewayPort, "gateway-port"); err != nil {
		return err
	}
	cfg.configDir = filepath.Join(root, ".coderv2", "clusters", cfg.clusterName)
	cfg.runtimeDir = filepath.Join(cfg.configDir, "runtime")
	cfg.imageRepo = "coder-local/" + cfg.clusterName
	cfg.context = "kind-" + cfg.clusterName
	cfg.coderURL = fmt.Sprintf("http://127.0.0.1:%d", cfg.coderPort)
	cfg.gatewayURL = fmt.Sprintf("http://127.0.0.1:%d", cfg.gatewayPort)
	cfg.binaryPath = filepath.Join(root, "build", fmt.Sprintf("coder_%s_%s", runtime.GOOS, runtime.GOARCH))
	cfg.childEnv = filterEnv(os.Environ(), "CODER_SESSION_TOKEN", "CODER_URL")
	return nil
}

func (cfg *clusterConfig) up(ctx context.Context, inv *serpent.Invocation) (err error) {
	if err := cfg.preflight(ctx); err != nil {
		return err
	}

	created, err := cfg.ensureCluster(ctx)
	if err != nil {
		return err
	}
	if created && !cfg.keepOnFailure {
		controlPlaneReady := false
		defer func() {
			if err != nil && !controlPlaneReady {
				_, _ = fmt.Fprintln(os.Stderr, "bootstrap failed before Coder was ready, removing the new kind cluster")
				_ = cfg.down(context.Background())
			}
		}()
		defer func() { controlPlaneReady = cfg.coderHealthy(context.Background()) }()
	}

	if err := cfg.ensurePortMappings(ctx); err != nil {
		return err
	}
	if err := cfg.ensureNamespaces(ctx); err != nil {
		return err
	}
	if err := cfg.installPostgres(ctx); err != nil {
		return err
	}
	if err := cfg.ensureDatabaseSecret(ctx); err != nil {
		return err
	}
	if created {
		if err := cfg.buildAndLoadImage(ctx); err != nil {
			return err
		}
	} else if err := cfg.ensureExistingImage(ctx); err != nil {
		return err
	}
	if err := cfg.ensureChartDependencies(ctx); err != nil {
		return err
	}
	if err := cfg.installCoder(ctx); err != nil {
		return err
	}
	if err := waitForHTTP(ctx, cfg.coderURL+"/healthz", healthTimeout, "Coder"); err != nil {
		return err
	}
	client, err := cfg.setupFirstUser(ctx)
	if err != nil {
		return err
	}

	ready, missing, err := premiumReady(ctx, client)
	if err != nil {
		return err
	}
	if !ready {
		if cfg.shouldPrompt(inv) {
			answer, promptErr := cliui.Prompt(inv, cliui.PromptOptions{
				Text:      "Premium features require a license. Add one now?",
				Default:   cliui.ConfirmNo,
				IsConfirm: true,
			})
			if promptErr != nil && answer != cliui.ConfirmNo && !errors.Is(promptErr, io.EOF) {
				return xerrors.Errorf("license prompt: %w", promptErr)
			}
			if promptErr == nil && answer == cliui.ConfirmYes {
				if err := cfg.runCoder(ctx, []string{"licenses", "add"}, inv.Stdin, inv.Stdout, inv.Stderr); err != nil {
					return xerrors.Errorf("adding license: %w", err)
				}
				ready, missing, err = premiumReady(ctx, client)
				if err != nil {
					return err
				}
			}
		}
		if !ready {
			cfg.printLicenseNextStep(inv.Stdout, missing)
			return nil
		}
	}

	if err := cfg.ensureComponentSecrets(ctx, client); err != nil {
		return err
	}
	if err := cfg.installProvisioner(ctx); err != nil {
		return err
	}
	if err := cfg.waitForProvisioner(ctx, client); err != nil {
		return err
	}
	if err := cfg.installGateway(ctx); err != nil {
		return err
	}
	if err := waitForHTTP(ctx, cfg.gatewayURL+"/readyz", healthTimeout, "AI Gateway"); err != nil {
		return err
	}
	if err := cfg.setupStarterTemplate(ctx, client); err != nil {
		return err
	}
	cfg.printBanner(inv.Stdout, "complete")
	return nil
}

func (cfg *clusterConfig) reload(ctx context.Context, options reloadOptions) error {
	if !cfg.clusterExists(ctx) {
		return xerrors.Errorf("kind cluster %q does not exist, run up first", cfg.clusterName)
	}
	switch {
	case !options.chartsOnly:
		if err := cfg.buildAndLoadImage(ctx); err != nil {
			return err
		}
	case cfg.readCurrentImage() == "":
		return xerrors.New("no current local image recorded, run up or reload first")
	case !cfg.nodeHasImage(ctx, cfg.readCurrentImage()):
		return xerrors.New("the current image is not loaded in the kind node, run reload without --charts-only")
	}
	if err := cfg.ensureChartDependencies(ctx); err != nil {
		return err
	}
	if err := cfg.installCoder(ctx); err != nil {
		return err
	}
	if err := waitForHTTP(ctx, cfg.coderURL+"/healthz", healthTimeout, "Coder"); err != nil {
		return err
	}
	if cfg.releaseExists(ctx, provisionerRelease) {
		if err := cfg.installProvisioner(ctx); err != nil {
			return err
		}
	}
	if cfg.releaseExists(ctx, gatewayRelease) {
		if err := cfg.installGateway(ctx); err != nil {
			return err
		}
		if err := waitForHTTP(ctx, cfg.gatewayURL+"/readyz", healthTimeout, "AI Gateway"); err != nil {
			return err
		}
	}
	cfg.printBanner(os.Stdout, "reloaded")
	return nil
}

func (cfg *clusterConfig) info(ctx context.Context, out io.Writer) error {
	stage := "cluster not found"
	image := cfg.readCurrentImage()
	if cfg.clusterExists(ctx) {
		stage = "control plane pending"
		if cfg.coderHealthy(ctx) {
			stage = "awaiting license or premium setup"
		}
		if cfg.releaseExists(ctx, provisionerRelease) && cfg.releaseExists(ctx, gatewayRelease) {
			stage = "complete"
		}
	}
	_, _ = fmt.Fprintf(out, "Cluster: %s\nContext: %s\nCoder: %s\nAI Gateway: %s\nImage: %s\nStage: %s\n", cfg.clusterName, cfg.context, cfg.coderURL, cfg.gatewayURL, emptyAs(image, "none"), stage)
	if stage == "awaiting license or premium setup" {
		_, _ = fmt.Fprintln(out, "Next: ./scripts/develop-local-cluster.sh coder licenses add -f <path>, then rerun up")
	}
	return nil
}

func (cfg *clusterConfig) down(ctx context.Context) error {
	clusters, err := cfg.kindClusters(ctx)
	if err != nil {
		return err
	}
	if contains(clusters, cfg.clusterName) {
		if err := cfg.run(ctx, nil, "kind", "delete", "cluster", "--name", cfg.clusterName); err != nil {
			return err
		}
	}
	if err := cfg.removeHostImages(ctx); err != nil {
		return err
	}
	if err := os.RemoveAll(cfg.configDir); err != nil {
		return xerrors.Errorf("remove cluster configuration: %w", err)
	}
	return nil
}

func (cfg *clusterConfig) cleanImages(ctx context.Context) error {
	if cfg.clusterExists(ctx) {
		if err := cfg.cleanNodeImages(ctx); err != nil {
			return err
		}
	}
	return cfg.removeHostImages(ctx)
}

func (cfg *clusterConfig) preflight(ctx context.Context) error {
	for _, command := range []string{"docker", "kind", "kubectl", "helm", "make", "git", "go"} {
		if _, err := exec.LookPath(command); err != nil {
			return xerrors.Errorf("%s is required: %w", command, err)
		}
	}
	if _, err := cfg.output(ctx, "docker", "version", "--format", "{{.Server.Version}}"); err != nil {
		return xerrors.New("Docker is not ready")
	}
	if _, err := cfg.dockerArch(ctx); err != nil {
		return err
	}
	if !cfg.clusterExists(ctx) {
		if portBusy(cfg.coderPort) {
			return xerrors.Errorf("coder port %d is already in use", cfg.coderPort)
		}
		if portBusy(cfg.gatewayPort) {
			return xerrors.Errorf("AI Gateway port %d is already in use", cfg.gatewayPort)
		}
	}
	return nil
}

func (cfg *clusterConfig) ensureCluster(ctx context.Context) (bool, error) {
	if cfg.clusterExists(ctx) {
		return false, nil
	}
	if err := os.MkdirAll(cfg.runtimeDir, 0o750); err != nil {
		return false, xerrors.Errorf("create runtime directory: %w", err)
	}
	configPath := filepath.Join(cfg.runtimeDir, "kind.yaml")
	contents := fmt.Sprintf(`kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
    image: %s
    extraPortMappings:
      - containerPort: %d
        hostPort: %d
        listenAddress: "127.0.0.1"
        protocol: TCP
      - containerPort: %d
        hostPort: %d
        listenAddress: "127.0.0.1"
        protocol: TCP
`, kindNodeImage, coderNodePort, cfg.coderPort, gatewayNodePort, cfg.gatewayPort)
	if err := os.WriteFile(configPath, []byte(contents), 0o600); err != nil {
		return false, xerrors.Errorf("write kind configuration: %w", err)
	}
	if err := cfg.run(ctx, nil, "kind", "create", "cluster", "--name", cfg.clusterName, "--config", configPath, "--wait", "5m"); err != nil {
		return false, err
	}
	if err := cfg.waitForKindAddons(ctx); err != nil {
		if !cfg.keepOnFailure {
			_ = cfg.run(context.Background(), nil, "kind", "delete", "cluster", "--name", cfg.clusterName)
		}
		return false, err
	}
	return true, nil
}

func (cfg *clusterConfig) waitForKindAddons(ctx context.Context) error {
	for _, rollout := range [][]string{
		{"--context", cfg.context, "--namespace", "kube-system", "rollout", "status", "deployment/coredns", "--timeout", rolloutTimeout.String()},
		{"--context", cfg.context, "--namespace", "local-path-storage", "rollout", "status", "deployment/local-path-provisioner", "--timeout", rolloutTimeout.String()},
	} {
		if err := cfg.run(ctx, nil, "kubectl", rollout...); err != nil {
			return xerrors.Errorf("wait for kind addon: %w", err)
		}
	}
	return nil
}

func (cfg *clusterConfig) ensurePortMappings(ctx context.Context) error {
	for _, mapping := range []struct {
		name          string
		containerPort int
		hostPort      int64
	}{
		{name: "Coder", containerPort: coderNodePort, hostPort: cfg.coderPort},
		{name: "AI Gateway", containerPort: gatewayNodePort, hostPort: cfg.gatewayPort},
	} {
		port, err := cfg.output(ctx, "docker", "inspect", "--format", fmt.Sprintf("{{(index (index .NetworkSettings.Ports \"%d/tcp\") 0).HostPort}}", mapping.containerPort), cfg.clusterName+"-control-plane")
		if err != nil {
			return xerrors.Errorf("read %s port mapping: %w", mapping.name, err)
		}
		if port != fmt.Sprint(mapping.hostPort) {
			return xerrors.Errorf("%s is mapped to host port %s, not requested port %d. Run down before changing host ports", mapping.name, port, mapping.hostPort)
		}
	}
	return nil
}

func (cfg *clusterConfig) ensureNamespaces(ctx context.Context) error {
	for _, namespace := range []string{cfg.namespace, cfg.workspaceNamespace} {
		manifest := fmt.Sprintf("apiVersion: v1\nkind: Namespace\nmetadata:\n  name: %s\n", namespace)
		if err := cfg.run(ctx, strings.NewReader(manifest), "kubectl", "--context", cfg.context, "apply", "-f", "-"); err != nil {
			return err
		}
	}
	return nil
}

func (cfg *clusterConfig) installPostgres(ctx context.Context) error {
	if cfg.releaseExists(ctx, postgresRelease) {
		return xerrors.Errorf("existing Helm-managed PostgreSQL release %q cannot be upgraded to the Coder PostgreSQL image in place. Run ./scripts/develop-local-cluster.sh down, then run up to create a new local database", postgresRelease)
	}
	if cfg.postgresStatefulSetExists(ctx) {
		name, err := cfg.output(ctx, "kubectl", "--context", cfg.context, "--namespace", cfg.namespace, "get", "statefulset", postgresRelease+"-postgresql", "-o", "jsonpath={.spec.selector.matchLabels.app\\.kubernetes\\.io/name}")
		if err != nil {
			return err
		}
		if name != "coder-postgresql" {
			return xerrors.Errorf("existing PostgreSQL StatefulSet %q is not managed by this command. Run ./scripts/develop-local-cluster.sh down before creating a new local database", postgresRelease+"-postgresql")
		}
	}
	if err := cfg.ensureDatabaseCredentials(ctx); err != nil {
		return err
	}

	manifest := fmt.Sprintf(`apiVersion: v1
kind: Service
metadata:
  name: %[1]s-postgresql
  namespace: %[2]s
spec:
  selector:
    app.kubernetes.io/name: coder-postgresql
    app.kubernetes.io/instance: %[1]s
  ports:
    - name: postgresql
      port: 5432
      targetPort: postgresql
---
apiVersion: v1
kind: Service
metadata:
  name: %[1]s-postgresql-headless
  namespace: %[2]s
spec:
  clusterIP: None
  selector:
    app.kubernetes.io/name: coder-postgresql
    app.kubernetes.io/instance: %[1]s
  ports:
    - name: postgresql
      port: 5432
      targetPort: postgresql
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: %[1]s-postgresql
  namespace: %[2]s
spec:
  serviceName: %[1]s-postgresql-headless
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: coder-postgresql
      app.kubernetes.io/instance: %[1]s
  template:
    metadata:
      labels:
        app.kubernetes.io/name: coder-postgresql
        app.kubernetes.io/instance: %[1]s
    spec:
      containers:
        - name: postgresql
          image: %[3]s
          imagePullPolicy: IfNotPresent
          ports:
            - name: postgresql
              containerPort: 5432
          env:
            - name: POSTGRES_USER
              value: coder
            - name: POSTGRES_DB
              value: coder
            - name: POSTGRES_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: %[4]s
                  key: password
            - name: PGDATA
              value: /var/lib/postgresql/data/pgdata
          readinessProbe:
            exec:
              command:
                - sh
                - -ec
                - pg_isready -U "$POSTGRES_USER" -d "$POSTGRES_DB"
            initialDelaySeconds: 2
            periodSeconds: 5
          livenessProbe:
            exec:
              command:
                - sh
                - -ec
                - pg_isready -U "$POSTGRES_USER" -d "$POSTGRES_DB"
            initialDelaySeconds: 10
            periodSeconds: 10
          resources:
            requests:
              cpu: 100m
              memory: 128Mi
          volumeMounts:
            - name: data
              mountPath: /var/lib/postgresql/data
  volumeClaimTemplates:
    - metadata:
        name: data
      spec:
        accessModes:
          - ReadWriteOnce
        resources:
          requests:
            storage: 2Gi
`, postgresRelease, cfg.namespace, postgresImage, databaseAuthName)
	if err := cfg.run(ctx, strings.NewReader(manifest), "kubectl", "--context", cfg.context, "apply", "-f", "-"); err != nil {
		return err
	}
	return cfg.run(ctx, nil, "kubectl", "--context", cfg.context, "--namespace", cfg.namespace, "rollout", "status", "statefulset/"+postgresRelease+"-postgresql", "--timeout", rolloutTimeout.String())
}

func (cfg *clusterConfig) postgresStatefulSetExists(ctx context.Context) bool {
	return cfg.commandSucceeds(ctx, "kubectl", "--context", cfg.context, "--namespace", cfg.namespace, "get", "statefulset", postgresRelease+"-postgresql")
}

func (cfg *clusterConfig) ensureDatabaseCredentials(ctx context.Context) error {
	if cfg.secretExists(ctx, databaseAuthName) {
		return nil
	}
	password, err := randomText(24)
	if err != nil {
		return err
	}
	manifest := fmt.Sprintf("apiVersion: v1\nkind: Secret\nmetadata:\n  name: %s\n  namespace: %s\ntype: Opaque\nstringData:\n  password: %s\n", databaseAuthName, cfg.namespace, password)
	return cfg.run(ctx, strings.NewReader(manifest), "kubectl", "--context", cfg.context, "apply", "-f", "-")
}

func (cfg *clusterConfig) ensureDatabaseSecret(ctx context.Context) error {
	if cfg.secretExists(ctx, databaseURLName) {
		return nil
	}
	passwordEncoded, err := cfg.secretValue(ctx, databaseAuthName, "password")
	if err != nil {
		return err
	}
	password, err := base64.StdEncoding.DecodeString(passwordEncoded)
	if err != nil {
		return xerrors.Errorf("decode database password: %w", err)
	}
	connectionURL := fmt.Sprintf("postgres://coder:%s@%s-postgresql.%s.svc.cluster.local:5432/coder?sslmode=disable", password, postgresRelease, cfg.namespace)
	manifest := fmt.Sprintf("apiVersion: v1\nkind: Secret\nmetadata:\n  name: %s\n  namespace: %s\ntype: Opaque\nstringData:\n  url: %s\n", databaseURLName, cfg.namespace, connectionURL)
	return cfg.run(ctx, strings.NewReader(manifest), "kubectl", "--context", cfg.context, "apply", "-f", "-")
}

func (cfg *clusterConfig) ensureExistingImage(ctx context.Context) error {
	image := cfg.readCurrentImage()
	if image == "" {
		return cfg.buildAndLoadImage(ctx)
	}
	if cfg.nodeHasImage(ctx, image) {
		return nil
	}
	if cfg.hostImageExists(ctx, image) {
		return cfg.run(ctx, nil, "kind", "load", "docker-image", image, "--name", cfg.clusterName)
	}
	return cfg.buildAndLoadImage(ctx)
}

func (cfg *clusterConfig) buildAndLoadImage(ctx context.Context) error {
	arch, err := cfg.dockerArch(ctx)
	if err != nil {
		return err
	}
	if err := cfg.buildCoderBinary(ctx, "linux", arch); err != nil {
		return err
	}
	sha, err := cfg.output(ctx, "git", "rev-parse", "--short", "HEAD")
	if err != nil {
		return err
	}
	nonce, err := randomText(4)
	if err != nil {
		return err
	}
	tag := fmt.Sprintf("%s-%d-%s", strings.TrimSpace(sha), time.Now().UTC().Unix(), nonce)
	cfg.currentImage = cfg.imageRepo + ":" + tag
	linuxBinary := filepath.Join(cfg.projectRoot, "build", fmt.Sprintf("coder_linux_%s", arch))
	if err := cfg.run(ctx, nil, filepath.Join(cfg.projectRoot, "scripts", "build_docker.sh"), "--arch", arch, "--target", cfg.currentImage, linuxBinary); err != nil {
		return err
	}
	if err := cfg.run(ctx, nil, "kind", "load", "docker-image", cfg.currentImage, "--name", cfg.clusterName); err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.runtimeDir, 0o750); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(cfg.runtimeDir, "image"), []byte(cfg.currentImage+"\n"), 0o600); err != nil {
		return xerrors.Errorf("record current image: %w", err)
	}
	return nil
}

func (cfg *clusterConfig) buildCoderBinary(ctx context.Context, osName, arch string) error {
	target := fmt.Sprintf("build/coder_%s_%s", osName, arch)
	return cfg.run(ctx, nil, "make", "-j", target)
}

func (cfg *clusterConfig) ensureChartDependencies(ctx context.Context) error {
	for _, chart := range []string{"helm/coder", "helm/provisioner", "helm/ai-gateway"} {
		if err := cfg.run(ctx, nil, "helm", "dependency", "build", filepath.Join(cfg.projectRoot, chart)); err != nil {
			return err
		}
	}
	return nil
}

func (cfg *clusterConfig) installCoder(ctx context.Context) error {
	valuesPath, err := cfg.writeValues("coder", fmt.Sprintf(`coder:
  image:
    repo: %s
    tag: %s
    pullPolicy: IfNotPresent
  service:
    type: NodePort
    httpNodePort: %d
  envUseClusterAccessURL: true
  serviceAccount:
    workspaceNamespaces:
      - name: %s
  resources:
    requests:
      cpu: 250m
      memory: 512Mi
  env:
    - name: CODER_PG_CONNECTION_URL
      valueFrom:
        secretKeyRef:
          name: %s
          key: url
    - name: CODER_PROVISIONER_DAEMONS
      value: "0"
provisionerDaemon:
  pskSecretName: ""
`, cfg.imageRepository(), cfg.imageTag(), coderNodePort, cfg.workspaceNamespace, databaseURLName))
	if err != nil {
		return err
	}
	return cfg.helmUpgrade(ctx, coderRelease, filepath.Join(cfg.projectRoot, "helm", "coder"), valuesPath, cfg.coderValues)
}

func (cfg *clusterConfig) installProvisioner(ctx context.Context) error {
	valuesPath, err := cfg.writeValues("provisioner", fmt.Sprintf(`coder:
  image:
    repo: %s
    tag: %s
    pullPolicy: IfNotPresent
  env:
    - name: CODER_URL
      value: http://coder.%s.svc.cluster.local:80
  serviceAccount:
    workspaceNamespaces:
      - name: %s
  resources:
    requests:
      cpu: 250m
      memory: 512Mi
provisionerDaemon:
  pskSecretName: ""
  keySecretName: %s
  keySecretKey: key
`, cfg.imageRepository(), cfg.imageTag(), cfg.namespace, cfg.workspaceNamespace, provisionerKeyName))
	if err != nil {
		return err
	}
	return cfg.helmUpgrade(ctx, provisionerRelease, filepath.Join(cfg.projectRoot, "helm", "provisioner"), valuesPath, cfg.provisionerValues)
}

func (cfg *clusterConfig) installGateway(ctx context.Context) error {
	valuesPath, err := cfg.writeValues("ai-gateway", fmt.Sprintf(`coder:
  image:
    repo: %s
    tag: %s
    pullPolicy: IfNotPresent
  resources:
    requests:
      cpu: 250m
      memory: 512Mi
  env:
    - name: CODER_URL
      value: http://coder.%s.svc.cluster.local:80
aigateway:
  keySecret:
    name: %s
    key: key
service:
  type: NodePort
  nodePort: %d
`, cfg.imageRepository(), cfg.imageTag(), cfg.namespace, gatewayKeyName, gatewayNodePort))
	if err != nil {
		return err
	}
	return cfg.helmUpgrade(ctx, gatewayRelease, filepath.Join(cfg.projectRoot, "helm", "ai-gateway"), valuesPath, cfg.gatewayValues)
}

func (cfg *clusterConfig) helmUpgrade(ctx context.Context, release, chart, generatedValues string, userValues []string) error {
	args := []string{
		"upgrade", "--install", release, chart,
		"--namespace", cfg.namespace,
		"--kube-context", cfg.context,
		"--wait", "--timeout", rolloutTimeout.String(),
		"--values", generatedValues,
	}
	for _, value := range userValues {
		args = append(args, "--values", value)
	}
	return cfg.run(ctx, nil, "helm", args...)
}

func (cfg *clusterConfig) writeValues(name, contents string) (string, error) {
	if err := os.MkdirAll(cfg.runtimeDir, 0o750); err != nil {
		return "", xerrors.Errorf("create runtime directory: %w", err)
	}
	path := filepath.Join(cfg.runtimeDir, name+"-values.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		return "", xerrors.Errorf("write %s values: %w", name, err)
	}
	return path, nil
}

func (cfg *clusterConfig) setupFirstUser(ctx context.Context) (*codersdk.Client, error) {
	serverURL, err := url.Parse(cfg.coderURL)
	if err != nil {
		return nil, err
	}
	client := codersdk.New(serverURL)
	root := config.Root(cfg.configDir)
	if token, err := root.Session().Read(); err == nil && token != "" {
		client.SetSessionToken(token)
		if _, err := client.User(ctx, codersdk.Me); err == nil {
			if err := root.URL().Write(cfg.coderURL); err != nil {
				return nil, xerrors.Errorf("write cluster URL: %w", err)
			}
			return client, nil
		}
		client.SetSessionToken("")
	}
	hasUser, err := client.HasFirstUser(ctx)
	if err != nil {
		return nil, xerrors.Errorf("check first user: %w", err)
	}
	if !hasUser {
		if _, err := client.CreateFirstUser(ctx, codersdk.CreateFirstUserRequest{
			Email: "admin@coder.com", Username: "admin", Name: "Admin User", Password: cfg.password,
		}); err != nil {
			return nil, xerrors.Errorf("create first user: %w", err)
		}
	}
	login, err := client.LoginWithPassword(ctx, codersdk.LoginWithPasswordRequest{Email: "admin@coder.com", Password: cfg.password})
	if err != nil {
		return nil, xerrors.Errorf("log in as admin: %w", err)
	}
	client.SetSessionToken(login.SessionToken)
	if err := root.Session().Write(login.SessionToken); err != nil {
		return nil, xerrors.Errorf("write cluster session: %w", err)
	}
	if err := root.URL().Write(cfg.coderURL); err != nil {
		return nil, xerrors.Errorf("write cluster URL: %w", err)
	}
	return client, nil
}

func premiumReady(ctx context.Context, client *codersdk.Client) (bool, []codersdk.FeatureName, error) {
	entitlements, err := client.Entitlements(ctx)
	if err != nil {
		return false, nil, xerrors.Errorf("read entitlements: %w", err)
	}
	var missing []codersdk.FeatureName
	for _, name := range []codersdk.FeatureName{codersdk.FeatureAIBridge, codersdk.FeatureExternalProvisionerDaemons} {
		feature, ok := entitlements.Features[name]
		if !ok || !feature.Enabled || !feature.Entitlement.Entitled() {
			missing = append(missing, name)
		}
	}
	return len(missing) == 0, missing, nil
}

func (cfg *clusterConfig) ensureComponentSecrets(ctx context.Context, client *codersdk.Client) error {
	org, err := client.OrganizationByName(ctx, codersdk.DefaultOrganization)
	if err != nil {
		return xerrors.Errorf("find default organization: %w", err)
	}
	if !cfg.secretExists(ctx, provisionerKeyName) {
		key, err := client.CreateProvisionerKey(ctx, org.ID, codersdk.CreateProvisionerKeyRequest{Name: cfg.clusterName})
		if err != nil {
			return xerrors.Errorf("create provisioner key: %w", err)
		}
		if err := cfg.applyStringSecret(ctx, provisionerKeyName, "key", key.Key); err != nil {
			return err
		}
	}
	if !cfg.secretExists(ctx, gatewayKeyName) {
		key, err := client.CreateAIGatewayKey(ctx, codersdk.CreateAIGatewayKeyRequest{Name: cfg.clusterName})
		if err != nil {
			return xerrors.Errorf("create AI Gateway key: %w", err)
		}
		if err := cfg.applyStringSecret(ctx, gatewayKeyName, "key", key.Key); err != nil {
			return err
		}
	}
	return nil
}

func (cfg *clusterConfig) applyStringSecret(ctx context.Context, name, key, value string) error {
	manifest := fmt.Sprintf("apiVersion: v1\nkind: Secret\nmetadata:\n  name: %s\n  namespace: %s\ntype: Opaque\ndata:\n  %s: %s\n", name, cfg.namespace, key, base64.StdEncoding.EncodeToString([]byte(value)))
	return cfg.run(ctx, strings.NewReader(manifest), "kubectl", "--context", cfg.context, "apply", "-f", "-")
}

func (cfg *clusterConfig) waitForProvisioner(ctx context.Context, client *codersdk.Client) error {
	org, err := client.OrganizationByName(ctx, codersdk.DefaultOrganization)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(healthTimeout)
	for time.Now().Before(deadline) {
		keys, err := client.ListProvisionerKeyDaemons(ctx, org.ID)
		if err != nil {
			return xerrors.Errorf("list provisioner daemons: %w", err)
		}
		for _, key := range keys {
			if key.Key.Name == cfg.clusterName && len(key.Daemons) > 0 {
				return nil
			}
		}
		if err := sleep(ctx, time.Second); err != nil {
			return err
		}
	}
	return xerrors.Errorf("provisioner did not connect within %s", healthTimeout)
}

func (cfg *clusterConfig) setupStarterTemplate(ctx context.Context, client *codersdk.Client) error {
	if cfg.starterTemplate == "" {
		return nil
	}
	org, err := client.OrganizationByName(ctx, codersdk.DefaultOrganization)
	if err != nil {
		return err
	}
	if _, err := client.TemplateByName(ctx, org.ID, cfg.starterTemplate); err == nil {
		return nil
	} else if coderError, ok := codersdk.AsError(err); !ok || coderError.StatusCode() != http.StatusNotFound {
		return xerrors.Errorf("look up starter template: %w", err)
	}
	examples, err := client.StarterTemplates(ctx)
	if err != nil {
		return err
	}
	example, ok := slice.Find(examples, func(example codersdk.TemplateExample) bool { return example.ID == cfg.starterTemplate })
	if !ok {
		return xerrors.Errorf("starter template %q not found", cfg.starterTemplate)
	}
	version, err := client.CreateTemplateVersion(ctx, org.ID, codersdk.CreateTemplateVersionRequest{
		StorageMethod: codersdk.ProvisionerStorageMethodFile,
		ExampleID:     example.ID,
		Provisioner:   codersdk.ProvisionerTypeTerraform,
		UserVariableValues: []codersdk.VariableValue{
			{Name: "namespace", Value: cfg.workspaceNamespace},
			{Name: "use_kubeconfig", Value: "false"},
		},
	})
	if err != nil {
		return xerrors.Errorf("create Kubernetes starter version: %w", err)
	}
	if _, err := waitForTemplateVersion(ctx, client, version.ID); err != nil {
		return xerrors.Errorf("provision Kubernetes starter template: %w", err)
	}
	_, err = client.CreateTemplate(ctx, org.ID, codersdk.CreateTemplateRequest{
		Name: cfg.starterTemplate, DisplayName: example.Name, Description: example.Description, Icon: example.Icon, VersionID: version.ID,
	})
	return err
}

func waitForTemplateVersion(ctx context.Context, client *codersdk.Client, id uuid.UUID) (codersdk.TemplateVersion, error) {
	deadline := time.Now().Add(healthTimeout)
	for time.Now().Before(deadline) {
		version, err := client.TemplateVersion(ctx, id)
		if err != nil {
			return version, err
		}
		switch version.Job.Status {
		case codersdk.ProvisionerJobSucceeded:
			return version, nil
		case codersdk.ProvisionerJobFailed:
			return version, xerrors.Errorf("template version failed: %s", version.Job.Error)
		case codersdk.ProvisionerJobCanceled:
			return version, xerrors.New("template version was canceled")
		}
		if err := sleep(ctx, time.Second); err != nil {
			return version, err
		}
	}
	return codersdk.TemplateVersion{}, xerrors.Errorf("template version did not finish within %s", healthTimeout)
}

func (cfg *clusterConfig) runCoder(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if !cfg.clusterExists(ctx) {
		return xerrors.Errorf("kind cluster %q does not exist, run up first", cfg.clusterName)
	}
	if err := waitForHTTP(ctx, cfg.coderURL+"/healthz", 10*time.Second, "Coder"); err != nil {
		return err
	}
	if err := cfg.buildCoderBinary(ctx, runtime.GOOS, runtime.GOARCH); err != nil {
		return err
	}
	commandArgs := append([]string{"--global-config", cfg.configDir}, args...)
	return cfg.runWithStreams(ctx, stdin, cfg.binaryPath, commandArgs, stdout, stderr)
}

func (cfg *clusterConfig) run(ctx context.Context, stdin io.Reader, command string, args ...string) error {
	return cfg.runWithStreams(ctx, stdin, command, args, os.Stdout, os.Stderr)
}

func (cfg *clusterConfig) runWithStreams(ctx context.Context, stdin io.Reader, command string, args []string, stdout, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = cfg.projectRoot
	cmd.Env = cfg.childEnv
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return xerrors.Errorf("%s failed: %w", printableCommand(command, args), err)
	}
	return nil
}

func (cfg *clusterConfig) output(ctx context.Context, command string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = cfg.projectRoot
	cmd.Env = cfg.childEnv
	output, err := cmd.Output()
	if err != nil {
		return "", xerrors.Errorf("%s failed: %w", printableCommand(command, args), err)
	}
	return strings.TrimSpace(string(output)), nil
}

func (cfg *clusterConfig) clusterExists(ctx context.Context) bool {
	clusters, err := cfg.kindClusters(ctx)
	return err == nil && contains(clusters, cfg.clusterName)
}

func (cfg *clusterConfig) kindClusters(ctx context.Context) ([]string, error) {
	output, err := cfg.output(ctx, "kind", "get", "clusters")
	if err != nil {
		return nil, xerrors.Errorf("list kind clusters: %w", err)
	}
	return strings.Fields(output), nil
}

func (cfg *clusterConfig) releaseExists(ctx context.Context, release string) bool {
	return cfg.commandSucceeds(ctx, "helm", "status", release, "--namespace", cfg.namespace, "--kube-context", cfg.context)
}

func (cfg *clusterConfig) secretExists(ctx context.Context, name string) bool {
	return cfg.commandSucceeds(ctx, "kubectl", "--context", cfg.context, "--namespace", cfg.namespace, "get", "secret", name)
}

func (cfg *clusterConfig) commandSucceeds(ctx context.Context, command string, args ...string) bool {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = cfg.projectRoot
	cmd.Env = cfg.childEnv
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run() == nil
}

func (cfg *clusterConfig) secretValue(ctx context.Context, name, key string) (string, error) {
	return cfg.output(ctx, "kubectl", "--context", cfg.context, "--namespace", cfg.namespace, "get", "secret", name, "-o", fmt.Sprintf("jsonpath={.data.%s}", key))
}

func (cfg *clusterConfig) dockerArch(ctx context.Context) (string, error) {
	arch, err := cfg.output(ctx, "docker", "version", "--format", "{{.Server.Arch}}")
	if err != nil {
		return "", err
	}
	switch arch {
	case "amd64", "x86_64":
		return "amd64", nil
	case "arm64", "aarch64":
		return "arm64", nil
	default:
		return "", xerrors.Errorf("unsupported Docker server architecture %q, supported architectures are amd64 and arm64", arch)
	}
}

func (cfg *clusterConfig) imageRepository() string {
	image := cfg.readCurrentImage()
	if image == "" {
		return cfg.imageRepo
	}
	repository, _, ok := strings.Cut(image, ":")
	if !ok {
		return image
	}
	return repository
}

func (cfg *clusterConfig) imageTag() string {
	image := cfg.readCurrentImage()
	_, tag, ok := strings.Cut(image, ":")
	if !ok {
		return ""
	}
	return tag
}

func (cfg *clusterConfig) readCurrentImage() string {
	if cfg.currentImage != "" {
		return cfg.currentImage
	}
	contents, err := os.ReadFile(filepath.Join(cfg.runtimeDir, "image"))
	if err != nil {
		return ""
	}
	cfg.currentImage = strings.TrimSpace(string(contents))
	return cfg.currentImage
}

func (cfg *clusterConfig) nodeHasImage(ctx context.Context, image string) bool {
	output, err := cfg.output(ctx, "docker", "exec", cfg.clusterName+"-control-plane", "crictl", "images", "--output", "json")
	if err != nil {
		return false
	}
	var images struct {
		Images []struct {
			RepoTags []string `json:"repoTags"`
		} `json:"images"`
	}
	if err := json.Unmarshal([]byte(output), &images); err != nil {
		return false
	}
	for _, nodeImage := range images.Images {
		for _, tag := range nodeImage.RepoTags {
			if sameImageReference(tag, image) {
				return true
			}
		}
	}
	return false
}

func sameImageReference(left, right string) bool {
	return strings.TrimPrefix(left, "docker.io/") == strings.TrimPrefix(right, "docker.io/")
}

func (cfg *clusterConfig) hostImageExists(ctx context.Context, image string) bool {
	cmd := exec.CommandContext(ctx, "docker", "image", "inspect", image)
	cmd.Dir = cfg.projectRoot
	cmd.Env = cfg.childEnv
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run() == nil
}

func (cfg *clusterConfig) cleanNodeImages(ctx context.Context) error {
	node := cfg.clusterName + "-control-plane"
	imagesJSON, err := cfg.output(ctx, "docker", "exec", node, "crictl", "images", "--output", "json")
	if err != nil {
		return err
	}
	containersJSON, err := cfg.output(ctx, "docker", "exec", node, "crictl", "ps", "--all", "--output", "json")
	if err != nil {
		return err
	}

	var images struct {
		Images []struct {
			ID       string   `json:"id"`
			RepoTags []string `json:"repoTags"`
		} `json:"images"`
	}
	if err := json.Unmarshal([]byte(imagesJSON), &images); err != nil {
		return xerrors.Errorf("decode node images: %w", err)
	}
	var containers struct {
		Containers []struct {
			ImageRef string `json:"imageRef"`
		} `json:"containers"`
	}
	if err := json.Unmarshal([]byte(containersJSON), &containers); err != nil {
		return xerrors.Errorf("decode node containers: %w", err)
	}
	inUse := make(map[string]struct{}, len(containers.Containers))
	for _, container := range containers.Containers {
		inUse[container.ImageRef] = struct{}{}
	}
	for _, image := range images.Images {
		if _, used := inUse[image.ID]; used || !hasImageRepository(image.RepoTags, cfg.imageRepo) {
			continue
		}
		if err := cfg.run(ctx, nil, "docker", "exec", node, "crictl", "rmi", image.ID); err != nil {
			return err
		}
	}
	return nil
}

func hasImageRepository(tags []string, repository string) bool {
	for _, tag := range tags {
		if strings.HasPrefix(strings.TrimPrefix(tag, "docker.io/"), repository+":") {
			return true
		}
	}
	return false
}

func (cfg *clusterConfig) removeHostImages(ctx context.Context) error {
	inUse, err := cfg.hostImagesInUse(ctx)
	if err != nil {
		return err
	}
	output, err := cfg.output(ctx, "docker", "images", "--format", "{{.Repository}}:{{.Tag}}", cfg.imageRepo)
	if err != nil {
		return err
	}
	for image := range strings.Lines(output) {
		if image == "" || image == "<none>:<none>" {
			continue
		}
		if _, used := inUse[image]; used {
			continue
		}
		if err := cfg.run(ctx, nil, "docker", "image", "rm", image); err != nil {
			return err
		}
	}
	return nil
}

func (cfg *clusterConfig) hostImagesInUse(ctx context.Context) (map[string]struct{}, error) {
	output, err := cfg.output(ctx, "docker", "ps", "--format", "{{.Image}}")
	if err != nil {
		return nil, err
	}
	images := make(map[string]struct{})
	for image := range strings.Lines(output) {
		if image != "" {
			images[image] = struct{}{}
		}
	}
	return images, nil
}

func (cfg *clusterConfig) coderHealthy(ctx context.Context) bool {
	healthContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(healthContext, http.MethodGet, cfg.coderURL+"/healthz", nil)
	if err != nil {
		return false
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	return response.StatusCode == http.StatusOK
}

func waitForHTTP(ctx context.Context, endpoint string, timeout time.Duration, name string) error {
	waitContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		request, err := http.NewRequestWithContext(waitContext, http.MethodGet, endpoint, nil)
		if err == nil {
			response, requestErr := http.DefaultClient.Do(request)
			if requestErr == nil {
				_ = response.Body.Close()
				if response.StatusCode == http.StatusOK {
					return nil
				}
			}
		}
		if err := sleep(waitContext, time.Second); err != nil {
			return xerrors.Errorf("%s did not become ready at %s within %s", name, endpoint, timeout)
		}
	}
}

func (cfg *clusterConfig) shouldPrompt(inv *serpent.Invocation) bool {
	if cfg.noLicensePrompt {
		return false
	}
	file, ok := inv.Stdin.(*os.File)
	return ok && isatty.IsTerminal(file.Fd())
}

func (cfg *clusterConfig) printLicenseNextStep(out io.Writer, missing []codersdk.FeatureName) {
	features := make([]string, 0, len(missing))
	for _, feature := range missing {
		features = append(features, feature.Humanize())
	}
	_, _ = fmt.Fprintf(out, "Coder is running, but %s require a license.\nAdd a license, then rerun up:\n  ./scripts/develop-local-cluster.sh coder licenses add -f <path>\nCoder: %s\nContext: %s\n", strings.Join(features, ", "), cfg.coderURL, cfg.context)
}

func (cfg *clusterConfig) printBanner(out io.Writer, stage string) {
	_, _ = fmt.Fprintf(out, "\nCoder local cluster is %s.\nCoder: %s\nAI Gateway: %s\nCluster: %s\nContext: %s\nImage: %s\n\nReload code: ./scripts/develop-local-cluster.sh reload\nReload charts: ./scripts/develop-local-cluster.sh reload --charts-only\nRemove everything: ./scripts/develop-local-cluster.sh down\n", stage, cfg.coderURL, cfg.gatewayURL, cfg.clusterName, cfg.context, cfg.readCurrentImage())
}

func findProjectRoot() (string, error) {
	directory, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil {
			return directory, nil
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", xerrors.New("could not find the repository root")
		}
		directory = parent
	}
}

func randomText(bytesLength int) (string, error) {
	bytes := make([]byte, bytesLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func shortHash(input string) string {
	hash := uint32(2166136261)
	for _, byte := range []byte(input) {
		hash ^= uint32(byte)
		hash *= 16777619
	}
	return fmt.Sprintf("%08x", hash)
}

var clusterNameRegexp = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

func validClusterName(name string) bool {
	return len(name) <= 63 && clusterNameRegexp.MatchString(name)
}

func validatePort(port int64, name string) error {
	if port < 1 || port > 65535 {
		return xerrors.Errorf("%s must be between 1 and 65535", name)
	}
	return nil
}

func portBusy(port int64) bool {
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return true
	}
	_ = listener.Close()
	return false
}

func filterEnv(env []string, excluded ...string) []string {
	result := make([]string, 0, len(env))
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		if !contains(excluded, key) {
			result = append(result, entry)
		}
	}
	return result
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func emptyAs(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func printableCommand(command string, args []string) string {
	return command + " " + strings.Join(args, " ")
}

func sleep(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
