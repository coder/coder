package catalog

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"text/template"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
)

const (
	nginxImage        = "nginx"
	nginxTag          = "alpine"
	oidcPort          = 4500
	prometheusUIPort2 = 9090
)

// LoadBalancerResult contains connection info for the running load
// balancer.
type LoadBalancerResult struct {
	// CoderdURL is the load-balanced coderd URL.
	CoderdURL string
}

var _ Service[LoadBalancerResult] = (*LoadBalancer)(nil)

// OnLoadBalancer returns the service name for the load balancer.
func OnLoadBalancer() ServiceName {
	return (&LoadBalancer{}).Name()
}

// LoadBalancer runs an nginx container that fronts all cdev services
// with separate listeners per service on sequential ports.
type LoadBalancer struct {
	currentStep atomic.Pointer[string]
	tmpDir      string
	result      LoadBalancerResult
}

// NewLoadBalancer creates a new LoadBalancer service.
func NewLoadBalancer() *LoadBalancer {
	return &LoadBalancer{}
}

func (lb *LoadBalancer) CurrentStep() string {
	if s := lb.currentStep.Load(); s != nil {
		return *s
	}
	return ""
}

func (lb *LoadBalancer) URL() string {
	return lb.result.CoderdURL
}

func (lb *LoadBalancer) setStep(step string) {
	lb.currentStep.Store(&step)
}

func (*LoadBalancer) Name() ServiceName {
	return CDevLoadBalancer
}

func (*LoadBalancer) Emoji() string {
	return "⚖️"
}

func (*LoadBalancer) DependsOn() []ServiceName {
	return []ServiceName{OnDocker()}
}

func (lb *LoadBalancer) Start(ctx context.Context, logger slog.Logger, cat *Catalog) error {
	defer lb.setStep("")

	dkr, ok := cat.MustGet(OnDocker()).(*Docker)
	if !ok {
		return xerrors.New("unexpected type for Docker service")
	}

	coderd, ok2 := cat.MustGet(OnCoderd()).(*Coderd)
	if !ok2 {
		return xerrors.New("unexpected type for Coderd service")
	}

	haCount := int(coderd.HACount())
	if haCount < 1 {
		haCount = 1
	}

	lb.setStep("generating nginx config")

	// Write nginx config under the current working directory so
	// Docker Desktop can access it (macOS /var/folders temp dirs
	// are not shared with the Docker VM by default).
	cwd, err := os.Getwd()
	if err != nil {
		return xerrors.Errorf("get working directory: %w", err)
	}
	tmpDir, err := os.MkdirTemp(cwd, ".cdev-lb-*")
	if err != nil {
		return xerrors.Errorf("create temp dir: %w", err)
	}
	lb.tmpDir = tmpDir

	nginxConf := generateNginxConfig(haCount)
	if err := os.WriteFile(filepath.Join(tmpDir, "nginx.conf"), []byte(nginxConf), 0o644); err != nil { //nolint:gosec // G306: nginx.conf must be readable by the container.
		return xerrors.Errorf("write nginx.conf: %w", err)
	}

	// Build port mappings for the compose service.
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

	lb.setStep("starting nginx container")
	logger.Info(ctx, "starting load balancer container", slog.F("ha_count", haCount))

	dkr.SetCompose("load-balancer", ComposeService{
		Image:    nginxImage + ":" + nginxTag,
		Volumes:  []string{filepath.Join(tmpDir, "nginx.conf") + ":/etc/nginx/nginx.conf:ro"},
		Ports:    ports,
		Networks: []string{composeNetworkName},
		Labels: composeServiceLabels("load-balancer"),
	})

	if err := dkr.DockerComposeUp(ctx, "load-balancer"); err != nil {
		return xerrors.Errorf("start load balancer container: %w", err)
	}

	lb.result = LoadBalancerResult{
		CoderdURL: fmt.Sprintf("http://localhost:%d", coderdBasePort),
	}

	logger.Info(ctx, "load balancer is ready",
		slog.F("coderd_url", lb.result.CoderdURL),
	)

	return nil
}

func (lb *LoadBalancer) Stop(_ context.Context) error {
	if lb.tmpDir != "" {
		_ = os.RemoveAll(lb.tmpDir)
		lb.tmpDir = ""
	}
	return nil
}

func (lb *LoadBalancer) Result() LoadBalancerResult {
	return lb.result
}

