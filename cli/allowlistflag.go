package cli

import (
	"encoding/csv"
	"strings"

	"github.com/spf13/pflag"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

var (
	_ pflag.SliceValue = &AllowListFlag{}
	_ pflag.Value      = &AllowListFlag{}
)

// AllowListFlag implements pflag.SliceValue for codersdk.APIAllowListTarget entries.
type AllowListFlag []codersdk.APIAllowListTarget

func AllowListFlagOf(al *[]codersdk.APIAllowListTarget) *AllowListFlag {
	return (*AllowListFlag)(al)
}

func (a AllowListFlag) String() string {
	return strings.Join(a.GetSlice(), ",")
}

func (a AllowListFlag) Value() []codersdk.APIAllowListTarget {
	return []codersdk.APIAllowListTarget(a)
}

func (AllowListFlag) Type() string { return "allow-list" }

func (a *AllowListFlag) Set(set string) error {
	values, err := csv.NewReader(strings.NewReader(set)).Read()
	if err != nil {
		return xerrors.Errorf("parse allow list entries as csv: %w", err)
	}
	for _, v := range values {
		if err := a.Append(v); err != nil {
			return err
		}
	}
	return nil
}

func (a *AllowListFlag) Append(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return xerrors.New("allow list entry cannot be empty")
	}
	var target codersdk.APIAllowListTarget
	if err := target.UnmarshalText([]byte(value)); err != nil {
		return err
	}

	*a = append(*a, target)
	return nil
}

func (a *AllowListFlag) Replace(items []string) error {
	*a = []codersdk.APIAllowListTarget{}
	for _, item := range items {
		if err := a.Append(item); err != nil {
			return err
		}
	}
	return nil
}

func (a *AllowListFlag) GetSlice() []string {
	out := make([]string, len(*a))
	for i, entry := range *a {
		out[i] = entry.String()
	}
	return out
}
