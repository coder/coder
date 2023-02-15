package health

import (
	"context"
	"net/url"
	"time"

	"github.com/coder/coder/codersdk"
)

func AccessURLAccessible(accessURL string, timeout time.Duration) CheckFunc {
	u, err := url.Parse(accessURL)
	if err != nil {
		return func() error { return err }
	}
	client := codersdk.New(u)
	return func() error {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		_, err := client.BuildInfo(ctx)
		return err
	}
}
