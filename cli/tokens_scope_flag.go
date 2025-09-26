package cli

import (
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

// scopeFlag stores repeatable --scope values as typed APIKeyScope.
type scopeFlag struct {
	scopes *[]codersdk.APIKeyScope
}

func newScopeFlag(dst *[]codersdk.APIKeyScope) *scopeFlag {
	return &scopeFlag{scopes: dst}
}

func (s *scopeFlag) ensureSlice() error {
	if s.scopes == nil {
		return xerrors.New("scope destination is nil")
	}
	if *s.scopes == nil {
		*s.scopes = make([]codersdk.APIKeyScope, 0)
	}
	return nil
}

func (s *scopeFlag) String() string {
	if s.scopes == nil || len(*s.scopes) == 0 {
		return ""
	}
	parts := make([]string, len(*s.scopes))
	for i, scope := range *s.scopes {
		parts[i] = string(scope)
	}
	return strings.Join(parts, ",")
}

func (s *scopeFlag) Set(raw string) error {
	if err := s.ensureSlice(); err != nil {
		return err
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return xerrors.New("scope cannot be empty")
	}
	*s.scopes = append(*s.scopes, codersdk.APIKeyScope(raw))
	return nil
}

func (*scopeFlag) Type() string { return "scope" }

func (s *scopeFlag) Append(value string) error {
	return s.Set(value)
}

func (s *scopeFlag) Replace(items []string) error {
	if err := s.ensureSlice(); err != nil {
		return err
	}
	(*s.scopes) = (*s.scopes)[:0]
	for _, item := range items {
		if err := s.Set(item); err != nil {
			return err
		}
	}
	return nil
}

func (s *scopeFlag) GetSlice() []string {
	if s.scopes == nil {
		return nil
	}
	out := make([]string, len(*s.scopes))
	for i, scope := range *s.scopes {
		out[i] = string(scope)
	}
	return out
}
