package example

import (
	"context"

	"github.com/coder/coder/v2/coderd/database/dbauthz"
)

// Package-level calls should be flagged just like in-function calls.
// This verifies the analyzer walks all call sites, not only those
// nested in functions.
var _ = dbauthz.AsSystemRestricted(context.Background()) // want "using 'AsSystemRestricted' is dangerous and should be accompanied by a //dbauthzcheck:ignore comment explaining why it's ok"

func directMisuse(ctx context.Context) context.Context {
	return dbauthz.AsSystemRestricted(ctx) // want "using 'AsSystemRestricted' is dangerous and should be accompanied by a //dbauthzcheck:ignore comment explaining why it's ok"
}

func notifierMisuse(ctx context.Context) context.Context {
	return dbauthz.AsNotifier(ctx) // want "using 'AsNotifier' is dangerous and should be accompanied by a //dbauthzcheck:ignore comment explaining why it's ok"
}

func fileReaderMisuseBackgroundCtx() context.Context {
	return dbauthz.AsFileReader(context.Background()) // want "using 'AsFileReader' is dangerous and should be accompanied by a //dbauthzcheck:ignore comment explaining why it's ok"
}

// Multi-arg As helpers like dbauthz.As(ctx, actor) and
// dbauthz.AsSubAgentAPI(ctx, orgID, userID) are intentionally NOT
// flagged — the original ruleguard rule matched `dbauthz.$f($c)`,
// which only captured single-arg call expressions. Keeping the same
// scope makes this a pure port.
func asNotFlagged(ctx context.Context) context.Context {
	return dbauthz.As(ctx, dbauthz.Subject{})
}

func subAgentNotFlagged(ctx context.Context) context.Context {
	return dbauthz.AsSubAgentAPI(ctx, "org", "user")
}

// Parenthesized selector expressions should still be flagged.
func parenthesizedMisuse(ctx context.Context) context.Context {
	return (dbauthz.AsSystemRestricted)(ctx) // want "using 'AsSystemRestricted' is dangerous and should be accompanied by a //dbauthzcheck:ignore comment explaining why it's ok"
}

func suppressedTrailing(ctx context.Context) context.Context {
	return dbauthz.AsSystemRestricted(ctx) //dbauthzcheck:ignore // trailing form is fine
}

func suppressedLeading(ctx context.Context) context.Context {
	//dbauthzcheck:ignore // leading form is fine when the call is too long
	return dbauthz.AsSystemRestricted(ctx)
}

func suppressedLeadingWithSpace(ctx context.Context) context.Context {
	// dbauthzcheck:ignore -- authors sometimes put a space after the //
	return dbauthz.AsSystemRestricted(ctx)
}

// suppressedMultilineJustification covers the common case where the
// justification for the suppression spans multiple comment lines
// before the flagged call. The whole comment group covers the call
// on the line immediately after the group ends.
func suppressedMultilineJustification(ctx context.Context) context.Context {
	//dbauthzcheck:ignore // this route intentionally returns everyone's
	// resources so users can see the full list of regions even when
	// they personally lack permission to touch them.
	return dbauthz.AsSystemRestricted(ctx)
}

// suppressedBlockComment shows the /* ... */ form also works.
func suppressedBlockComment(ctx context.Context) context.Context {
	/* dbauthzcheck:ignore -- block-comment form is accepted too */
	return dbauthz.AsSystemRestricted(ctx)
}

// notSuppressedBlankLine verifies that a suppression comment separated
// from the call by a blank line does NOT apply, because the blank line
// ends the comment group and the call sits two lines past the group.
func notSuppressedBlankLine(ctx context.Context) context.Context {
	//dbauthzcheck:ignore

	return dbauthz.AsSystemRestricted(ctx) // want "using 'AsSystemRestricted' is dangerous and should be accompanied by a //dbauthzcheck:ignore comment explaining why it's ok"
}

// notSuppressedFollowingStatement verifies that a suppression directive
// only covers the single statement directly following its comment
// group. A later, unrelated call must still be flagged.
func notSuppressedFollowingStatement(ctx context.Context) context.Context {
	//dbauthzcheck:ignore // covers only the assignment below.
	suppressed := dbauthz.AsSystemRestricted(ctx)
	_ = suppressed
	return dbauthz.AsNotifier(ctx) // want "using 'AsNotifier' is dangerous and should be accompanied by a //dbauthzcheck:ignore comment explaining why it's ok"
}

