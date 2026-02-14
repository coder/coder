package catalog

import (
	"fmt"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Compose file types that marshal to valid docker-compose YAML.

// ComposeFile represents a docker-compose.yml file.
type ComposeFile struct {
	Services map[string]ComposeService `yaml:"services"`
	Volumes  map[string]ComposeVolume  `yaml:"volumes,omitempty"`
	Networks map[string]ComposeNetwork `yaml:"networks,omitempty"`

	cfg ComposeConfig `yaml:"-"`
}

// NewComposeFile creates a new ComposeFile with initialized maps and
// the given config stored for use by builder methods.
func NewComposeFile(cfg ComposeConfig) *ComposeFile {
	return &ComposeFile{
		Services: make(map[string]ComposeService),
		Volumes:  make(map[string]ComposeVolume),
		Networks: map[string]ComposeNetwork{
			composeNetworkName: {Driver: "bridge"},
		},
		cfg: cfg,
	}
}

// ComposeService represents a single service in a compose file.
type ComposeService struct {
	Image       string                      `yaml:"image,omitempty"`
	Build       *ComposeBuild               `yaml:"build,omitempty"`
	Command     any                         `yaml:"command,omitempty"`
	Entrypoint  any                         `yaml:"entrypoint,omitempty"`
	Environment map[string]string           `yaml:"environment,omitempty"`
	Ports       []string                    `yaml:"ports,omitempty"`
	Volumes     []string                    `yaml:"volumes,omitempty"`
	DependsOn   map[string]ComposeDependsOn `yaml:"depends_on,omitempty"`
	Networks    []string                    `yaml:"networks,omitempty"`
	NetworkMode string                      `yaml:"network_mode,omitempty"`
	WorkingDir  string                      `yaml:"working_dir,omitempty"`
	Labels      []string                    `yaml:"labels,omitempty"`
	GroupAdd    []string                    `yaml:"group_add,omitempty"`
	User        string                      `yaml:"user,omitempty"`
	Restart     string                      `yaml:"restart,omitempty"`
	Healthcheck *ComposeHealthcheck         `yaml:"healthcheck,omitempty"`
}

// ComposeBuild represents build configuration for a service.
type ComposeBuild struct {
	Context    string `yaml:"context"`
	Dockerfile string `yaml:"dockerfile,omitempty"`
}

// ComposeDependsOn represents a dependency condition.
type ComposeDependsOn struct {
	Condition string `yaml:"condition"`
}

// ComposeHealthcheck represents a healthcheck configuration.
type ComposeHealthcheck struct {
	Test        []string `yaml:"test"`
	Interval    string   `yaml:"interval,omitempty"`
	Timeout     string   `yaml:"timeout,omitempty"`
	Retries     int      `yaml:"retries,omitempty"`
	StartPeriod string   `yaml:"start_period,omitempty"`
}

// ComposeVolume represents a named volume declaration.
type ComposeVolume struct{}

// ComposeNetwork represents a network declaration.
type ComposeNetwork struct {
	Driver string `yaml:"driver,omitempty"`
}

const (
	composeNetworkName = "coder-dev"
	composeDogfood     = "codercom/oss-dogfood:latest"
)

// ComposeConfig holds the configuration for generating a compose file.
type ComposeConfig struct {
	CoderdCount      int
	ProvisionerCount int
	OIDC             bool
	Prometheus       bool
	DockerGroup      string
	DockerSocket     string
	CWD              string
	License          string
}

func composeServiceLabels(service string) []string {
	return []string{
		CDevLabel + "=true",
		CDevService + "=" + service,
	}
}

// Generate builds the full ComposeFile from the given config.
func Generate(cfg ComposeConfig) *ComposeFile {
	if cfg.CoderdCount < 1 {
		cfg.CoderdCount = 1
	}

	cf := NewComposeFile(cfg)
	cf.AddDatabase().AddInitVolumes().AddBuildSlim()
	for i := range cfg.CoderdCount {
		cf.AddCoderd(i)
	}
	if cfg.OIDC {
		cf.AddOIDC()
	}
	cf.AddSite()
	for i := range cfg.ProvisionerCount {
		cf.AddProvisioner(i)
	}
	if cfg.Prometheus {
		cf.AddPrometheus()
	}
	cf.AddLoadBalancer(cfg.CoderdCount)

	return cf
}
// AddLoadBalancer adds the nginx load balancer service that fronts
// all cdev services with separate listeners per service.
func (cf *ComposeFile) AddLoadBalancer(haCount int) *ComposeFile {
	cfg := cf.cfg
	if haCount < 1 {
		haCount = 1
	}

	nginxConf := filepath.Join(cfg.CWD, ".cdev-lb", "nginx.conf")

	var ports []string
	addPort := func(port int) {
		ports = append(ports, fmt.Sprintf("%d:%d", port, port))
	}

	// Load-balanced coderd.
	addPort(coderdBasePort)
	// Individual coderd instances (3001..3000+N).
	for i := range haCount {
		addPort(coderdBasePort + 1 + i)
	}
	// pprof per instance.
	for i := range haCount {
		addPort(pprofBasePort + i)
	}
	// Metrics per instance.
	for i := range haCount {
		addPort(prometheusBasePort + i)
	}
	// OIDC.
	addPort(oidcPort)
	// Prometheus UI.
	addPort(prometheusUIPort2)
	// Site dev server.
	addPort(sitePort)

	cf.Services["load-balancer"] = ComposeService{
		Image:    nginxImage + ":" + nginxTag,
		Volumes:  []string{nginxConf + ":/etc/nginx/nginx.conf:ro"},
		Ports:    ports,
		Networks: []string{composeNetworkName},
		Labels: composeServiceLabels("load-balancer"),
	}
	return cf
}

// GenerateYAML generates the compose YAML bytes from the given config.
func GenerateYAML(cfg ComposeConfig) ([]byte, error) {
	cf := Generate(cfg)
	return yaml.Marshal(cf)
}

// AddDatabase adds the PostgreSQL database service.
func (cf *ComposeFile) AddDatabase() *ComposeFile {
	cf.Volumes["coder_dev_data"] = ComposeVolume{}
	cf.Services["database"] = ComposeService{
		Image: "postgres:17",
		Environment: map[string]string{
			"POSTGRES_USER":     postgresUser,
			"POSTGRES_PASSWORD": postgresPassword,
			"POSTGRES_DB":       postgresDB,
		},
		Volumes:  []string{"coder_dev_data:/var/lib/postgresql/data"},
		Ports:    []string{"5432:5432"},
		Networks: []string{composeNetworkName},
		Labels:   composeServiceLabels("database"),
		Healthcheck: &ComposeHealthcheck{
			Test:     []string{"CMD-SHELL", "pg_isready -U coder"},
			Interval: "2s",
			Timeout:  "5s",
			Retries:  10,
		},
	}
	return cf
}

// AddInitVolumes adds the volume initialization service.
func (cf *ComposeFile) AddInitVolumes() *ComposeFile {
	cf.Volumes["go_cache"] = ComposeVolume{}
	cf.Volumes["coder_cache"] = ComposeVolume{}
	cf.Volumes["site_node_modules"] = ComposeVolume{}

	cf.Services["init-volumes"] = ComposeService{
		Image: composeDogfood,
		User:  "0:0",
		Volumes: []string{
			"go_cache:/go-cache",
			"coder_cache:/cache",
			"site_node_modules:/app/site/node_modules",
		},
		Command: "chown -R 1000:1000 /go-cache /cache /app/site/node_modules",
		Labels:  composeServiceLabels("init-volumes"),
	}
	return cf
}

// AddBuildSlim adds the slim binary build service.
func (cf *ComposeFile) AddBuildSlim() *ComposeFile {
	cfg := cf.cfg
	cf.Services["build-slim"] = ComposeService{
		Image:       composeDogfood,
		NetworkMode: "host",
		WorkingDir:  "/app",
		GroupAdd:    []string{cfg.DockerGroup},
		Environment: map[string]string{
			"GOMODCACHE":  "/go-cache/mod",
			"GOCACHE":     "/go-cache/build",
			"DOCKER_HOST": fmt.Sprintf("unix://%s", cfg.DockerSocket),
		},
		Volumes: []string{
			fmt.Sprintf("%s:/app", cfg.CWD),
			"go_cache:/go-cache",
			"coder_cache:/cache",
			fmt.Sprintf("%s:/var/run/docker.sock", cfg.DockerSocket),
		},
		Command: `sh -c 'make -j build-slim && mkdir -p /cache/site/orig/bin && cp site/out/bin/coder-* /cache/site/orig/bin/ 2>/dev/null || true && echo "Slim binaries built and cached."'`,
		DependsOn: map[string]ComposeDependsOn{
			"init-volumes": {Condition: "service_completed_successfully"},
			"database":     {Condition: "service_healthy"},
		},
		Labels: composeServiceLabels("build-slim"),
	}
	return cf
}

// AddCoderd adds a coderd service instance at the given index.
func (cf *ComposeFile) AddCoderd(index int) *ComposeFile {
	cfg := cf.cfg
	name := fmt.Sprintf("coderd-%d", index)
	hostPort := 3000 + index
	pprofPort := 6060 + index
	promPort := 2112 + index
	volName := fmt.Sprintf("coderv2_config_%d", index)
	cf.Volumes[volName] = ComposeVolume{}

	pgURL := "postgresql://coder:coder@database:5432/coder?sslmode=disable" //nolint:gosec // G101: Dev-only postgres credentials.
	accessURL := fmt.Sprintf("http://localhost:%d", hostPort)

	env := map[string]string{
		"CODER_PG_CONNECTION_URL":             pgURL,
		"CODER_HTTP_ADDRESS":                  "0.0.0.0:3000",
		"CODER_ACCESS_URL":                    accessURL,
		"CODER_SWAGGER_ENABLE":                "true",
		"CODER_DANGEROUS_ALLOW_CORS_REQUESTS": "true",
		"CODER_TELEMETRY_ENABLE":              "false",
		"GOMODCACHE":                          "/go-cache/mod",
		"GOCACHE":                             "/go-cache/build",
		"CODER_CACHE_DIRECTORY":               "/cache",
		"DOCKER_HOST":                         "unix:///var/run/docker.sock",
		"CODER_PPROF_ENABLE":                  "true",
		"CODER_PPROF_ADDRESS":                 "0.0.0.0:6060",
		"CODER_PROMETHEUS_ENABLE":             "true",
		"CODER_PROMETHEUS_ADDRESS":            "0.0.0.0:2112",
	}
	if cfg.ProvisionerCount > 0 {
		env["CODER_PROVISIONER_DAEMONS"] = "0"
	}
	if cfg.License != "" {
		env["CODER_LICENSE"] = cfg.License
	}

	cmd := []string{
		"go", "run", "./enterprise/cmd/coder", "server",
		"--http-address", "0.0.0.0:3000",
		"--access-url", accessURL,
		"--swagger-enable",
		"--dangerous-allow-cors-requests=true",
		"--enable-terraform-debug-mode",
		"--pprof-enable",
		"--pprof-address", "0.0.0.0:6060",
		"--prometheus-enable",
		"--prometheus-address", "0.0.0.0:2112",
	}
	if cfg.OIDC {
		cmd = append(cmd,
			"--oidc-issuer-url", "http://oidc:4500",
			"--oidc-client-id", "static-client-id",
			"--oidc-client-secret", "static-client-secret",
		)
	}

	depends := map[string]ComposeDependsOn{
		"database":   {Condition: "service_healthy"},
		"build-slim": {Condition: "service_completed_successfully"},
	}
	if cfg.OIDC {
		depends["oidc"] = ComposeDependsOn{Condition: "service_healthy"}
	}

	cf.Services[name] = ComposeService{
		Image:       composeDogfood,
		WorkingDir:  "/app",
		Networks:    []string{composeNetworkName},
		GroupAdd:    []string{cfg.DockerGroup},
		Environment: env,
		Command:     cmd,
		Ports: []string{
			fmt.Sprintf("%d:3000", hostPort),
			fmt.Sprintf("%d:6060", pprofPort),
			fmt.Sprintf("%d:2112", promPort),
		},
		Volumes: []string{
			fmt.Sprintf("%s:/app", cfg.CWD),
			"go_cache:/go-cache",
			"coder_cache:/cache",
			fmt.Sprintf("%s:/home/coder/.config/coderv2", volName),
			fmt.Sprintf("%s:/var/run/docker.sock", cfg.DockerSocket),
		},
		DependsOn: depends,
		Restart:   "unless-stopped",
		Labels:    composeServiceLabels("coderd"),
		Healthcheck: &ComposeHealthcheck{
			Test:        []string{"CMD-SHELL", "curl -sf http://localhost:3000/api/v2/buildinfo || exit 1"},
			Interval:    "5s",
			Timeout:     "5s",
			Retries:     60,
			StartPeriod: "120s",
		},
	}
	return cf
}

// AddOIDC adds the OIDC test identity provider service.
func (cf *ComposeFile) AddOIDC() *ComposeFile {
	cf.Services["oidc"] = ComposeService{
		Build: &ComposeBuild{
			Context:    ".",
			Dockerfile: "scripts/testidp/Dockerfile.testidp",
		},
		Ports:    []string{"4500:4500"},
		Networks: []string{composeNetworkName},
		Command:  "-client-id static-client-id -client-sec static-client-secret -issuer http://oidc:4500",
		Labels:   composeServiceLabels("oidc"),
		Healthcheck: &ComposeHealthcheck{
			Test:     []string{"CMD-SHELL", "curl -sf http://localhost:4500/.well-known/openid-configuration || exit 1"},
			Interval: "2s",
			Timeout:  "5s",
			Retries:  15,
		},
	}
	return cf
}

// AddSite adds the frontend dev server service.
func (cf *ComposeFile) AddSite() *ComposeFile {
	cfg := cf.cfg
	cf.Services["site"] = ComposeService{
		Image:      composeDogfood,
		Networks:   []string{composeNetworkName},
		WorkingDir: "/app/site",
		Environment: map[string]string{
			"CODER_HOST": "http://coderd-0:3000",
		},
		Ports: []string{"8080:8080"},
		Volumes: []string{
			fmt.Sprintf("%s/site:/app/site", cfg.CWD),
			"site_node_modules:/app/site/node_modules",
		},
		Command: `sh -c "pnpm install --frozen-lockfile && pnpm dev --host"`,
		DependsOn: map[string]ComposeDependsOn{
			"coderd-0": {Condition: "service_healthy"},
		},
		Labels: composeServiceLabels("site"),
	}
	return cf
}

// AddProvisioner adds an external provisioner service at the given index.
func (cf *ComposeFile) AddProvisioner(index int) *ComposeFile {
	cfg := cf.cfg
	name := fmt.Sprintf("provisioner-%d", index)

	env := map[string]string{
		"CODER_URL":                    "http://coderd-0:3000",
		"GOMODCACHE":                   "/go-cache/mod",
		"GOCACHE":                      "/go-cache/build",
		"CODER_CACHE_DIRECTORY":        "/cache",
		"DOCKER_HOST":                  "unix:///var/run/docker.sock",
		"CODER_PROVISIONER_DAEMON_NAME": fmt.Sprintf("cdev-provisioner-%d", index),
	}

	cf.Services[name] = ComposeService{
		Image:       composeDogfood,
		Networks:    []string{composeNetworkName},
		WorkingDir:  "/app",
		Environment: env,
		Command:     []string{"go", "run", "./enterprise/cmd/coder", "provisioner", "start", "--verbose"},
		Volumes: []string{
			fmt.Sprintf("%s:/app", cfg.CWD),
			"go_cache:/go-cache",
			"coder_cache:/cache",
			fmt.Sprintf("%s:/var/run/docker.sock", cfg.DockerSocket),
		},
		GroupAdd: []string{cfg.DockerGroup},
		DependsOn: map[string]ComposeDependsOn{
			"coderd-0": {Condition: "service_healthy"},
		},
		Labels: composeServiceLabels("provisioner"),
	}
	return cf
}

// AddPrometheus adds Prometheus monitoring services.
func (cf *ComposeFile) AddPrometheus() *ComposeFile {
	cfg := cf.cfg
	cf.Volumes["prometheus"] = ComposeVolume{}

	// Build scrape targets for all coderd instances.
	var targets []string
	for i := range cfg.CoderdCount {
		targets = append(targets, fmt.Sprintf("coderd-%d:2112", i))
	}
	targetsStr := `"` + strings.Join(targets, `", "`) + `"`

	configScript := fmt.Sprintf(
		`mkdir -p /prom-vol/config /prom-vol/data && printf '%%s' 'global:
  scrape_interval: 15s

scrape_configs:
  - job_name: "coder"
    static_configs:
      - targets: [%s]
' > /prom-vol/config/prometheus.yml`, targetsStr)

	cf.Services["prometheus-init"] = ComposeService{
		Image:      "prom/prometheus:latest",
		Entrypoint: []string{"sh", "-c"},
		Command:    configScript,
		Volumes:    []string{"prometheus:/prom-vol"},
		Labels:     composeServiceLabels("prometheus-init"),
	}

	cf.Services["prometheus"] = ComposeService{
		Image: "prom/prometheus:latest",
		Command: []string{
			"--config.file=/prom-vol/config/prometheus.yml",
			"--storage.tsdb.path=/prom-vol/data",
			"--web.listen-address=0.0.0.0:9090",
		},
		Ports:    []string{"9090:9090"},
		Networks: []string{composeNetworkName},
		Volumes:  []string{"prometheus:/prom-vol"},
		DependsOn: map[string]ComposeDependsOn{
			"prometheus-init": {Condition: "service_completed_successfully"},
			"coderd-0":        {Condition: "service_healthy"},
		},
		Labels: composeServiceLabels("prometheus"),
		Healthcheck: &ComposeHealthcheck{
			Test:     []string{"CMD-SHELL", "curl -sf http://localhost:9090/-/ready || exit 1"},
			Interval: "2s",
			Timeout:  "5s",
			Retries:  15,
		},
	}
	return cf
}
