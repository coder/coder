// Package gitsyncmock contains generated mocks for the gitsync package.
package gitsyncmock

//go:generate mockgen -destination ./store.go -package gitsyncmock github.com/coder/coder/v2/coderd/gitsync Store
//go:generate mockgen -destination ./publisher.go -package gitsyncmock github.com/coder/coder/v2/coderd/gitsync EventPublisher
