// Package notificationsmock contains a mocked implementation of the
// notifications.Enqueuer interface for use in tests.
package notificationsmock

//go:generate mockgen -destination ./notificationsmock.go -package notificationsmock github.com/coder/coder/v2/coderd/notifications Enqueuer
