package catalog

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	"github.com/ory/dockertest/v3/docker"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/provisionerkey"
	"github.com/coder/serpent"

	_ "github.com/lib/pq"
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
func OnProvisioner() string {
	return (&Provisioner{}).Name()
}

// Provisioner runs external provisioner daemons in Docker containers.
type Provisioner struct {
	count  int64
	result ProvisionerResult
}

// NewProvisioner creates a new Provisioner and registers a Configure
// callback to disable built-in provisioners on coderd when external
// provisioners are enabled.
func NewProvisioner(cat *Catalog) *Provisioner {
	p := &Provisioner{}
	Configure[*Coderd](cat, OnCoderd(), func(c *Coderd) {
		if p.count > 0 {
			c.ExtraEnv = append(c.ExtraEnv, "CODER_PROVISIONER_DAEMONS=0")
		}
	})
	return p
}

func (p *Provisioner) Name() string {
	return "provisioner"
}

func (p *Provisioner) Emoji() string {
	return "⚙️"
}

func (p *Provisioner) DependsOn() []string {
	return []string{OnCoderd()}
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

	pg := cat.MustGet(OnPostgres()).(*Postgres)

	// Open direct DB connection to create the provisioner key.
	sqlDB, err := sql.Open("postgres", pg.Result().URL)
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
	for i := range p.count {
		if err := p.startProvisioner(ctx, logger, cat, int(i), secret); err != nil {
			return xerrors.Errorf("start provisioner %d: %w", i, err)
		}
	}

	return nil
}

func (p *Provisioner) startProvisioner(ctx context.Context, logger slog.Logger, cat *Catalog, index int, key string) error {
	dkr := cat.MustGet(OnDocker()).(*Docker)
	pool := dkr.Result()
	coderd := cat.MustGet(OnCoderd()).(*Coderd)
	build := Get[*BuildSlim](cat)

	labels := NewServiceLabels(CDevProvisioner)

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
					fmt.Sprintf("CODER_URL=%s", coderd.Result().URL),
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
				GroupAdd:    []string{dockerGroup},
				NetworkMode: "host",
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

func (p *Provisioner) Stop(_ context.Context) error {
	return nil
}

func (p *Provisioner) Result() ProvisionerResult {
	return p.result
}
