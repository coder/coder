// Package updatecheck provides a mechanism for periodically checking
// for updates to Coder.
//
// The update check is performed by querying the GitHub API for the
// latest release of Coder.
package updatecheck

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/google/go-github/v43/github"
	"golang.org/x/mod/semver"
	"golang.org/x/xerrors"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
)

const (
	// defaultURL defines the URL to check for the latest version of Coder.
	defaultURL = "https://api.github.com/repos/coder/coder/releases/latest"
)

// Checker is responsible for periodically checking for updates.
type Checker struct {
	ctx        context.Context
	cancel     context.CancelFunc
	db         database.Store
	log        slog.Logger
	opts       Options
	firstCheck chan struct{}
	closed     chan struct{}
}

// Options set optional parameters for the update check.
type Options struct {
	// Client is the HTTP client to use for the update check,
	// if omitted, http.DefaultClient will be used.
	Client *http.Client
	// URL is the URL to check for the latest version of Coder,
	// if omitted, the default URL will be used.
	URL string
	// Interval is the interval at which to check for updates,
	// default 24h.
	Interval time.Duration
	// UpdateTimeout sets the timeout for the update check,
	// default 30s.
	UpdateTimeout time.Duration
	// Notify is called when a newer version of Coder (than the
	// last update check) is available.
	Notify func(r Result)
}

// New returns a new Checker that periodically checks for Coder updates.
func New(db database.Store, log slog.Logger, opts Options) *Checker {
	if opts.Client == nil {
		opts.Client = http.DefaultClient
	}
	if opts.URL == "" {
		opts.URL = defaultURL
	}
	if opts.Interval == 0 {
		opts.Interval = 24 * time.Hour
	}
	if opts.UpdateTimeout == 0 {
		opts.UpdateTimeout = 30 * time.Second
	}
	if opts.Notify == nil {
		opts.Notify = func(_ Result) {}
	}

	ctx, cancel := context.WithCancel(context.Background())
	c := &Checker{
		ctx:        ctx,
		cancel:     cancel,
		db:         db,
		log:        log,
		opts:       opts,
		firstCheck: make(chan struct{}),
		closed:     make(chan struct{}),
	}
	go c.start()
	return c
}

// Result is the result from the last update check.
type Result struct {
	Checked time.Time `json:"checked,omitempty"`
	Version string    `json:"version,omitempty"`
	URL     string    `json:"url,omitempty"`
}

// Latest returns the latest version of Coder.
func (c *Checker) Latest(ctx context.Context) (r Result, err error) {
	select {
	case <-c.ctx.Done():
		return r, c.ctx.Err()
	case <-ctx.Done():
		return r, ctx.Err()
	case <-c.firstCheck:
	}

	return c.lastUpdateCheck(ctx)
}

func (c *Checker) init() (Result, error) {
	defer close(c.firstCheck)

	r, err := c.lastUpdateCheck(c.ctx)
	if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
		return Result{}, xerrors.Errorf("last update check: %w", err)
	}
	if r.Checked.IsZero() || time.Since(r.Checked) > c.opts.Interval {
		r, err = c.update()
		if err != nil {
			return Result{}, xerrors.Errorf("update check failed: %w", err)
		}
	}

	return r, nil
}

func (c *Checker) start() {
	defer close(c.closed)

	r, err := c.init()
	if err != nil {
		if xerrors.Is(err, context.Canceled) {
			return
		}
		c.log.Error(c.ctx, "init failed", slog.Error(err))
	} else {
		c.opts.Notify(r)
	}

	t := time.NewTicker(c.opts.Interval)
	defer t.Stop()

	diff := time.Until(r.Checked.Add(c.opts.Interval))
	if diff > 0 {
		c.log.Debug(c.ctx, "time until next update check", slog.F("duration", diff))
		t.Reset(diff)
	} else {
		c.log.Debug(c.ctx, "time until next update check", slog.F("duration", c.opts.Interval))
	}

	for {
		select {
		case <-t.C:
			rr, err := c.update()
			if err != nil {
				if xerrors.Is(err, context.Canceled) {
					return
				}
				c.log.Error(c.ctx, "update check failed", slog.Error(err))
			} else {
				c.notifyIfNewer(r, rr)
				r = rr
			}
			c.log.Debug(c.ctx, "time until next update check", slog.F("duration", c.opts.Interval))
			t.Reset(c.opts.Interval)
		case <-c.ctx.Done():
			return
		}
	}
}

func (c *Checker) update() (r Result, err error) {
	ctx, cancel := context.WithTimeout(c.ctx, c.opts.UpdateTimeout)
	defer cancel()

	c.log.Debug(c.ctx, "checking for update")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.opts.URL, nil)
	if err != nil {
		return r, xerrors.Errorf("new request: %w", err)
	}
	resp, err := c.opts.Client.Do(req)
	if err != nil {
		return r, xerrors.Errorf("client do: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return r, xerrors.Errorf("unexpected status code %d: %s", resp.StatusCode, b)
	}

	var rr github.RepositoryRelease
	err = json.NewDecoder(resp.Body).Decode(&rr)
	if err != nil {
		return r, xerrors.Errorf("json decode: %w", err)
	}

	r = Result{
		Checked: time.Now(),
		Version: rr.GetTagName(),
		URL:     rr.GetHTMLURL(),
	}
	c.log.Debug(ctx, "update check result", slog.F("latest_version", r.Version))

	b, err := json.Marshal(r)
	if err != nil {
		return r, xerrors.Errorf("json marshal result: %w", err)
	}

	// nolint:gocritic // Inserting the last update check is a system function.
	err = c.db.UpsertLastUpdateCheck(dbauthz.AsSystemRestricted(ctx), string(b))
	if err != nil {
		return r, err
	}

	return r, nil
}

func (c *Checker) notifyIfNewer(prev, next Result) {
	if (prev.Version == "" && next.Version != "") || semver.Compare(next.Version, prev.Version) > 0 {
		c.opts.Notify(next)
	}
}

func (c *Checker) lastUpdateCheck(ctx context.Context) (r Result, err error) {
	// nolint:gocritic // Getting the last update check is a system function.
	s, err := c.db.GetLastUpdateCheck(dbauthz.AsSystemRestricted(ctx))
	if err != nil {
		return r, err
	}
	return r, json.Unmarshal([]byte(s), &r)
}

func (c *Checker) Close() error {
	c.cancel()
	<-c.closed
	return nil
}