// nginxConfigData holds the data for rendering the nginx config
// template.
type nginxConfigData struct {
	HACount         int
	CoderdBasePort  int
	PprofBasePort   int
	MetricsBasePort int
	Instances       []int
}

//nolint:lll // Template content is inherently wide.
var nginxConfigTmpl = template.Must(template.New("nginx.conf").Funcs(template.FuncMap{
	"add": func(a, b int) int { return a + b },
	"pct": func(i, total int) string {
		if total <= 0 || i == total-1 {
			return "*"
		}
		return fmt.Sprintf("%.1f%%", float64(i+1)/float64(total)*100)
	},
}).Parse(`events {
    worker_connections 1024;
}

http {
    # Use Docker's embedded DNS so nginx resolves container
    # hostnames at request time rather than at startup. This
    # lets the load balancer start before its backends exist.
    resolver 127.0.0.11 valid=5s;

    # Map upgrade header to connection type for conditional websocket support.
    map $http_upgrade $connection_upgrade {
        default upgrade;
        ''      close;
    }

    # Distribute requests across coderd instances by request ID.
    split_clients $request_id $coderd_backend {
{{- range $i, $idx := .Instances }}
        {{ pct $i $.HACount }}  coderd-{{ $idx }}:3000;
{{- end }}
    }

    # Load-balanced coderd.
    server {
        listen 3000;
        location / {
            proxy_pass http://$coderd_backend;
            proxy_set_header Host $http_host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto $scheme;
            proxy_http_version 1.1;
            proxy_set_header Upgrade $http_upgrade;
            proxy_set_header Connection $connection_upgrade;
            proxy_read_timeout 86400s;
            proxy_send_timeout 86400s;
        }
    }
{{ range .Instances }}
    # coderd-{{ . }} direct access.
    server {
        listen {{ add $.CoderdBasePort (add . 1) }};
        location / {
            set $coderd_{{ . }} http://coderd-{{ . }}:3000;
            proxy_pass $coderd_{{ . }};
            proxy_set_header Host $http_host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto $scheme;
            proxy_http_version 1.1;
            proxy_set_header Upgrade $http_upgrade;
            proxy_set_header Connection $connection_upgrade;
            proxy_read_timeout 86400s;
            proxy_send_timeout 86400s;
        }
    }
{{ end }}
{{- range .Instances }}
    # pprof coderd-{{ . }}.
    server {
        listen {{ add $.PprofBasePort . }};
        location / {
            set $pprof_{{ . }} http://coderd-{{ . }}:6060;
            proxy_pass $pprof_{{ . }};
        }
    }
{{ end }}
{{- range .Instances }}
    # metrics coderd-{{ . }}.
    server {
        listen {{ add $.MetricsBasePort . }};
        location / {
            set $metrics_{{ . }} http://coderd-{{ . }}:2112;
            proxy_pass $metrics_{{ . }};
        }
    }
{{ end }}
    # OIDC.
    server {
        listen 4500;
        location / {
            set $oidc http://oidc:4500;
            proxy_pass $oidc;
        }
    }

    # Prometheus UI.
    server {
        listen 9090;
        location / {
            set $prometheus http://prometheus:9090;
            proxy_pass $prometheus;
        }
    }

    # Site dev server.
    server {
        listen 8080;
        location / {
            set $site http://site:8080;
            proxy_pass $site;
            proxy_set_header Host $http_host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto $scheme;
            proxy_http_version 1.1;
            proxy_set_header Upgrade $http_upgrade;
            proxy_set_header Connection $connection_upgrade;
            proxy_read_timeout 86400s;
            proxy_send_timeout 86400s;
        }
    }
}
`))

// generateNginxConfig builds the nginx.conf for load balancing all
// cdev services.
func generateNginxConfig(haCount int) string {
	instances := make([]int, haCount)
	for i := range haCount {
		instances[i] = i
	}
	var buf bytes.Buffer
	err := nginxConfigTmpl.Execute(&buf, nginxConfigData{
		HACount:         haCount,
		CoderdBasePort:  coderdBasePort,
		PprofBasePort:   pprofBasePort,
		MetricsBasePort: prometheusBasePort,
		Instances:       instances,
	})
	if err != nil {
		panic(fmt.Sprintf("nginx config template: %v", err))
	}
	return buf.String()
}
