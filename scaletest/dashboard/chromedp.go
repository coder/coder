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
)

// Action is just a function that does something.
type Action func(ctx context.Context) error

// Selector locates an element on a page.
type Selector string

// Label identifies an action.
type Label string

// defaultSelectors is a map of labels to selectors.
var defaultSelectors = map[Label]Selector{
	"workspaces_list":            `nav a[href="/workspaces"]:not(.active)`,
	"templates_list":             `nav a[href="/templates"]:not(.active)`,
	"users_list":                 `nav a[href^="/users"]:not(.active)`,
	"deployment_status":          `nav a[href="/deployment/general"]:not(.active)`,
	"starter_templates":          `a[href="/starter-templates"]`,
	"workspaces_table_row":       `tr[role="button"][data-testid^="workspace-"]`,
	"workspace_builds_table_row": `tr[role="button"][data-testid^="build-"]`,
	"templates_table_row":        `tr[role="button"][data-testid^="template-"]`,
	"template_docs":              `a[href^="/templates/"][href$="/docs"]:not([aria-current])`,
	"template_files":             `a[href^="/templates/"][href$="/files"]:not([aria-current])`,
	"template_versions":          `a[href^="/templates/"][href$="/versions"]:not([aria-current])`,
	"template_embed":             `a[href^="/templates/"][href$="/embed"]:not([aria-current])`,
	"template_insights":          `a[href^="/templates/"][href$="/insights"]:not([aria-current])`,
}

// ClickRandomElement returns an action that will click an element from the given selectors at random.
// If no elements are found, an error is returned.
// If more than one element is found, one is chosen at random.
// The label of the clicked element is returned.
func ClickRandomElement(ctx context.Context) (Label, Action, error) {
	var matched Selector
	var matchedLabel Label
	var found bool
	var err error
	for l, s := range defaultSelectors {
		matched, found, err = randMatch(ctx, s)
		if err != nil {
			return "", nil, xerrors.Errorf("find matches for %q: %w", s, err)
		}
		if !found {
			continue
		}
		matchedLabel = l
		break
	}
	if !found {
		return "", nil, xerrors.Errorf("no matches found")
	}

	return "click_" + matchedLabel, func(ctx context.Context) error {
		if err := clickAndWait(ctx, matched); err != nil {
			return xerrors.Errorf("click %q: %w", matched, err)
		}
		return nil
	}, nil
}

// randMatch returns a random match for the given selector.
// The returned selector is the full XPath of the matched node.
// If no matches are found, an error is returned.
// If multiple matches are found, one is chosen at random.
func randMatch(ctx context.Context, s Selector) (Selector, bool, error) {
	var nodes []*cdp.Node
	err := chromedp.Run(ctx, chromedp.Nodes(s, &nodes, chromedp.NodeVisible, chromedp.AtLeast(0)))
	if err != nil {
		return "", false, xerrors.Errorf("get nodes for selector %q: %w", s, err)
	}
	if len(nodes) == 0 {
		return "", false, nil
	}
	n := pick(nodes)
	return Selector(n.FullXPath()), true, nil
}

// clickAndWait clicks the given selector and waits for the page to finish loading.
// The page is considered loaded when the network event "LoadingFinished" is received.
func clickAndWait(ctx context.Context, s Selector) error {
	return chromedp.Run(ctx, chromedp.Tasks{
		chromedp.Click(s, chromedp.NodeVisible),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return waitForEvent(ctx, func(e interface{}) bool {
				if _, ok := e.(*network.EventLoadingFinished); ok {
					return true
				}
				return false
			})
		}),
	})
}

// initChromeDPCtx initializes a chromedp context with the given session token cookie
//
//nolint:revive // yes, headless is a control flag
func initChromeDPCtx(ctx context.Context, u *url.URL, sessionToken string, headless bool) (context.Context, context.CancelFunc, error) {
	dir, err := os.MkdirTemp("", "scaletest-dashboard")
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
		_ = os.RemoveAll(dir)
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
	exp := cdp.TimeSinceEpoch(time.Now().Add(30 * 24 * time.Hour))
	err := chromedp.Run(ctx, network.SetCookie("coder_session_token", token).
		WithExpires(&exp).
		WithDomain(domain).
		WithHTTPOnly(false))
	if err != nil {
		return xerrors.Errorf("set coder_session_token cookie: %w", err)
	}
	return nil
}

// waitForEvent waits for a lifecycle event that matches the given function.
// Adapted from https://github.com/chromedp/chromedp/issues/431
func waitForEvent(ctx context.Context, matcher func(e interface{}) bool) error {
	ch := make(chan struct{})
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()
	chromedp.ListenTarget(cctx, func(evt interface{}) {
		if matcher(evt) {
			cancel()
			close(ch)
		}
	})
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func visitMainPage(ctx context.Context, u *url.URL) error {
	return chromedp.Run(ctx, chromedp.Navigate(u.String()))
}
