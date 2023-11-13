package cliui

import (
	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/codersdk"
)

var defaultQuery = "owner:me"

// WorkspaceFilter wraps codersdk.WorkspaceFilter
// and allows easy integration to a CLI command.
// Example usage:
//
//	func (r *RootCmd) MyCmd() *clibase.Cmd {
//	  var (
//	    filter cliui.WorkspaceFilter
//	    ...
//	  )
//	  cmd := &clibase.Cmd{
//	    ...
//	  }
//	  filter.AttachOptions(&cmd.Options)
//	  ...
//	  return cmd
//	}
//
// The above will add the following flags to the command:
// --all
// --search
type WorkspaceFilter struct {
	searchQuery string
	all         bool
}

func (w *WorkspaceFilter) Filter() codersdk.WorkspaceFilter {
	var f codersdk.WorkspaceFilter
	if w.all {
		return f
	}
	f.FilterQuery = w.searchQuery
	if f.FilterQuery == "" {
		f.FilterQuery = defaultQuery
	}
	return f
}

func (w *WorkspaceFilter) AttachOptions(opts *clibase.OptionSet) {
	*opts = append(*opts,
		clibase.Option{
			Flag:          "all",
			FlagShorthand: "a",
			Description:   "Specifies whether all workspaces will be listed or not.",

			Value: clibase.BoolOf(&w.all),
		},
		clibase.Option{
			Flag:        "search",
			Description: "Search for a workspace with a query.",
			Default:     defaultQuery,
			Value:       clibase.StringOf(&w.searchQuery),
		},
	)
}
