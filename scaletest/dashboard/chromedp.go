package dashboard

import (
	"context"
	"net/url"
	"os"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"golang.org/x/xerrors"

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

// ClickRandomElement returns an action that will click an element from defaultTargets.
// If no elements are found, an error is returned.
// If more than one element is found, one is chosen at random.
// The label of the clicked element is returned.
func ClickRandomElement(ctx context.Context, randIntn func(int) int) (Label, Action, error) {
	var xpath Selector
	var found bool
	var err error
	matches := make([]Target, 0)
	for _, tgt := range defaultTargets {
		xpath, found, err = randMatch(ctx, tgt.ClickOn, randIntn)
		if err != nil {
			return "", nil, xerrors.Errorf("find matches for %q: %w", tgt.ClickOn, err)
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
		return "", nil, xerrors.Errorf("no matches found")
	}
	match := pick(matches, randIntn)
	// rely on map iteration order being random
	act := func(actx context.Context) error {
		if err := clickAndWait(actx, match.ClickOn, match.WaitFor); err != nil {
			return xerrors.Errorf("click %q: %w", match.ClickOn, err)
		}
		return nil
	}
	return match.Label, act, nil
}

// randMatch returns a random match for the given selector.
// The returned selector is the full XPath of the matched node.
// If no matches are found, an error is returned.
// If multiple matches are found, one is chosen at random.
func randMatch(ctx context.Context, s Selector, randIntn func(int) int) (Selector, bool, error) {
	var nodes []*cdp.Node
	err := chromedp.Run(ctx, chromedp.Nodes(s, &nodes, chromedp.NodeVisible, chromedp.AtLeast(0)))
	if err != nil {
		return "", false, xerrors.Errorf("get nodes for selector %q: %w", s, err)
	}
	if len(nodes) == 0 {
		return "", false, nil
	}
	n := pick(nodes, randIntn)
	return Selector(n.FullXPath()), true, nil
}

// clickAndWait clicks the given selector and waits for the page to finish loading.
// The page is considered loaded when the network event "LoadingFinished" is received.
func clickAndWait(ctx context.Context, clickOn, waitFor Selector) error {
	return chromedp.Run(ctx, chromedp.Tasks{
		chromedp.Click(clickOn, chromedp.NodeVisible),
		chromedp.WaitVisible(waitFor, chromedp.NodeVisible),
	})
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

	// set cookies
	if err := setSessionTokenCookie(cdpCtx, sessionToken, u.Host); err != nil {
		cancelFunc()
		return nil, nil, xerrors.Errorf("set session token cookie: %w", err)
	}

	// visit main page
	if err := visitMainPage(cdpCtx, u); err != nil {
		cancelFunc()
		return nil, nil, xerrors.Errorf("visit main page: %w", err)
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
		return xerrors.Errorf("set coder_session_token cookie: %w", err)
	}
	return nil
}

func visitMainPage(ctx context.Context, u *url.URL) error {
	return chromedp.Run(ctx, chromedp.Navigate(u.String()))
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
