//go:build timing

package testutil

var _ = func() any {
	timing = true
	return nil
}()
