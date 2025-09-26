package cli

import (
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

// allowListFlag implements pflag.SliceValue for codersdk.APIAllowListTarget entries.
type allowListFlag struct {
	targets *[]codersdk.APIAllowListTarget
}

func newAllowListFlag(dst *[]codersdk.APIAllowListTarget) *allowListFlag {
	return &allowListFlag{targets: dst}
}

func (a *allowListFlag) ensureSlice() error {
	if a.targets == nil {
		return xerrors.New("allow list destination is nil")
	}
	if *a.targets == nil {
		*a.targets = make([]codersdk.APIAllowListTarget, 0)
	}
	return nil
}

func (a *allowListFlag) String() string {
	if a.targets == nil || len(*a.targets) == 0 {
		return ""
	}
	parts := make([]string, len(*a.targets))
	for i, t := range *a.targets {
		parts[i] = t.String()
	}
	return strings.Join(parts, ",")
}

func (a *allowListFlag) Set(raw string) error {
	if err := a.ensureSlice(); err != nil {
		return err
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return xerrors.New("allow list entry cannot be empty")
	}
	var target codersdk.APIAllowListTarget
	if err := target.UnmarshalText([]byte(raw)); err != nil {
		return err
	}
	*a.targets = append(*a.targets, target)
	return nil
}

func (*allowListFlag) Type() string { return "allowList" }

func (a *allowListFlag) Append(value string) error {
	return a.Set(value)
}

func (a *allowListFlag) Replace(items []string) error {
	if err := a.ensureSlice(); err != nil {
		return err
	}
	(*a.targets) = (*a.targets)[:0]
	for _, item := range items {
		if err := a.Set(item); err != nil {
			return err
		}
	}
	return nil
}

func (a *allowListFlag) GetSlice() []string {
	if a.targets == nil {
		return nil
	}
	out := make([]string, len(*a.targets))
	for i, t := range *a.targets {
		out[i] = t.String()
	}
	return out
}
