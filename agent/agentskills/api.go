package agentskills

import (
	"net/http"
	"time"

	"github.com/ammario/tlru"
	"github.com/go-chi/chi/v5"

	"cdr.dev/slog/v3"

	"github.com/coder/coder/v2/coderd/httpapi"
)

// Skill represents a discovered skill's metadata.
type Skill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"path"`
}

// API exposes skill-discovery operations through the agent.
type API struct {
	logger  slog.Logger
	homeDir string
	cache   *tlru.Cache[string, []Skill]
}

const (
	skillCacheTTL    = 5 * time.Minute
	skillCacheMaxLen = 100
	cacheKey         = "skills"
)

func NewAPI(logger slog.Logger, homeDir string) *API {
	cache := tlru.New[string](tlru.ConstantCost[[]Skill], skillCacheMaxLen)
	return &API{
		logger:  logger,
		homeDir: homeDir,
		cache:   cache,
	}
}

// Routes returns the HTTP handler for skill-related routes.
func (api *API) Routes() http.Handler {
	r := chi.NewRouter()
	r.Get("/", api.handleListSkills)
	return r
}

func (api *API) handleListSkills(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if skills, _, ok := api.cache.Get(cacheKey); ok {
		httpapi.Write(ctx, rw, http.StatusOK, skills)
		return
	}

	skills := discoverSkills(api.logger, api.homeDir)
	api.cache.Set(cacheKey, skills, skillCacheTTL)
	httpapi.Write(ctx, rw, http.StatusOK, skills)
}
