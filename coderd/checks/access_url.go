package checks

import (
	"context"
	"net/url"
	"time"

	"github.com/coder/coder/codersdk"
)

func CanHitAccessURL(accessURL *url.URL, timeout time.Duration) CheckFunc {
	client := codersdk.New(accessURL)
	return func() error {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		_, err := client.BuildInfo(ctx)
		return err
	}
}
