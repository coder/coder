package cli

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestHeaderTransport(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"JWTRefresh", "StaticOutput", "CommandFailure"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			clock := quartz.NewMock(t)
			signer, err := jose.NewSigner(jose.SigningKey{
				Algorithm: jose.HS512,
				Key:       []byte(strings.Repeat("k", 64)),
			}, nil)
			require.NoError(t, err)
			token := func(expiry *jwt.NumericDate) string {
				value, err := jwt.Signed(signer).Claims(jwt.Claims{Expiry: expiry}).Serialize()
				require.NoError(t, err)
				return value
			}
			first := token(jwt.NewNumericDate(clock.Now().Add(time.Minute)))
			later := token(jwt.NewNumericDate(clock.Now().Add(time.Hour)))
			initial := "Authorization=Bearer " + first + "\nX-Token=" + later + "\nX-Removed=old\n"
			if name == "StaticOutput" {
				initial = "X-Value=static\nX-Token=" + token(nil) + "\n"
			}

			outputPath := filepath.Join(t.TempDir(), "headers")
			write := func(output string) {
				require.NoError(t, os.WriteFile(outputPath, []byte(output), 0o600))
			}
			write(initial)
			command := fmt.Sprintf("cat %q", outputPath)
			if runtime.GOOS == "windows" {
				command = `type "` + outputPath + `"`
			}
			serverURL, err := url.Parse("https://coder.example.com")
			require.NoError(t, err)
			transport, err := headerTransport(ctx, serverURL, []string{"X-Static=unchanged"}, command, clock)
			require.NoError(t, err)

			var received []http.Header
			transport.Transport = roundTripper(func(req *http.Request) (*http.Response, error) {
				received = append(received, req.Header.Clone())
				return httptest.NewRecorder().Result(), nil
			})
			send := func() error {
				req := httptest.NewRequest(http.MethodGet, serverURL.String(), nil).WithContext(ctx)
				res, err := transport.RoundTrip(req)
				if res != nil {
					require.NoError(t, res.Body.Close())
				}
				return err
			}

			require.NoError(t, send())
			require.Len(t, received, 1)
			require.Equal(t, "unchanged", received[0].Get("X-Static"))
			updated := "Authorization=Bearer " + later + "\nX-Value=updated\n"
			write(updated)

			if name == "StaticOutput" {
				clock.Advance(24 * time.Hour).MustWait(ctx)
				require.NoError(t, send())
				require.Equal(t, received[0], received[1])
				return
			}

			clock.Advance(49 * time.Second).MustWait(ctx)
			require.NoError(t, send())
			require.Equal(t, received[0], received[1])
			clock.Advance(time.Second).MustWait(ctx)

			if name == "CommandFailure" {
				require.NoError(t, os.Remove(outputPath))
				err := send()
				require.ErrorContains(t, err, "run header command")
				require.NotContains(t, err.Error(), command)
				require.Len(t, received, 2, "a failed refresh must not send stale headers")
				write(updated)
			}

			require.NoError(t, send())
			require.Len(t, received, 3)
			require.Equal(t, []string{"Bearer " + later}, received[2].Values("Authorization"))
			require.Equal(t, "unchanged", received[2].Get("X-Static"))
			require.Equal(t, "updated", received[2].Get("X-Value"))
			require.Empty(t, received[2].Get("X-Removed"))
			require.Empty(t, received[2].Get("X-Token"))
		})
	}
}
