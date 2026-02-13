package catalog

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"

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

// Provisioner runs external provisioner daemons via docker compose.
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

	// Register and start provisioner containers via compose.
	dkr, ok := cat.MustGet(OnDocker()).(*Docker)
	if !ok {
		return xerrors.New("unexpected type for Docker service")
	}
	coderd, ok := cat.MustGet(OnCoderd()).(*Coderd)
	if !ok {
		return xerrors.New("unexpected type for Coderd service")
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

	_ = coderd.Result() // ensure dep is used

	p.setStep("Starting provisioner daemons")
	var serviceNames []string
	for i := range p.count {
		index := int(i)
		name := fmt.Sprintf("provisioner-%d", index)
		serviceNames = append(serviceNames, name)

		logger.Info(ctx, "registering provisioner compose service", slog.F("index", index))

		dkr.SetCompose(name, ComposeService{
			Image:      dogfoodImage + ":" + dogfoodTag,
			Networks:   []string{composeNetworkName},
			WorkingDir: "/app",
			Environment: map[string]string{
				"CODER_URL":                    "http://coderd-0:3000",
				"CODER_PROVISIONER_DAEMON_KEY":  secret,
				"CODER_PROVISIONER_DAEMON_NAME": fmt.Sprintf("cdev-provisioner-%d", index),
				"GOMODCACHE":                   "/go-cache/mod",
				"GOCACHE":                      "/go-cache/build",
				"CODER_CACHE_DIRECTORY":        "/cache",
				"DOCKER_HOST":                  fmt.Sprintf("unix://%s", dockerSocket),
			},
			Command: []string{
				"go", "run", "./enterprise/cmd/coder",
				"provisioner", "start",
				"--verbose",
			},
			Volumes: []string{
				fmt.Sprintf("%s:/app", cwd),
				"go_cache:/go-cache",
				"coder_cache:/cache",
				fmt.Sprintf("%s:%s", dockerSocket, dockerSocket),
			},
			GroupAdd: []string{dockerGroup},
			DependsOn: map[string]ComposeDependsOn{
				"coderd-0": {Condition: "service_healthy"},
			},
			Labels: composeServiceLabels("provisioner"),
		})
	}

	if err := dkr.DockerComposeUp(ctx, serviceNames...); err != nil {
		return xerrors.Errorf("docker compose up provisioners: %w", err)
	}

	return nil
}

func (*Provisioner) Stop(_ context.Context) error {
	return nil
}

func (p *Provisioner) Result() ProvisionerResult {
	return p.result
}
