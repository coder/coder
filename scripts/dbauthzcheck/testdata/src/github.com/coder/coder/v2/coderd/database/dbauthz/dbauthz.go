// Package dbauthz is a minimal stand-in for
// github.com/coder/coder/v2/coderd/database/dbauthz used by the
// dbauthzcheck analyzer's testdata. Only the symbols the analyzer needs
// to see are declared here.
package dbauthz

import "context"

type Subject struct{}

func As(ctx context.Context, _ Subject) context.Context { return ctx }

func AsSystemRestricted(ctx context.Context) context.Context { return ctx }

func AsNotifier(ctx context.Context) context.Context { return ctx }

func AsFileReader(ctx context.Context) context.Context { return ctx }

func AsSubAgentAPI(ctx context.Context, _ string, _ string) context.Context { return ctx }

// AsRemoveActor is deliberately a value, not a function, to verify the
// analyzer doesn't flag it.
var AsRemoveActor = Subject{}

// Helper is a non-As helper used to verify that non-matching names are
// not flagged.
func Helper(ctx context.Context) context.Context { return ctx }
