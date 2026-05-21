package cliui

import (
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

var defaultQuery = "owner:me"

// WorkspaceFilter wraps codersdk.WorkspaceFilter
// and allows easy integration to a CLI command.
// Example usage:
//
//	func (r *RootCmd) MyCmd() *serpent.Command {
//	  var (
//	    filter cliui.WorkspaceFilter
//	    ...
//	  )
//	  cmd := &serpent.Command{
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

func (w *WorkspaceFilter) AttachOptions(opts *serpent.OptionSet) {
	*opts = append(*opts,
		serpent.Option{
			Flag:          "all",
			FlagShorthand: "a",
			Description:   "Specifies whether all workspaces will be listed or not.",

			Value: serpent.BoolOf(&w.all),
		},
		serpent.Option{
			Flag:        "search",
			Description: "Search for a workspace with a query.",
			Default:     defaultQuery,
			Value:       serpent.StringOf(&w.searchQuery),
		},
	)
}