// wrappingCall is a helper used to exercise the multi-line statement
// suppression path below.
func wrappingCall(_ context.Context, _ string) error { return nil }

// suppressedMultilineStatement covers the common pattern where the
// suppression directive sits above a multi-line call whose dbauthz.As*
// argument is on a line past the line immediately following the
// comment group. The analyzer walks from the enclosing statement's
// first line down to the flagged call, so the leading suppression
// still applies.
func suppressedMultilineStatement(ctx context.Context) error {
	//dbauthzcheck:ignore // system access is needed for this rare lookup.
	return wrappingCall(
		dbauthz.AsSystemRestricted(ctx),
		"some arg",
	)
}

// unsuppressedMultilineStatement is the negative case: the same shape,
// minus the suppression. The analyzer should still fire.
func unsuppressedMultilineStatement(ctx context.Context) error {
	return wrappingCall(
		dbauthz.AsSystemRestricted(ctx), // want "using 'AsSystemRestricted' is dangerous and should be accompanied by a //dbauthzcheck:ignore comment explaining why it's ok"
		"some arg",
	)
}

// asRemoveActor is a value, not a function call, and must not fire.
func asRemoveActor() dbauthz.Subject {
	return dbauthz.AsRemoveActor
}

// helperOK is a dbauthz function that doesn't start with "As".
func helperOK(ctx context.Context) context.Context {
	return dbauthz.Helper(ctx)
}

// shadowedPackage defines a local "dbauthz" that must not be confused
// with the real one. The analyzer matches by package path, not
// identifier text, so calls to this shadow are safe.
type shadowedPackage struct{}

func (shadowedPackage) AsSystemRestricted(ctx context.Context) context.Context {
	return ctx
}

func shadowedPackageOK(ctx context.Context) context.Context {
	var dbauthz shadowedPackage
	return dbauthz.AsSystemRestricted(ctx)
}

// asStringInComment ensures "dbauthz.AsSystemRestricted" appearing in a
// string doesn't trigger the analyzer.
func asStringInComment() string {
	return "dbauthz.AsSystemRestricted(ctx)"
}

// customCtxMisuse exercises the path where the argument's declared type
// is a struct that embeds context.Context. The analyzer should still
// see it as implementing the interface.
func customCtxMisuse() context.Context {
	return dbauthz.AsSystemRestricted(wrapCtx{context.Background()}) // want "using 'AsSystemRestricted' is dangerous and should be accompanied by a //dbauthzcheck:ignore comment explaining why it's ok"
}

type wrapCtx struct{ context.Context }

// quotedDirectiveInComment ensures that a comment mentioning the
// suppression directive inside a longer sentence does not accidentally
// silence a real diagnostic. The directive must appear at the start of
// the comment content.
func quotedDirectiveInComment(ctx context.Context) context.Context {
	// To silence this linter, add //dbauthzcheck:ignore on the call.
	return dbauthz.AsSystemRestricted(ctx) // want "using 'AsSystemRestricted' is dangerous and should be accompanied by a //dbauthzcheck:ignore comment explaining why it's ok"
}

// deeplyNestedSuppressed ensures the enclosing-statement search finds
// a suppression when the dbauthz.As* call is buried under several
// nested expressions.
func deeplyNestedSuppressed(ctx context.Context) context.Context {
	//dbauthzcheck:ignore // deeply nested auth wrap.
	return wrapMany(
		wrap(dbauthz.AsSystemRestricted(ctx)),
	)
}

// deeplyNestedUnsuppressed is the negative case.
func deeplyNestedUnsuppressed(ctx context.Context) context.Context {
	return wrapMany(
		wrap(dbauthz.AsSystemRestricted(ctx)), // want "using 'AsSystemRestricted' is dangerous and should be accompanied by a //dbauthzcheck:ignore comment explaining why it's ok"
	)
}

func wrap(c context.Context) context.Context     { return c }
func wrapMany(c context.Context) context.Context { return c }
