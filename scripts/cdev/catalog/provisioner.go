package catalog

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"

	"github.com/ory/dockertest/v3/docker"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/provisionerkey"
	"github.com/coder/serpent"

	_ "github.com/lib/pq" // Imported for postgres driver side effects.
)

// ProvisionerResult contains the provisioner key for connecting
// external provisioner daemons.
type ProvisionerResult struct {
	// Key is the plaintext provisioner key.
	Key string
}

var _ Service[ProvisionerResult] = (*Provisioner)(nil)
var _ ConfigurableService = (*Provisioner)(nil)

// OnProvisioner returns the service name for the provisioner service.
func OnProvisioner() ServiceName {
	return (&Provisioner{}).Name()
}

// Provisioner runs external provisioner daemons in Docker containers.
type Provisioner struct {
	currentStep atomic.Pointer[string]
	count       int64
	result      ProvisionerResult
}

func (p *Provisioner) CurrentStep() string {
	if s := p.currentStep.Load(); s != nil {
		return *s
	}
	return ""
}

func (p *Provisioner) setStep(step string) {
	p.currentStep.Store(&step)
}

// NewProvisioner creates a new Provisioner and registers a Configure
// callback to disable built-in provisioners on coderd when external
// provisioners are enabled.
func NewProvisioner(cat *Catalog) *Provisioner {
	p := &Provisioner{}
	Configure[*Coderd](cat, OnCoderd(), func(c *Coderd) {
		if p.count > 0 {
			// Fail fast: license is required for external provisioners.
			RequireLicense("external provisioners (--provisioner-count > 0)")
			c.ExtraEnv = append(c.ExtraEnv, "CODER_PROVISIONER_DAEMONS=0")
		}
	})
	return p
}

// Count returns the configured number of provisioner instances.
func (p *Provisioner) Count() int64 { return p.count }

func (*Provisioner) Name() ServiceName {
	return CDevProvisioner
}

func (*Provisioner) Emoji() string {
	return "⚙️"
}

func (*Provisioner) DependsOn() []ServiceName {
	return []ServiceName{OnCoderd()}
}

func (p *Provisioner) Options() serpent.OptionSet {
	return serpent.OptionSet{{
		Name:        "Provisioner Count",
		Description: "Number of external provisioner daemons to start. 0 disables (uses built-in).",
		Flag:        "provisioner-count",
		Env:         "CDEV_PROVISIONER_COUNT",
		Default:     "0",
		Value:       serpent.Int64Of(&p.count),
	}}
}

func (p *Provisioner) Start(ctx context.Context, logger slog.Logger, cat *Catalog) error {
	if p.count == 0 {
		return nil
	}
	defer p.setStep("")

	pg, ok := cat.MustGet(OnPostgres()).(*Postgres)
	if !ok {
		return xerrors.New("unexpected type for Postgres service")
	}

	// Ensure license is in the database before provisioner setup.
	if err := EnsureLicense(ctx, logger, cat); err != nil {
		return xerrors.Errorf("ensure license: %w", err)
	}

	// Open direct DB connection to create the provisioner key.
	sqlDB, err := pg.sqlDB()
	if err != nil {
		return xerrors.Errorf("open database: %w", err)
	}
	defer sqlDB.Close()

	store := database.New(sqlDB)

	// Get default organization.
	org, err := store.GetDefaultOrganization(ctx)
	if err != nil {
		return xerrors.Errorf("get default organization: %w", err)
	}

	// Generate provisioner key.
	params, secret, err := provisionerkey.New(org.ID, "cdev-external", nil)
	if err != nil {
		return xerrors.Errorf("generate provisioner key: %w", err)
	}

	// Upsert: delete existing, then insert fresh.
	existing, err := store.GetProvisionerKeyByName(ctx, database.GetProvisionerKeyByNameParams{
		OrganizationID: org.ID,
		Name:           "cdev-external",
	})
	if err == nil {
		_ = store.DeleteProvisionerKey(ctx, existing.ID)
	}
	_, err = store.InsertProvisionerKey(ctx, params)
	if err != nil {
		return xerrors.Errorf("insert provisioner key: %w", err)
	}

	p.result = ProvisionerResult{Key: secret}
	logger.Info(ctx, "provisioner key created", slog.F("name", "cdev-external"))

	// Start provisioner containers.
	p.setStep("Starting provisioner daemon")
	for i := range p.count {
		if err := p.startProvisioner(ctx, logger, cat, int(i), secret); err != nil {
			return xerrors.Errorf("start provisioner %d: %w", i, err)
		}
	}

	return nil
}

func (p *Provisioner) startProvisioner(ctx context.Context, logger slog.Logger, cat *Catalog, index int, key string) error {
	dkr, ok := cat.MustGet(OnDocker()).(*Docker)
	if !ok {
		return xerrors.New("unexpected type for Docker service")
	}
	pool := dkr.Result()
	_ = cat.MustGet(OnCoderd()).(*Coderd)
	build := Get[*BuildSlim](cat)

	labels := NewServiceLabels(CDevProvisioner)

	networkID, err := dkr.EnsureNetwork(ctx, labels)
	if err != nil {
		return xerrors.Errorf("ensure network: %w", err)
	}

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

	logger.Info(ctx, "starting provisioner container", slog.F("index", index))

	cntSink := NewLoggerSink(cat.w, p)
	cntLogger := slog.Make(cntSink)
	defer cntSink.Close()

	_, err = RunContainer(ctx, pool, CDevProvisioner, ContainerRunOptions{
		CreateOpts: docker.CreateContainerOptions{
			Name: fmt.Sprintf("cdev_provisioner_%d", index),
			Config: &docker.Config{
				Image:      dogfoodImage + ":" + dogfoodTag,
				WorkingDir: "/app",
				Env: []string{
					"CODER_URL=http://load-balancer:3000",
					fmt.Sprintf("CODER_PROVISIONER_DAEMON_KEY=%s", key),
					fmt.Sprintf("CODER_PROVISIONER_DAEMON_NAME=cdev-provisioner-%d", index),
					"GOMODCACHE=/go-cache/mod",
					"GOCACHE=/go-cache/build",
					"CODER_CACHE_DIRECTORY=/cache",
					fmt.Sprintf("DOCKER_HOST=unix://%s", dockerSocket),
				},
				Cmd: []string{
					"go", "run", "./enterprise/cmd/coder",
					"provisioner", "start",
					"--verbose",
				},
				Labels: labels,
			},
			HostConfig: &docker.HostConfig{
				Binds: []string{
					fmt.Sprintf("%s:/app", cwd),
					fmt.Sprintf("%s:/go-cache", build.GoCache.Name),
					fmt.Sprintf("%s:/cache", build.CoderCache.Name),
					fmt.Sprintf("%s:%s", dockerSocket, dockerSocket),
				},
				GroupAdd: []string{dockerGroup},
			},
			NetworkingConfig: &docker.NetworkingConfig{
				EndpointsConfig: map[string]*docker.EndpointConfig{
					CDevNetworkName: {
						NetworkID: networkID,
						Aliases:   []string{fmt.Sprintf("provisioner-%d", index)},
					},
				},
			},
		},
		Logger:   cntLogger,
		Detached: true,
	})
	if err != nil {
		return xerrors.Errorf("run provisioner container: %w", err)
	}

	return nil
}

func (*Provisioner) Stop(_ context.Context) error {
	return nil
}

func (p *Provisioner) Result() ProvisionerResult {
	return p.result
}
