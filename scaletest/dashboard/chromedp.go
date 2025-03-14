package dashboard
import (
	"errors"
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/coder/coder/v2/cryptorand"
	"cdr.dev/slog"
)
// Action is just a function that does something.
type Action func(ctx context.Context) error
// Selector locates an element on a page.
type Selector string
// Target is a thing that can be clicked.
type Target struct {
	// Label is a human-readable label for the target.
	Label Label
	// ClickOn is the selector that locates the element to be clicked.
	ClickOn Selector
	// WaitFor is a selector that is expected to appear after the target is clicked.
	WaitFor Selector
}
// Label identifies an action.
type Label string
var defaultTargets = []Target{
	{
		Label:   "workspace_list",
		ClickOn: `nav a[href="/workspaces"]:not(.active)`,
		WaitFor: `tr[role="button"][data-testid^="workspace-"]`,
	},
	{
		Label:   "starter_templates",
		ClickOn: `a[href="/starter-templates"]`,
		WaitFor: `a[href^="/starter-templates/"]`,
	},
	{
		Label:   "workspace_details",
		ClickOn: `tr[role="button"][data-testid^="workspace-"]`,
		WaitFor: `tr[role="button"][data-testid^="build-"]`,
	},
	{
		Label:   "workspace_build_details",
		ClickOn: `tr[role="button"][data-testid^="build-"]`,
		WaitFor: `*[aria-label="Build details"]`,
	},
	{
		Label:   "template_list",
		ClickOn: `nav a[href="/templates"]:not(.active)`,
		WaitFor: `tr[role="button"][data-testid^="template-"]`,
	},
	{
		Label:   "template_docs",
		ClickOn: `a[href^="/templates/"][href$="/docs"]:not([aria-current])`,
		WaitFor: `#readme`,
	},
	{
		Label:   "template_files",
		ClickOn: `a[href^="/templates/"][href$="/docs"]:not([aria-current])`,
		WaitFor: `.monaco-editor`,
	},
	{
		Label:   "template_versions",
		ClickOn: `a[href^="/templates/"][href$="/versions"]:not([aria-current])`,
		WaitFor: `tr[role="button"][data-testid^="version-"]`,
	},
	{
		Label:   "template_version_details",
		ClickOn: `tr[role="button"][data-testid^="version-"]`,
		WaitFor: `.monaco-editor`,
	},
	{
		Label:   "user_list",
		ClickOn: `nav a[href^="/users"]:not(.active)`,
		WaitFor: `tr[data-testid^="user-"]`,
	},
}
// clickRandomElement returns an action that will click an element from defaultTargets.
// If no elements are found, an error is returned.
// If more than one element is found, one is chosen at random.
// The label of the clicked element is returned.
func clickRandomElement(ctx context.Context, log slog.Logger, randIntn func(int) int, deadline time.Time) (Label, Action, error) {
	var xpath Selector
	var found bool
	var err error
	matches := make([]Target, 0)
	for _, tgt := range defaultTargets {
		xpath, found, err = randMatch(ctx, log, tgt.ClickOn, randIntn, deadline)
		if err != nil {
			return "", nil, fmt.Errorf("find matches for %q: %w", tgt.ClickOn, err)
		}
		if !found {
			continue
		}
		matches = append(matches, Target{
			Label:   tgt.Label,
			ClickOn: xpath,
			WaitFor: tgt.WaitFor,
		})
	}
	if len(matches) == 0 {
		log.Debug(ctx, "no matches found this time")
		return "", nil, fmt.Errorf("no matches found")
	}
	match := pick(matches, randIntn)
	act := func(actx context.Context) error {
		log.Debug(ctx, "clicking", slog.F("label", match.Label), slog.F("xpath", match.ClickOn))
		if err := runWithDeadline(ctx, deadline, chromedp.Click(match.ClickOn, chromedp.NodeReady)); err != nil {
			log.Error(ctx, "click failed", slog.F("label", match.Label), slog.F("xpath", match.ClickOn), slog.Error(err))
			return fmt.Errorf("click %q: %w", match.ClickOn, err)
		}
		if err := runWithDeadline(ctx, deadline, chromedp.WaitReady(match.WaitFor)); err != nil {
			log.Error(ctx, "wait failed", slog.F("label", match.Label), slog.F("xpath", match.WaitFor), slog.Error(err))
			return fmt.Errorf("wait for %q: %w", match.WaitFor, err)
		}
		return nil
	}
	return match.Label, act, nil
}
// randMatch returns a random match for the given selector.
// The returned selector is the full XPath of the matched node.
// If no matches are found, an error is returned.
// If multiple matches are found, one is chosen at random.
func randMatch(ctx context.Context, log slog.Logger, s Selector, randIntn func(int) int, deadline time.Time) (Selector, bool, error) {
	var nodes []*cdp.Node
	log.Debug(ctx, "getting nodes for selector", slog.F("selector", s))
	if err := runWithDeadline(ctx, deadline, chromedp.Nodes(s, &nodes, chromedp.NodeReady, chromedp.AtLeast(0))); err != nil {
		log.Debug(ctx, "failed to get nodes for selector", slog.F("selector", s), slog.Error(err))
		return "", false, fmt.Errorf("get nodes for selector %q: %w", s, err)
	}
	if len(nodes) == 0 {
		log.Debug(ctx, "no nodes found for selector", slog.F("selector", s))
		return "", false, nil
	}
	n := pick(nodes, randIntn)
	log.Debug(ctx, "found node", slog.F("node", n.FullXPath()))
	return Selector(n.FullXPath()), true, nil
}
func waitForWorkspacesPageLoaded(ctx context.Context, deadline time.Time) error {
	return runWithDeadline(ctx, deadline, chromedp.WaitReady(`tbody.MuiTableBody-root`))
}
func runWithDeadline(ctx context.Context, deadline time.Time, acts ...chromedp.Action) error {
	deadlineCtx, deadlineCancel := context.WithDeadline(ctx, deadline)
	defer deadlineCancel()
	c := chromedp.FromContext(ctx)
	tasks := chromedp.Tasks(acts)
	return tasks.Do(cdp.WithExecutor(deadlineCtx, c.Target))
}
// initChromeDPCtx initializes a chromedp context with the given session token cookie
//
//nolint:revive // yes, headless is a control flag
func initChromeDPCtx(ctx context.Context, log slog.Logger, u *url.URL, sessionToken string, headless bool) (context.Context, context.CancelFunc, error) {
	dir, err := os.MkdirTemp("", "scaletest-dashboard-*")
	if err != nil {
		return nil, nil, err
	}
	allocOpts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.UserDataDir(dir),
		chromedp.DisableGPU,
	)
	if !headless { // headless is the default
		allocOpts = append(allocOpts, chromedp.Flag("headless", false))
	}
	allocCtx, allocCtxCancel := chromedp.NewExecAllocator(ctx, allocOpts...)
	cdpCtx, cdpCancel := chromedp.NewContext(allocCtx)
	cancelFunc := func() {
		cdpCancel()
		allocCtxCancel()
		if err := os.RemoveAll(dir); err != nil {
			log.Error(ctx, "failed to remove temp user data dir", slog.F("dir", dir), slog.Error(err))
		}
	}
	// force a viewport size of 1024x768 so we don't go into mobile mode
	if err := chromedp.Run(cdpCtx, chromedp.EmulateViewport(1024, 768)); err != nil {
		cancelFunc()
		allocCtxCancel()
		return nil, nil, fmt.Errorf("set viewport size: %w", err)
	}
	// set cookies
	if err := setSessionTokenCookie(cdpCtx, sessionToken, u.Host); err != nil {
		cancelFunc()
		return nil, nil, fmt.Errorf("set session token cookie: %w", err)
	}
	// visit main page
	if err := visitMainPage(cdpCtx, u); err != nil {
		cancelFunc()
		return nil, nil, fmt.Errorf("visit main page: %w", err)
	}
	return cdpCtx, cancelFunc, nil
}
func setSessionTokenCookie(ctx context.Context, token, domain string) error {
	exp := cdp.TimeSinceEpoch(time.Now().Add(24 * time.Hour))
	err := chromedp.Run(ctx, network.SetCookie("coder_session_token", token).
		WithExpires(&exp).
		WithDomain(domain).
		WithHTTPOnly(false))
	if err != nil {
		return fmt.Errorf("set coder_session_token cookie: %w", err)
	}
	return nil
}
func visitMainPage(ctx context.Context, u *url.URL) error {
	return chromedp.Run(ctx, chromedp.Navigate(u.String()))
}
func Screenshot(ctx context.Context, name string) (string, error) {
	var buf []byte
	if err := chromedp.Run(ctx, chromedp.CaptureScreenshot(&buf)); err != nil {
		return "", fmt.Errorf("capture screenshot: %w", err)
	}
	randExt, err := cryptorand.String(4)
	if err != nil {
		// this should never happen
		return "", fmt.Errorf("generate random string: %w", err)
	}
	fname := fmt.Sprintf("scaletest-dashboard-%s-%s-%s.png", name, time.Now().Format("20060102-150405"), randExt)
	pwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	fpath := filepath.Join(pwd, fname)
	f, err := os.OpenFile(fpath, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return "", fmt.Errorf("open file: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(buf); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}
	return fpath, nil
}
// pick chooses a random element from a slice.
// If the slice is empty, it returns the zero value of the type.
func pick[T any](s []T, randIntn func(int) int) T {
	if len(s) == 0 {
		var zero T
		return zero
	}
	// nolint:gosec
	return s[randIntn(len(s))]
}
